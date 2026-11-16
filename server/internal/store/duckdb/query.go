package duckdb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

// CatalogEntry is one stored HealthKit type.
type CatalogEntry struct {
	Type  string
	Table string
	Count int64
	Units []string
	Start time.Time
	End   time.Time
}

// SeriesPoint is one aggregated bucket. LocalBucket is civil time, not UTC.
type SeriesPoint struct {
	LocalBucket time.Time
	Unit        string
	Value       float64
}

// Record is one stored sample.
type Record struct {
	Type        string
	Value       *float64
	ValueText   string
	Unit        string
	SourceName  string
	Start       time.Time
	End         time.Time
	TZOffsetMin int
	Metadata    json.RawMessage
	cursorKey   []byte
}

// CursorKey returns the key used to build the next page cursor.
func (r Record) CursorKey() []byte { return r.cursorKey }

// WorkoutRow is one workout plus its children.
type WorkoutRow struct {
	ID           string
	ActivityType string
	DurationSec  *float64
	Distance     *float64
	DistanceUnit string
	Energy       *float64
	EnergyUnit   string
	SourceName   string
	Start        time.Time
	End          time.Time
	TZOffsetMin  int
	Metadata     json.RawMessage
	Events       []WorkoutEventRow
	Statistics   []WorkoutStatRow
}

// WorkoutEventRow is one event inside a workout.
type WorkoutEventRow struct {
	Type        string
	At          time.Time
	TZOffsetMin int
	DurationSec *float64
}

// WorkoutStatRow is one workout aggregate.
type WorkoutStatRow struct {
	Type    string
	Sum     *float64
	Minimum *float64
	Maximum *float64
	Average *float64
	Unit    string
}

// ActivityRow is one daily activity summary.
type ActivityRow struct {
	Day              time.Time
	ActiveEnergy     *float64
	ActiveEnergyGoal *float64
	ActiveEnergyUnit string
	ExerciseMinutes  *float64
	ExerciseGoal     *float64
	StandHours       *float64
	StandGoal        *float64
	MoveMinutes      *float64
	MoveGoal         *float64
}

// Catalog lists types stored for the user.
func (db *DB) Catalog(ctx context.Context, userID string) ([]CatalogEntry, error) {
	quantities, err := db.catalogTable(ctx, userID, "quantity_samples", "quantity", true)
	if err != nil {
		return nil, err
	}
	categories, err := db.catalogTable(ctx, userID, "category_samples", "category", false)
	if err != nil {
		return nil, err
	}
	return append(quantities, categories...), nil
}

func (db *DB) catalogTable(ctx context.Context, userID, table, kind string, withUnit bool) ([]CatalogEntry, error) {
	query := `
		SELECT type, COUNT(*), MIN(start_utc), MAX(end_utc), CAST(NULL AS VARCHAR)
		FROM ` + table + `
		WHERE user_id = CAST(? AS UUID)
		GROUP BY type`
	if withUnit {
		query = `
		SELECT type, COUNT(*), MIN(start_utc), MAX(end_utc), unit
		FROM ` + table + `
		WHERE user_id = CAST(? AS UUID)
		GROUP BY type, unit`
	}
	rows, err := db.sql.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("duckdb catalog %s: %w", table, err)
	}
	defer rows.Close()
	byType := map[string]*CatalogEntry{}
	var order []string
	for rows.Next() {
		var entry CatalogEntry
		var unit sql.NullString
		entry.Table = kind
		if err := rows.Scan(&entry.Type, &entry.Count, &entry.Start, &entry.End, &unit); err != nil {
			return nil, fmt.Errorf("duckdb catalog scan: %w", err)
		}
		existing, ok := byType[entry.Type]
		if !ok {
			if unit.Valid && unit.String != "" {
				entry.Units = []string{unit.String}
			}
			byType[entry.Type] = &entry
			order = append(order, entry.Type)
			continue
		}
		existing.Count += entry.Count
		if entry.Start.Before(existing.Start) {
			existing.Start = entry.Start
		}
		if entry.End.After(existing.End) {
			existing.End = entry.End
		}
		if unit.Valid && unit.String != "" {
			existing.Units = append(existing.Units, unit.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]CatalogEntry, 0, len(order))
	for _, typ := range order {
		out = append(out, *byType[typ])
	}
	return out, nil
}

