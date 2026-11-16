// Package duckdb stores Apple Health facts for analytical reads.
package duckdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"com.bestony.health-connect/migrations"

	_ "github.com/duckdb/duckdb-go/v2"
)

// DB is one embedded DuckDB file. One process may write it.
type DB struct {
	sql  *sql.DB
	log  *slog.Logger
	path string
}

// Open creates the database file and applies migrations.
func Open(ctx context.Context, path string, log *slog.Logger) (*DB, error) {
	if log == nil {
		log = slog.Default()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("duckdb abs: %w", err)
	}
	path = abs
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("duckdb mkdir: %w", err)
	}
	sqlDB, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("duckdb open: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	db := &DB{sql: sqlDB, log: log, path: path}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("duckdb ping: %w", err)
	}
	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := chmod(path); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	log.Info("duckdb ready", "path", path)
	return db, nil
}

// Close closes the database.
func (db *DB) Close() error {
	return db.sql.Close()
}

func (db *DB) migrate(ctx context.Context) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("duckdb begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL
		)`); err != nil {
		return fmt.Errorf("duckdb schema_migrations: %w", err)
	}
	entries, err := fs.ReadDir(migrations.DuckDB, "duckdb")
	if err != nil {
		return fmt.Errorf("duckdb read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&n); err != nil {
			return fmt.Errorf("duckdb migration lookup: %w", err)
		}
		if n > 0 {
			continue
		}
		body, err := migrations.DuckDB.ReadFile("duckdb/" + name)
		if err != nil {
			return fmt.Errorf("duckdb read %s: %w", name, err)
		}
		for _, stmt := range splitSQL(string(body)) {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("duckdb apply %s: %w", name, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, version, time.Now().UTC()); err != nil {
			return fmt.Errorf("duckdb record %s: %w", name, err)
		}
		db.log.Info("duckdb migration applied", "version", version, "file", name)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("duckdb commit migrations: %w", err)
	}
	return nil
}

func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("duckdb migration name %q: want NNN_name.sql", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("duckdb migration name %q: %w", name, err)
	}
	return version, nil
}

func splitSQL(script string) []string {
	parts := strings.Split(script, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}

func chmod(path string) error {
	for _, suffix := range []string{"", ".wal"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("duckdb stat: %w", err)
		}
		if info.Mode().Perm() == 0o600 {
			continue
		}
		if err := os.Chmod(path+suffix, 0o600); err != nil {
			return fmt.Errorf("duckdb chmod: %w", err)
		}
	}
	return nil
}
