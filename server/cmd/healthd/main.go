// Command healthd serves the HealthConnect ingest API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"com.bestony.health-connect/internal/config"
	"com.bestony.health-connect/internal/export"
	"com.bestony.health-connect/internal/httpapi"
	"com.bestony.health-connect/internal/ingest"
	"com.bestony.health-connect/internal/store/duckdb"
	"com.bestony.health-connect/internal/store/sqlite"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "healthd: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	meta, err := sqlite.Open(ctx, filepath.Join(cfg.DataDir, "meta.sqlite"), log)
	if err != nil {
		return err
	}
	defer func() { _ = meta.Close() }()

	tokenPath := filepath.Join(cfg.DataDir, "bootstrap.token")
	created, err := meta.BootstrapToken(ctx, tokenPath)
	if err != nil {
		return err
	}
	if created {
		log.Info("bootstrap token created", "path", tokenPath)
	}
	if err := meta.InterruptActive(ctx); err != nil {
		return err
	}

	facts, err := duckdb.Open(ctx, filepath.Join(cfg.DataDir, "health.duckdb"), log)
	if err != nil {
		return err
	}
	defer func() { _ = facts.Close() }()
	if err := facts.ClearStaging(ctx); err != nil {
		return err
	}

	inbox := filepath.Join(cfg.DataDir, "inbox")
	worker := ingest.NewWorker(meta, facts, inbox, ingest.Limits{
		MaxEntries: 10000,
		MaxBytes:   cfg.MaxUncompressedBytes,
	}, log)
	worker.Start(ctx)
	service := ingest.NewService(meta, worker, inbox, log)
	exports := export.NewService(meta, facts, cfg.DataDir, log)

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.New(httpapi.Deps{
			Log: log, Tokens: meta, NotFound: sqlite.ErrNotFound,
			Imports: service, Exports: exports, Facts: facts, Meta: meta, MaxUpload: cfg.MaxUploadBytes,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("http listen", "addr", cfg.HTTPAddr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		log.Info("http shutdown")
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}