// SampleTable reports "quantity", "category", or "" for a type.
func (db *DB) SampleTable(ctx context.Context, userID, typeName string) (string, error) {
	var n int
	if err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM quantity_samples
		WHERE user_id = CAST(? AS UUID) AND type = ?`, userID, typeName).Scan(&n); err != nil {
		return "", fmt.Errorf("duckdb sample table: %w", err)
	}
	if n > 0 {
		return "quantity", nil
	}
	if err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM category_samples
		WHERE user_id = CAST(? AS UUID) AND type = ?`, userID, typeName).Scan(&n); err != nil {
		return "", fmt.Errorf("duckdb sample table: %w", err)
	}
	if n > 0 {
		return "category", nil
	}
	return "", nil
}

// Series aggregates one quantity type. grain is "hour" or "day". agg is sum, avg, min, or max.
func (db *DB) Series(ctx context.Context, userID, typeName, grain, agg string, from, to time.Time) ([]SeriesPoint, error) {
	if grain != "hour" && grain != "day" {
		return nil, fmt.Errorf("duckdb series grain")
	}
	aggExpr, ok := map[string]string{"sum": "sum(value)", "avg": "avg(value)", "min": "min(value)", "max": "max(value)"}[agg]
	if !ok {
		return nil, fmt.Errorf("duckdb series agg")
	}
	// grain is constrained to hour or day above, so it is safe to inline.
	rows, err := db.sql.QueryContext(ctx, `
		SELECT date_trunc('`+grain+`', start_utc + tz_offset_min * INTERVAL 1 MINUTE) AS bucket,
		       unit, `+aggExpr+`
		FROM quantity_samples
		WHERE user_id = CAST(? AS UUID) AND type = ? AND start_utc >= ? AND start_utc < ?
		GROUP BY 1, 2
		ORDER BY 1, 2`, userID, typeName, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("duckdb series: %w", err)
	}
	defer rows.Close()
	var out []SeriesPoint
	for rows.Next() {
		var point SeriesPoint
		if err := rows.Scan(&point.LocalBucket, &point.Unit, &point.Value); err != nil {
			return nil, fmt.Errorf("duckdb series scan: %w", err)
		}
		out = append(out, point)
	}
	return out, rows.Err()
}

