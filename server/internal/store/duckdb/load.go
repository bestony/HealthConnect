package duckdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"com.bestony.health-connect/internal/domain"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
)

const flushEvery = 20000

// Loader appends facts into staging tables. Close it before CommitImport.
type Loader struct {
	quantity    *duckdb.Appender
	category    *duckdb.Appender
	correlation *duckdb.Appender
	members     *duckdb.Appender
	workouts    *duckdb.Appender
	events      *duckdb.Appender
	stats       *duckdb.Appender
	activity    *duckdb.Appender
	records     *duckdb.Appender
	pending     int
	all         []*duckdb.Appender
}

// NewLoader builds appenders on a driver connection.
func NewLoader(conn driver.Conn) (*Loader, error) {
	names := []string{
		"quantity_samples_staging",
		"category_samples_staging",
		"correlations_staging",
		"correlation_members_staging",
		"workouts_staging",
		"workout_events_staging",
		"workout_statistics_staging",
		"activity_summaries_staging",
		"health_records_staging",
	}
	apps := make([]*duckdb.Appender, 0, len(names))
	for _, name := range names {
		app, err := duckdb.NewAppenderFromConn(conn, "main", name)
		if err != nil {
			for _, opened := range apps {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("duckdb appender %s: %w", name, err)
		}
		apps = append(apps, app)
	}
	return &Loader{
		quantity: apps[0], category: apps[1], correlation: apps[2], members: apps[3],
		workouts: apps[4], events: apps[5], stats: apps[6], activity: apps[7], records: apps[8], all: apps,
	}, nil
}

// Flush writes buffered rows.
func (l *Loader) Flush() error {
	for _, app := range l.all {
		if err := app.Flush(); err != nil {
			return fmt.Errorf("duckdb flush: %w", err)
		}
	}
	l.pending = 0
	return nil
}

// Close flushes and closes the appenders.
func (l *Loader) Close() error {
	var first error
	for _, app := range l.all {
		if err := app.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (l *Loader) note() error {
	l.pending++
	if l.pending < flushEvery {
		return nil
	}
	return l.Flush()
}

// AppendQuantity stages one numeric sample.
func (l *Loader) AppendQuantity(userID, importID string, sample domain.QuantitySample) error {
	user, imp, err := ids(userID, importID)
	if err != nil {
		return err
	}
	if err := l.quantity.AppendRow(
		user, imp, nil, domain.QuantityKey(userID, sample), sample.Type, sample.Value, sample.Unit,
		sample.SourceName, emptyNil(sample.SourceVersion), emptyNil(sample.Device),
		sample.Start.UTC(), sample.End.UTC(), int16(sample.TZOffsetMin), timeOrNil(sample.Creation),
		jsonMap(sample.Metadata), jsonMap(sample.Attrs),
	); err != nil {
		return fmt.Errorf("duckdb append quantity: %w", err)
	}
	return l.note()
}

// AppendCategory stages one category sample.
func (l *Loader) AppendCategory(userID, importID string, sample domain.CategorySample) error {
	user, imp, err := ids(userID, importID)
	if err != nil {
		return err
	}
	if err := l.category.AppendRow(
		user, imp, nil, domain.CategoryKey(userID, sample), sample.Type, sample.ValueText,
		sample.SourceName, emptyNil(sample.SourceVersion), emptyNil(sample.Device),
		sample.Start.UTC(), sample.End.UTC(), int16(sample.TZOffsetMin), timeOrNil(sample.Creation),
		jsonMap(sample.Metadata), jsonMap(sample.Attrs),
	); err != nil {
		return fmt.Errorf("duckdb append category: %w", err)
	}
	return l.note()
}

// AppendCorrelation stages one correlation and its child samples.
func (l *Loader) AppendCorrelation(userID, importID string, item domain.Correlation) error {
	user, imp, err := ids(userID, importID)
	if err != nil {
		return err
	}
	corrID, err := parseUUID(item.ID)
	if err != nil {
		return err
	}
	if err := l.correlation.AppendRow(
		corrID, user, imp, nil, domain.CorrelationKey(userID, item), item.Type,
		item.SourceName, emptyNil(item.SourceVersion), emptyNil(item.Device),
		item.Start.UTC(), item.End.UTC(), int16(item.TZOffsetMin), timeOrNil(item.Creation),
		jsonMap(item.Metadata), jsonMap(item.Attrs),
	); err != nil {
		return fmt.Errorf("duckdb append correlation: %w", err)
	}
	if err := l.note(); err != nil {
		return err
	}
	for _, sample := range item.Quantities {
		if err := l.members.AppendRow(corrID, imp, domain.QuantityKey(userID, sample)); err != nil {
			return fmt.Errorf("duckdb append correlation member: %w", err)
		}
		if err := l.note(); err != nil {
			return err
		}
		if err := l.AppendQuantity(userID, importID, sample); err != nil {
			return err
		}
	}
	for _, sample := range item.Categories {
		if err := l.members.AppendRow(corrID, imp, domain.CategoryKey(userID, sample)); err != nil {
			return fmt.Errorf("duckdb append correlation member: %w", err)
		}
		if err := l.note(); err != nil {
			return err
		}
		if err := l.AppendCategory(userID, importID, sample); err != nil {
			return err
		}
	}
	return nil
}

// AppendWorkout stages one workout and its children.
func (l *Loader) AppendWorkout(userID, importID string, item domain.Workout) error {
	user, imp, err := ids(userID, importID)
	if err != nil {
		return err
	}
	workoutID, err := parseUUID(item.ID)
	if err != nil {
		return err
	}
	if err := l.workouts.AppendRow(
		workoutID, user, imp, nil, domain.WorkoutKey(userID, item), item.ActivityType,
		f64(item.DurationSec), f64(item.Distance), emptyNil(item.DistanceUnit),
		f64(item.Energy), emptyNil(item.EnergyUnit), item.SourceName,
		emptyNil(item.SourceVersion), emptyNil(item.Device),
		item.Start.UTC(), item.End.UTC(), int16(item.TZOffsetMin), timeOrNil(item.Creation),
		jsonMap(item.Metadata), jsonMap(item.Attrs),
	); err != nil {
		return fmt.Errorf("duckdb append workout: %w", err)
	}
	if err := l.note(); err != nil {
		return err
	}
	for _, event := range item.Events {
		if err := l.events.AppendRow(workoutID, imp, event.Type, event.At.UTC(), int16(event.TZOffsetMin), f64(event.DurationSec)); err != nil {
			return fmt.Errorf("duckdb append workout event: %w", err)
		}
		if err := l.note(); err != nil {
			return err
		}
	}
	for _, stat := range item.Statistics {
		if err := l.stats.AppendRow(workoutID, imp, stat.Type, f64(stat.Sum), f64(stat.Minimum), f64(stat.Maximum), f64(stat.Average), emptyNil(stat.Unit)); err != nil {
			return fmt.Errorf("duckdb append workout statistic: %w", err)
		}
		if err := l.note(); err != nil {
			return err
		}
	}
	return nil
}

// AppendActivity stages one daily summary.
func (l *Loader) AppendActivity(userID, importID string, item domain.ActivitySummary) error {
	user, imp, err := ids(userID, importID)
	if err != nil {
		return err
	}
	if err := l.activity.AppendRow(
		user, imp, item.Day.UTC(),
		f64(item.ActiveEnergy), f64(item.ActiveEnergyGoal), emptyNil(item.ActiveEnergyUnit),
		f64(item.ExerciseMinutes), f64(item.ExerciseGoal), f64(item.StandHours), f64(item.StandGoal),
		f64(item.MoveMinutes), f64(item.MoveGoal), jsonMap(item.Attrs),
	); err != nil {
		return fmt.Errorf("duckdb append activity: %w", err)
	}
	return l.note()
}

// HealthRecord is one raw NDJSON record kept beside the typed fact tables.
type HealthRecord struct {
	Kind           string
	TypeIdentifier string
	UUID           string
	Start          *time.Time
	End            *time.Time
	TZOffsetMin    *int
	Source         string
	Device         string
	Payload        json.RawMessage
}

// AppendHealthRecord stages one raw record.
func (l *Loader) AppendHealthRecord(userID, importID string, rec HealthRecord) error {
	user, imp, err := ids(userID, importID)
	if err != nil {
		return err
	}
	var offset any
	if rec.TZOffsetMin != nil {
		offset = int16(*rec.TZOffsetMin)
	}
	payload := any(nil)
	if len(rec.Payload) > 0 {
		payload = json.RawMessage(rec.Payload)
	}
	if err := l.records.AppendRow(
		user, imp, rec.Kind, rec.TypeIdentifier, emptyNil(rec.UUID),
		timeOrNil(rec.Start), timeOrNil(rec.End), offset,
		emptyNil(rec.Source), emptyNil(rec.Device), payload,
	); err != nil {
		return fmt.Errorf("duckdb append health record: %w", err)
	}
	return l.note()
}

// WithLoader runs fn while a driver connection is held, then releases it.
func (db *DB) WithLoader(ctx context.Context, fn func(*Loader) error) error {
	conn, err := db.sql.Conn(ctx)
	if err != nil {
		return fmt.Errorf("duckdb conn: %w", err)
	}
	defer conn.Close()
	return conn.Raw(func(raw any) error {
		loader, err := NewLoader(raw.(driver.Conn))
		if err != nil {
			return err
		}
		defer loader.Close()
		if err := fn(loader); err != nil {
			return err
		}
		return loader.Flush()
	})
}

// CommitImport moves staging rows into the fact tables.
func (db *DB) CommitImport(ctx context.Context, userID, importID, mode string) (int64, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("duckdb begin commit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if mode == "replace" {
		statements := []string{
			`DELETE FROM correlation_members WHERE correlation_id IN (SELECT id FROM correlations WHERE user_id = CAST(? AS UUID))`,
			`DELETE FROM correlations WHERE user_id = CAST(? AS UUID)`,
			`DELETE FROM workout_events WHERE workout_id IN (SELECT id FROM workouts WHERE user_id = CAST(? AS UUID))`,
			`DELETE FROM workout_statistics WHERE workout_id IN (SELECT id FROM workouts WHERE user_id = CAST(? AS UUID))`,
			`DELETE FROM workouts WHERE user_id = CAST(? AS UUID)`,
			`DELETE FROM quantity_samples WHERE user_id = CAST(? AS UUID)`,
			`DELETE FROM category_samples WHERE user_id = CAST(? AS UUID)`,
			`DELETE FROM activity_summaries WHERE user_id = CAST(? AS UUID)`,
			`DELETE FROM health_records WHERE user_id = CAST(? AS UUID)`,
		}
		for _, stmt := range statements {
			if _, err := tx.ExecContext(ctx, stmt, userID); err != nil {
				return 0, fmt.Errorf("duckdb replace delete: %w", err)
			}
		}
	}
	inserts := []string{
		`INSERT INTO quantity_samples SELECT * FROM quantity_samples_staging s
		 WHERE s.import_id = CAST(? AS UUID) AND NOT EXISTS (
		   SELECT 1 FROM quantity_samples q WHERE q.user_id = s.user_id AND q.natural_key = s.natural_key)`,
		`INSERT INTO category_samples SELECT * FROM category_samples_staging s
		 WHERE s.import_id = CAST(? AS UUID) AND NOT EXISTS (
		   SELECT 1 FROM category_samples q WHERE q.user_id = s.user_id AND q.natural_key = s.natural_key)`,
		`INSERT INTO correlations SELECT * FROM correlations_staging s
		 WHERE s.import_id = CAST(? AS UUID) AND NOT EXISTS (
		   SELECT 1 FROM correlations q WHERE q.user_id = s.user_id AND q.natural_key = s.natural_key)`,
		`INSERT INTO correlation_members SELECT * FROM correlation_members_staging s
		 WHERE s.import_id = CAST(? AS UUID) AND EXISTS (
		   SELECT 1 FROM correlations c WHERE c.id = s.correlation_id AND c.import_id = s.import_id)`,
		`INSERT INTO workouts SELECT * FROM workouts_staging s
		 WHERE s.import_id = CAST(? AS UUID) AND NOT EXISTS (
		   SELECT 1 FROM workouts q WHERE q.user_id = s.user_id AND q.natural_key = s.natural_key)`,
		`INSERT INTO workout_events SELECT * FROM workout_events_staging s
		 WHERE s.import_id = CAST(? AS UUID) AND EXISTS (
		   SELECT 1 FROM workouts w WHERE w.id = s.workout_id AND w.import_id = s.import_id)`,
		`INSERT INTO workout_statistics SELECT * FROM workout_statistics_staging s
		 WHERE s.import_id = CAST(? AS UUID) AND EXISTS (
		   SELECT 1 FROM workouts w WHERE w.id = s.workout_id AND w.import_id = s.import_id)`,
		`INSERT INTO activity_summaries SELECT * FROM activity_summaries_staging s
		 WHERE s.import_id = CAST(? AS UUID) AND NOT EXISTS (
		   SELECT 1 FROM activity_summaries a WHERE a.user_id = s.user_id AND a.day = s.day)`,
		`INSERT INTO health_records SELECT * FROM health_records_staging WHERE import_id = CAST(? AS UUID)`,
	}
	for _, stmt := range inserts {
		if _, err := tx.ExecContext(ctx, stmt, importID); err != nil {
			return 0, fmt.Errorf("duckdb commit insert: %w", err)
		}
	}
	var written int64
	if err := tx.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM quantity_samples WHERE import_id = CAST(? AS UUID)) +
			(SELECT COUNT(*) FROM category_samples WHERE import_id = CAST(? AS UUID)) +
			(SELECT COUNT(*) FROM correlations WHERE import_id = CAST(? AS UUID)) +
			(SELECT COUNT(*) FROM workouts WHERE import_id = CAST(? AS UUID)) +
			(SELECT COUNT(*) FROM activity_summaries WHERE import_id = CAST(? AS UUID))`,
		importID, importID, importID, importID, importID).Scan(&written); err != nil {
		return 0, fmt.Errorf("duckdb count written: %w", err)
	}
	if err := deleteStaging(ctx, tx, importID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("duckdb commit facts: %w", err)
	}
	if err := db.reorder(ctx); err != nil {
		db.log.Warn("duckdb reorder failed", "import_id", importID, "error_code", "reorder_failed")
	}
	db.log.Info("duckdb commit", "import_id", importID, "user_id", userID, "mode", mode, "rows_written", written)
	return written, nil
}

// ClearStaging removes leftover staging rows after a crash.
func (db *DB) ClearStaging(ctx context.Context) error {
	for _, table := range stagingTables {
		if _, err := db.sql.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("duckdb clear %s: %w", table, err)
		}
	}
	db.log.Info("duckdb staging cleared")
	return nil
}

// DeleteStaging removes one import from the staging tables.
func (db *DB) DeleteStaging(ctx context.Context, importID string) error {
	return deleteStaging(ctx, db.sql, importID)
}

func deleteStaging(ctx context.Context, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, importID string) error {
	for _, table := range stagingTables {
		if _, err := exec.ExecContext(ctx, "DELETE FROM "+table+" WHERE import_id = CAST(? AS UUID)", importID); err != nil {
			return fmt.Errorf("duckdb delete staging %s: %w", table, err)
		}
	}
	return nil
}

var stagingTables = []string{
	"quantity_samples_staging",
	"category_samples_staging",
	"correlations_staging",
	"correlation_members_staging",
	"workouts_staging",
	"workout_events_staging",
	"workout_statistics_staging",
	"activity_summaries_staging",
	"health_records_staging",
}

func (db *DB) reorder(ctx context.Context) error {
	steps := []string{
		`CREATE OR REPLACE TABLE quantity_samples AS SELECT * FROM quantity_samples ORDER BY user_id, type, start_utc`,
		`CREATE INDEX quantity_user_type_start ON quantity_samples (user_id, type, start_utc)`,
		`CREATE OR REPLACE TABLE category_samples AS SELECT * FROM category_samples ORDER BY user_id, type, start_utc`,
		`CREATE INDEX category_user_type_start ON category_samples (user_id, type, start_utc)`,
		`CREATE OR REPLACE TABLE workouts AS SELECT * FROM workouts ORDER BY user_id, start_utc`,
		`CREATE INDEX workouts_user_start ON workouts (user_id, start_utc)`,
		`CREATE OR REPLACE TABLE activity_summaries AS SELECT * FROM activity_summaries ORDER BY user_id, day`,
		`CREATE INDEX activity_user_day ON activity_summaries (user_id, day)`,
	}
	for _, stmt := range steps {
		if _, err := db.sql.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// CountFacts returns the number of stored top-level facts for a user.
func (db *DB) CountFacts(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := db.sql.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM quantity_samples WHERE user_id = CAST(? AS UUID)) +
			(SELECT COUNT(*) FROM category_samples WHERE user_id = CAST(? AS UUID)) +
			(SELECT COUNT(*) FROM correlations WHERE user_id = CAST(? AS UUID)) +
			(SELECT COUNT(*) FROM workouts WHERE user_id = CAST(? AS UUID)) +
			(SELECT COUNT(*) FROM activity_summaries WHERE user_id = CAST(? AS UUID))`,
		userID, userID, userID, userID, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("duckdb count facts: %w", err)
	}
	return n, nil
}

func ids(userID, importID string) (duckdb.UUID, duckdb.UUID, error) {
	user, err := parseUUID(userID)
	if err != nil {
		return duckdb.UUID{}, duckdb.UUID{}, fmt.Errorf("user id: %w", err)
	}
	imp, err := parseUUID(importID)
	if err != nil {
		return duckdb.UUID{}, duckdb.UUID{}, fmt.Errorf("import id: %w", err)
	}
	return user, imp, nil
}

func parseUUID(raw string) (duckdb.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return duckdb.UUID{}, err
	}
	return duckdb.UUID(id), nil
}

func emptyNil(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func f64(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func jsonMap(m map[string]string) any {
	if len(m) == 0 {
		return nil
	}
	body, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return json.RawMessage(body)
}

func timeOrNil(v *time.Time) any {
	if v == nil || v.IsZero() {
		return nil
	}
	return v.UTC()
}
