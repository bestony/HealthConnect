package duckdb

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"com.bestony.health-connect/internal/domain"
	"com.bestony.health-connect/internal/ingest/applexml"
)

func TestWorkoutsQuery(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	userID, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	importID, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := loadFixture(t, db, userID, importID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CommitImport(ctx, userID, importID, "replace"); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	rows, err := db.Workouts(ctx, userID, from, to, 10, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestReplaceThenMerge(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	userID, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	first, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := loadFixture(t, db, userID, first); err != nil {
		t.Fatal(err)
	}
	written, err := db.CommitImport(ctx, userID, first, "replace")
	if err != nil {
		t.Fatal(err)
	}
	// steps, sleep, stand, blood pressure correlation, workout, activity summary.
	// Systolic and diastolic are quantity rows too.
	if written != 8 {
		t.Fatalf("written = %d", written)
	}
	count, err := db.CountFacts(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 8 {
		t.Fatalf("count = %d", count)
	}

	second, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := loadFixture(t, db, userID, second); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CommitImport(ctx, userID, second, "merge"); err != nil {
		t.Fatal(err)
	}
	count, err = db.CountFacts(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 8 {
		t.Fatalf("merge count = %d", count)
	}

	third, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := loadFixture(t, db, userID, third); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CommitImport(ctx, userID, third, "replace"); err != nil {
		t.Fatal(err)
	}
	count, err = db.CountFacts(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 8 {
		t.Fatalf("second replace count = %d", count)
	}
	var steps float64
	if err := db.sql.QueryRowContext(ctx, `
		SELECT value FROM quantity_samples
		WHERE user_id = CAST(? AS UUID) AND type = 'HKQuantityTypeIdentifierStepCount'`, userID).Scan(&steps); err != nil {
		t.Fatal(err)
	}
	if steps != 1200 {
		t.Fatalf("steps = %v", steps)
	}
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "health.duckdb"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func loadFixture(t *testing.T, db *DB, userID, importID string) error {
	t.Helper()
	f, err := os.Open("../../../testdata/export_min.xml")
	if err != nil {
		return err
	}
	defer f.Close()
	return db.WithLoader(context.Background(), func(loader *Loader) error {
		sink := &fixtureSink{loader: loader, userID: userID, importID: importID}
		return applexml.Parse(f, sink)
	})
}

type fixtureSink struct {
	loader   *Loader
	userID   string
	importID string
}

func (s *fixtureSink) OnExportDate(time.Time) error              { return nil }
func (s *fixtureSink) OnCharacteristics(map[string]string) error { return nil }
func (s *fixtureSink) OnIgnored(string) error                    { return nil }
func (s *fixtureSink) OnQuantity(sample domain.QuantitySample) error {
	return s.loader.AppendQuantity(s.userID, s.importID, sample)
}
func (s *fixtureSink) OnCategory(sample domain.CategorySample) error {
	return s.loader.AppendCategory(s.userID, s.importID, sample)
}
func (s *fixtureSink) OnCorrelation(item domain.Correlation) error {
	return s.loader.AppendCorrelation(s.userID, s.importID, item)
}
func (s *fixtureSink) OnWorkout(item domain.Workout) error {
	return s.loader.AppendWorkout(s.userID, s.importID, item)
}
func (s *fixtureSink) OnActivity(item domain.ActivitySummary) error {
	return s.loader.AppendActivity(s.userID, s.importID, item)
}