// Records pages quantity or category samples. tableName must be one of the two fact tables.
func (db *DB) Records(ctx context.Context, userID, typeName, tableName string, from, to time.Time, limit int, cursorStart *time.Time, cursorKey []byte) ([]Record, error) {
	if tableName != "quantity_samples" && tableName != "category_samples" {
		return nil, fmt.Errorf("duckdb records table")
	}
	valueExpr := "CAST(NULL AS DOUBLE), value_text, CAST(NULL AS VARCHAR)"
	if tableName == "quantity_samples" {
		valueExpr = "value, CAST(NULL AS VARCHAR), unit"
	}
	hasCursor := 0
	var cursor time.Time
	if cursorStart != nil {
		hasCursor = 1
		cursor = cursorStart.UTC()
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT type, `+valueExpr+`, source_name, start_utc, end_utc, tz_offset_min, metadata, natural_key
		FROM `+tableName+`
		WHERE user_id = CAST(? AS UUID) AND type = ? AND start_utc >= ? AND start_utc < ?
		  AND (? = 0 OR start_utc > ? OR (start_utc = ? AND natural_key > ?))
		ORDER BY start_utc, natural_key
		LIMIT ?`, userID, typeName, from.UTC(), to.UTC(), hasCursor, cursor, cursor, cursorKey, limit)
	if err != nil {
		return nil, fmt.Errorf("duckdb records: %w", err)
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var rec Record
		var value sql.NullFloat64
		var valueText, unit sql.NullString
		var meta any
		var key []byte
		if err := rows.Scan(&rec.Type, &value, &valueText, &unit, &rec.SourceName, &rec.Start, &rec.End, &rec.TZOffsetMin, &meta, &key); err != nil {
			return nil, fmt.Errorf("duckdb records scan: %w", err)
		}
		if value.Valid {
			rec.Value = &value.Float64
		}
		rec.ValueText = valueText.String
		rec.Unit = unit.String
		rec.Metadata = rawJSON(meta)
		rec.cursorKey = key
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Workouts pages workouts and loads their events and statistics.
// An empty activityType selects every activity type.
func (db *DB) Workouts(ctx context.Context, userID string, from, to time.Time, limit int, cursorStart *time.Time, cursorID, activityType string) ([]WorkoutRow, error) {
	hasCursor := 0
	var cursor time.Time
	if cursorStart != nil {
		hasCursor = 1
		cursor = cursorStart.UTC()
	}
	if cursorID == "" {
		cursorID = "00000000-0000-0000-0000-000000000000"
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, activity_type, duration_sec, distance, distance_unit, energy, energy_unit,
		       source_name, start_utc, end_utc, tz_offset_min, metadata
		FROM workouts
		WHERE user_id = CAST(? AS UUID) AND start_utc >= ? AND start_utc < ?
		  AND (? = '' OR activity_type = ?)
		  AND (? = 0 OR start_utc > ? OR (start_utc = ? AND id > CAST(? AS UUID)))
		ORDER BY start_utc, id
		LIMIT ?`, userID, from.UTC(), to.UTC(), activityType, activityType, hasCursor, cursor, cursor, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("duckdb workouts: %w", err)
	}
	defer rows.Close()
	var out []WorkoutRow
	for rows.Next() {
		var row WorkoutRow
		var id duckdb.UUID
		var duration, distance, energy sql.NullFloat64
		var distanceUnit, energyUnit sql.NullString
		var meta any
		if err := rows.Scan(&id, &row.ActivityType, &duration, &distance, &distanceUnit, &energy, &energyUnit, &row.SourceName, &row.Start, &row.End, &row.TZOffsetMin, &meta); err != nil {
			return nil, fmt.Errorf("duckdb workout scan: %w", err)
		}
		row.ID = id.String()
		row.DurationSec = nullFloat(duration)
		row.Distance = nullFloat(distance)
		row.Energy = nullFloat(energy)
		row.DistanceUnit = distanceUnit.String
		row.EnergyUnit = energyUnit.String
		row.Metadata = rawJSON(meta)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]any, len(out))
	index := map[string]int{}
	for i, row := range out {
		ids[i] = row.ID
		index[row.ID] = i
		out[i].Events = []WorkoutEventRow{}
		out[i].Statistics = []WorkoutStatRow{}
	}
	in := strings.TrimRight(strings.Repeat("CAST(? AS UUID),", len(ids)), ",")
	eventRows, err := db.sql.QueryContext(ctx, `
		SELECT workout_id, event_type, event_utc, tz_offset_min, duration_sec
		FROM workout_events WHERE workout_id IN (`+in+`) ORDER BY event_utc`, ids...)
	if err != nil {
		return nil, fmt.Errorf("duckdb workout events: %w", err)
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var id duckdb.UUID
		var event WorkoutEventRow
		var duration sql.NullFloat64
		if err := eventRows.Scan(&id, &event.Type, &event.At, &event.TZOffsetMin, &duration); err != nil {
			return nil, err
		}
		event.DurationSec = nullFloat(duration)
		out[index[id.String()]].Events = append(out[index[id.String()]].Events, event)
	}
	if err := eventRows.Err(); err != nil {
		return nil, err
	}
	statRows, err := db.sql.QueryContext(ctx, `
		SELECT workout_id, type, sum_value, minimum, maximum, average, unit
		FROM workout_statistics WHERE workout_id IN (`+in+`)`, ids...)
	if err != nil {
		return nil, fmt.Errorf("duckdb workout stats: %w", err)
	}
	defer statRows.Close()
	for statRows.Next() {
		var id duckdb.UUID
		var stat WorkoutStatRow
		var sum, minimum, maximum, average sql.NullFloat64
		var unit sql.NullString
		if err := statRows.Scan(&id, &stat.Type, &sum, &minimum, &maximum, &average, &unit); err != nil {
			return nil, err
		}
		stat.Sum = nullFloat(sum)
		stat.Minimum = nullFloat(minimum)
		stat.Maximum = nullFloat(maximum)
		stat.Average = nullFloat(average)
		stat.Unit = unit.String
		out[index[id.String()]].Statistics = append(out[index[id.String()]].Statistics, stat)
	}
	return out, statRows.Err()
}

