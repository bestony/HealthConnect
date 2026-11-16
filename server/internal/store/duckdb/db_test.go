package duckdb

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "health.duckdb")
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(ctx, path, log)
	if err != nil {
		t.Fatal(err)
	}
	var tables int
	err = db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'main' AND table_name IN (
			'quantity_samples', 'category_samples', 'correlations', 'correlation_members',
			'workouts', 'workout_events', 'workout_statistics', 'activity_summaries',
			'quantity_samples_staging'
		)`).Scan(&tables)
	if err != nil {
		t.Fatal(err)
	}
	if tables != 9 {
		t.Fatalf("tables = %d", tables)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}