// ActivitySummaries returns daily rows in [from, to).
func (db *DB) ActivitySummaries(ctx context.Context, userID string, from, to time.Time) ([]ActivityRow, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT day, active_energy, active_energy_goal, active_energy_unit,
		       exercise_minutes, exercise_goal, stand_hours, stand_goal, move_minutes, move_goal
		FROM activity_summaries
		WHERE user_id = CAST(? AS UUID) AND day >= ? AND day < ?
		ORDER BY day`, userID, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("duckdb activity: %w", err)
	}
	defer rows.Close()
	var out []ActivityRow
	for rows.Next() {
		var row ActivityRow
		var energy, energyGoal, exercise, exerciseGoal, stand, standGoal, move, moveGoal sql.NullFloat64
		var unit sql.NullString
		if err := rows.Scan(&row.Day, &energy, &energyGoal, &unit, &exercise, &exerciseGoal, &stand, &standGoal, &move, &moveGoal); err != nil {
			return nil, fmt.Errorf("duckdb activity scan: %w", err)
		}
		row.ActiveEnergy = nullFloat(energy)
		row.ActiveEnergyGoal = nullFloat(energyGoal)
		row.ActiveEnergyUnit = unit.String
		row.ExerciseMinutes = nullFloat(exercise)
		row.ExerciseGoal = nullFloat(exerciseGoal)
		row.StandHours = nullFloat(stand)
		row.StandGoal = nullFloat(standGoal)
		row.MoveMinutes = nullFloat(move)
		row.MoveGoal = nullFloat(moveGoal)
		out = append(out, row)
	}
	return out, rows.Err()
}

// Overview counts workouts, activity days, and clinical records for one user.
type Overview struct {
	Workouts      int64
	ActivityDays  int64
	HealthRecords int64
}

// Overview returns counts that are not part of the sample catalog.
func (db *DB) Overview(ctx context.Context, userID string) (Overview, error) {
	var out Overview
	err := db.sql.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM workouts WHERE user_id = CAST(? AS UUID)),
			(SELECT COUNT(*) FROM activity_summaries WHERE user_id = CAST(? AS UUID)),
			(SELECT COUNT(*) FROM health_records WHERE user_id = CAST(? AS UUID))`,
		userID, userID, userID).Scan(&out.Workouts, &out.ActivityDays, &out.HealthRecords)
	if err != nil {
		return Overview{}, fmt.Errorf("duckdb overview: %w", err)
	}
	return out, nil
}

// QuantityStat is one unit's aggregate over a half-open window.
type QuantityStat struct {
	Unit  string
	Count int64
	Min   float64
	Max   float64
	Avg   float64
	Sum   float64
	Days  int64
}

// QuantityStats aggregates one numeric type in [from, to).
func (db *DB) QuantityStats(ctx context.Context, userID, typeName string, from, to time.Time) ([]QuantityStat, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT unit, COUNT(*), MIN(value), MAX(value), AVG(value), SUM(value),
		       COUNT(DISTINCT date_trunc('day', start_utc + tz_offset_min * INTERVAL 1 MINUTE))
		FROM quantity_samples
		WHERE user_id = CAST(? AS UUID) AND type = ? AND start_utc >= ? AND start_utc < ?
		GROUP BY unit
		ORDER BY unit`, userID, typeName, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("duckdb quantity stats: %w", err)
	}
	defer rows.Close()
	var out []QuantityStat
	for rows.Next() {
		var stat QuantityStat
		if err := rows.Scan(&stat.Unit, &stat.Count, &stat.Min, &stat.Max, &stat.Avg, &stat.Sum, &stat.Days); err != nil {
			return nil, fmt.Errorf("duckdb quantity stats scan: %w", err)
		}
		out = append(out, stat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("duckdb quantity stats: %w", err)
	}
	return out, nil
}

// CategoryCount is one category value and its count.
type CategoryCount struct {
	Value string
	Count int64
}

// CategoryCounts returns the most common values for one category type.
func (db *DB) CategoryCounts(ctx context.Context, userID, typeName string, from, to time.Time, limit int) ([]CategoryCount, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT value_text, COUNT(*)
		FROM category_samples
		WHERE user_id = CAST(? AS UUID) AND type = ? AND start_utc >= ? AND start_utc < ?
		GROUP BY value_text
		ORDER BY COUNT(*) DESC, value_text
		LIMIT ?`, userID, typeName, from.UTC(), to.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("duckdb category counts: %w", err)
	}
	defer rows.Close()
	var out []CategoryCount
	for rows.Next() {
		var item CategoryCount
		if err := rows.Scan(&item.Value, &item.Count); err != nil {
			return nil, fmt.Errorf("duckdb category counts scan: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("duckdb category counts: %w", err)
	}
	return out, nil
}

// LocalDays returns distinct local civil days that have a numeric sample.
func (db *DB) LocalDays(ctx context.Context, userID, typeName string, from, to time.Time) ([]time.Time, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT DISTINCT date_trunc('day', start_utc + tz_offset_min * INTERVAL 1 MINUTE)
		FROM quantity_samples
		WHERE user_id = CAST(? AS UUID) AND type = ? AND start_utc >= ? AND start_utc < ?
		ORDER BY 1`, userID, typeName, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("duckdb local days: %w", err)
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var day time.Time
		if err := rows.Scan(&day); err != nil {
			return nil, fmt.Errorf("duckdb local days scan: %w", err)
		}
		out = append(out, day)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("duckdb local days: %w", err)
	}
	return out, nil
}

// HealthRecordRow is one stored clinical record.
type HealthRecordRow struct {
	Kind           string
	TypeIdentifier string
	Start          *time.Time
	End            *time.Time
	TZOffsetMin    *int
	Source         string
	Device         string
	Payload        json.RawMessage
}

// HealthRecords lists clinical records in [from, to). An empty typeIdentifier selects every type.
// Records with no start time are omitted.
func (db *DB) HealthRecords(ctx context.Context, userID, typeIdentifier string, from, to time.Time, limit int) ([]HealthRecordRow, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT kind, type_identifier, start_utc, end_utc, tz_offset_min, source_name, device, payload
		FROM health_records
		WHERE user_id = CAST(? AS UUID)
		  AND (? = '' OR type_identifier = ?)
		  AND start_utc >= ? AND start_utc < ?
		ORDER BY start_utc, type_identifier
		LIMIT ?`, userID, typeIdentifier, typeIdentifier, from.UTC(), to.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("duckdb health records: %w", err)
	}
	defer rows.Close()
	var out []HealthRecordRow
	for rows.Next() {
		var row HealthRecordRow
		var start, end sql.NullTime
		var offset sql.NullInt64
		var source, device sql.NullString
		var payload any
		if err := rows.Scan(&row.Kind, &row.TypeIdentifier, &start, &end, &offset, &source, &device, &payload); err != nil {
			return nil, fmt.Errorf("duckdb health records scan: %w", err)
		}
		if start.Valid {
			row.Start = &start.Time
		}
		if end.Valid {
			row.End = &end.Time
		}
		if offset.Valid {
			n := int(offset.Int64)
			row.TZOffsetMin = &n
		}
		row.Source = source.String
		row.Device = device.String
		row.Payload = rawJSON(payload)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("duckdb health records: %w", err)
	}
	return out, nil
}

// DeleteUser removes one user's facts.
func (db *DB) DeleteUser(ctx context.Context, userID string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("duckdb begin delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
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
			return fmt.Errorf("duckdb delete user: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("duckdb commit delete: %w", err)
	}
	db.log.Info("duckdb user data deleted", "user_id", userID)
	return nil
}

func nullFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func rawJSON(v any) json.RawMessage {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		if len(t) == 0 {
			return nil
		}
		return append(json.RawMessage(nil), t...)
	case string:
		if t == "" {
			return nil
		}
		return json.RawMessage(t)
	default:
		body, err := json.Marshal(t)
		if err != nil {
			return nil
		}
		return body
	}
}

// EncodeCursor builds an opaque page token from a timestamp and a blob or id.
func EncodeCursor(at time.Time, key []byte) string {
	return hex.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano))) + "." + hex.EncodeToString(key)
}

// DecodeCursor parses a token from EncodeCursor.
func DecodeCursor(raw string) (time.Time, []byte, error) {
	stampHex, keyHex, ok := strings.Cut(raw, ".")
	if !ok {
		return time.Time{}, nil, fmt.Errorf("cursor")
	}
	stamp, err := hex.DecodeString(stampHex)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, string(stamp))
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("cursor")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("cursor")
	}
	return at, key, nil
}
