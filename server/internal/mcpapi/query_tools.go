package mcpapi

import (
	"context"
	"time"

	"com.bestony.health-connect/internal/store/duckdb"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type seriesInput struct {
	Type  string `json:"type" jsonschema:"HealthKit quantity type identifier"`
	From  string `json:"from" jsonschema:"RFC3339 start, inclusive"`
	To    string `json:"to" jsonschema:"RFC3339 end, exclusive"`
	Grain string `json:"grain" jsonschema:"hour or day"`
	Agg   string `json:"agg" jsonschema:"sum, avg, min, or max"`
}

type recordsInput struct {
	Type   string `json:"type" jsonschema:"HealthKit type identifier"`
	From   string `json:"from" jsonschema:"RFC3339 start, inclusive"`
	To     string `json:"to" jsonschema:"RFC3339 end, exclusive"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50, maximum 200"`
	Cursor string `json:"cursor,omitempty" jsonschema:"cursor from the previous page"`
}

type workoutsInput struct {
	From          string `json:"from" jsonschema:"RFC3339 start, inclusive"`
	To            string `json:"to" jsonschema:"RFC3339 end, exclusive"`
	ActivityType  string `json:"activity_type,omitempty" jsonschema:"optional HKWorkoutActivityType filter"`
	Limit         int    `json:"limit,omitempty" jsonschema:"page size, default 20, maximum 100"`
	Cursor        string `json:"cursor,omitempty" jsonschema:"cursor from the previous page"`
	IncludeEvents bool   `json:"include_events,omitempty" jsonschema:"include workout events"`
}

type activityInput struct {
	From string `json:"from" jsonschema:"start date, inclusive, YYYY-MM-DD"`
	To   string `json:"to" jsonschema:"end date, exclusive, YYYY-MM-DD"`
}

type clinicalInput struct {
	TypeIdentifier string `json:"type_identifier,omitempty" jsonschema:"optional clinical type identifier"`
	From           string `json:"from" jsonschema:"RFC3339 start, inclusive"`
	To             string `json:"to" jsonschema:"RFC3339 end, exclusive"`
	Limit          int    `json:"limit,omitempty" jsonschema:"page size, default 5, maximum 20"`
}

func (a *api) listCatalog(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	out, err := a.call(ctx, "list_catalog", nil, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		items, err := a.facts.Catalog(ctx, userID)
		if err != nil {
			return nil, err
		}
		counts, err := a.facts.Overview(ctx, userID)
		if err != nil {
			return nil, err
		}
		types := make([]map[string]any, 0, len(items))
		for _, item := range items {
			units := item.Units
			if units == nil {
				units = []string{}
			}
			types = append(types, map[string]any{
				"type": item.Type, "table": item.Table, "count": item.Count, "units": units,
				"start": formatTime(item.Start), "end": formatTime(item.End),
			})
		}
		out := map[string]any{
			"types": types, "workouts": counts.Workouts,
			"activity_days": counts.ActivityDays, "health_records": counts.HealthRecords,
		}
		return out, fit(out)
	})
	return nil, out, err
}

func (a *api) getProfile(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	out, err := a.call(ctx, "get_profile", nil, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		items, err := a.meta.ListCharacteristics(ctx, userID)
		if err != nil {
			return nil, err
		}
		attrs := make([]map[string]string, 0, len(items))
		for _, item := range items {
			attrs = append(attrs, map[string]string{"key": item.Key, "value": item.Value})
		}
		out := map[string]any{"attributes": attrs}
		a.log.Debug("mcp", "tool", "get_profile", "count", len(attrs))
		return out, fit(out)
	})
	return nil, out, err
}

func (a *api) querySeries(ctx context.Context, _ *mcp.CallToolRequest, in seriesInput) (*mcp.CallToolResult, any, error) {
	out, err := a.call(ctx, "query_series", []any{"type", in.Type, "grain", in.Grain, "agg", in.Agg, "from", in.From, "to", in.To}, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		from, to, err := a.seriesWindow(in)
		if err != nil {
			return nil, err
		}
		table, err := a.facts.SampleTable(ctx, userID, in.Type)
		if err != nil {
			return nil, err
		}
		if table == "category" {
			return nil, fail("series_not_numeric", "This type is not numeric.")
		}
		points, err := a.facts.Series(ctx, userID, in.Type, in.Grain, in.Agg, from, to)
		if err != nil {
			return nil, err
		}
		if len(points) > maxSeriesPoints {
			return nil, fail("result_too_large", "The result is too large. Use a shorter range or a coarser grain.")
		}
		outPoints := make([]map[string]any, 0, len(points))
		for _, point := range points {
			outPoints = append(outPoints, map[string]any{
				"local_bucket": formatCivil(point.LocalBucket),
				"unit":         point.Unit,
				"value":        point.Value,
			})
		}
		out := map[string]any{"points": outPoints}
		return out, fit(out)
	})
	return nil, out, err
}

func (a *api) seriesWindow(in seriesInput) (time.Time, time.Time, error) {
	if in.Type == "" || (in.Grain != "hour" && in.Grain != "day") || !validAgg(in.Agg) {
		return time.Time{}, time.Time{}, fail("invalid_query", "The query is not valid.")
	}
	from, to, err := parseRange(in.From, in.To)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	max := maxDayWindow
	if in.Grain == "hour" {
		max = maxHourWindow
	}
	if err := window(from, to, max); err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}

func (a *api) queryRecords(ctx context.Context, _ *mcp.CallToolRequest, in recordsInput) (*mcp.CallToolResult, any, error) {
	out, err := a.call(ctx, "query_records", []any{"type", in.Type, "from", in.From, "to", in.To, "limit", clampLimit(in.Limit, defaultRecords, maxRecords)}, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		if in.Type == "" {
			return nil, fail("invalid_query", "The query is not valid.")
		}
		from, to, err := parseRange(in.From, in.To)
		if err != nil {
			return nil, err
		}
		if err := ordered(from, to); err != nil {
			return nil, err
		}
		limit := clampLimit(in.Limit, defaultRecords, maxRecords)
		var cursorAt *time.Time
		var cursorKey []byte
		if in.Cursor != "" {
			at, key, err := duckdb.DecodeCursor(in.Cursor)
			if err != nil {
				return nil, fail("invalid_query", "The cursor is not valid.")
			}
			cursorAt = &at
			cursorKey = key
		}
		table, err := a.facts.SampleTable(ctx, userID, in.Type)
		if err != nil {
			return nil, err
		}
		records := []map[string]any{}
		var next string
		if table != "" {
			tableName := "quantity_samples"
			if table == "category" {
				tableName = "category_samples"
			}
			rows, err := a.facts.Records(ctx, userID, in.Type, tableName, from, to, limit, cursorAt, cursorKey)
			if err != nil {
				return nil, err
			}
			records = make([]map[string]any, 0, len(rows))
			for _, rec := range rows {
				item := map[string]any{
					"type": rec.Type, "source_name": rec.SourceName,
					"start": formatTime(rec.Start), "end": formatTime(rec.End),
					"tz_offset_min": rec.TZOffsetMin,
				}
				if rec.Value != nil {
					item["value"] = *rec.Value
					item["unit"] = rec.Unit
				} else {
					item["value_text"] = rec.ValueText
				}
				if len(rec.Metadata) > 0 {
					item["metadata"] = string(rec.Metadata)
				}
				records = append(records, item)
			}
			if len(rows) == limit {
				last := rows[len(rows)-1]
				next = duckdb.EncodeCursor(last.Start, last.CursorKey())
			}
		}
		out := map[string]any{"records": records}
		if next != "" {
			out["next_cursor"] = next
		}
		return out, fit(out)
	})
	return nil, out, err
}

func (a *api) queryWorkouts(ctx context.Context, _ *mcp.CallToolRequest, in workoutsInput) (*mcp.CallToolResult, any, error) {
	limit := clampLimit(in.Limit, defaultWorkouts, maxWorkouts)
	out, err := a.call(ctx, "query_workouts", []any{"activity_type", in.ActivityType, "from", in.From, "to", in.To, "limit", limit}, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		from, to, err := parseRange(in.From, in.To)
		if err != nil {
			return nil, err
		}
		if err := ordered(from, to); err != nil {
			return nil, err
		}
		var cursorAt *time.Time
		var cursorID string
		if in.Cursor != "" {
			at, key, err := duckdb.DecodeCursor(in.Cursor)
			if err != nil {
				return nil, fail("invalid_query", "The cursor is not valid.")
			}
			cursorAt = &at
			cursorID = string(key)
		}
		rows, err := a.facts.Workouts(ctx, userID, from, to, limit, cursorAt, cursorID, in.ActivityType)
		if err != nil {
			return nil, err
		}
		workouts := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			item := map[string]any{
				"id": row.ID, "activity_type": row.ActivityType, "source_name": row.SourceName,
				"start": formatTime(row.Start), "end": formatTime(row.End),
				"tz_offset_min": row.TZOffsetMin,
			}
			putFloat(item, "duration_sec", row.DurationSec)
			if row.Distance != nil {
				item["distance"] = *row.Distance
				item["distance_unit"] = row.DistanceUnit
			}
			if row.Energy != nil {
				item["energy"] = *row.Energy
				item["energy_unit"] = row.EnergyUnit
			}
			stats := make([]map[string]any, 0, len(row.Statistics))
			for _, stat := range row.Statistics {
				one := map[string]any{"type": stat.Type, "unit": stat.Unit}
				putFloat(one, "sum", stat.Sum)
				putFloat(one, "minimum", stat.Minimum)
				putFloat(one, "maximum", stat.Maximum)
				putFloat(one, "average", stat.Average)
				stats = append(stats, one)
			}
			item["statistics"] = stats
			if in.IncludeEvents {
				events := make([]map[string]any, 0, len(row.Events))
				for _, event := range row.Events {
					one := map[string]any{
						"type": event.Type, "at": formatTime(event.At), "tz_offset_min": event.TZOffsetMin,
					}
					putFloat(one, "duration_sec", event.DurationSec)
					events = append(events, one)
				}
				item["events"] = events
			}
			workouts = append(workouts, item)
		}
		out := map[string]any{"workouts": workouts}
		if len(rows) == limit && len(rows) > 0 {
			last := rows[len(rows)-1]
			out["next_cursor"] = duckdb.EncodeCursor(last.Start, []byte(last.ID))
		}
		return out, fit(out)
	})
	return nil, out, err
}

func (a *api) queryActivity(ctx context.Context, _ *mcp.CallToolRequest, in activityInput) (*mcp.CallToolResult, any, error) {
	out, err := a.call(ctx, "query_activity", []any{"from", in.From, "to", in.To}, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		from, err := parseDay(in.From)
		if err != nil {
			return nil, fail("invalid_query", "The query is not valid.")
		}
		to, err := parseDay(in.To)
		if err != nil {
			return nil, fail("invalid_query", "The query is not valid.")
		}
		if err := window(from, to, maxActivityDays*24*time.Hour); err != nil {
			return nil, err
		}
		rows, err := a.facts.ActivitySummaries(ctx, userID, from, to)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			item := map[string]any{"day": row.Day.UTC().Format("2006-01-02"), "active_energy_unit": row.ActiveEnergyUnit}
			putFloat(item, "active_energy", row.ActiveEnergy)
			putFloat(item, "active_energy_goal", row.ActiveEnergyGoal)
			putFloat(item, "exercise_minutes", row.ExerciseMinutes)
			putFloat(item, "exercise_goal", row.ExerciseGoal)
			putFloat(item, "stand_hours", row.StandHours)
			putFloat(item, "stand_goal", row.StandGoal)
			putFloat(item, "move_minutes", row.MoveMinutes)
			putFloat(item, "move_goal", row.MoveGoal)
			items = append(items, item)
		}
		out := map[string]any{"activity_summaries": items}
		return out, fit(out)
	})
	return nil, out, err
}

func (a *api) queryHealthRecords(ctx context.Context, _ *mcp.CallToolRequest, in clinicalInput) (*mcp.CallToolResult, any, error) {
	limit := clampLimit(in.Limit, defaultClinical, maxClinical)
	out, err := a.call(ctx, "query_health_records", []any{"type_identifier", in.TypeIdentifier, "from", in.From, "to", in.To, "limit", limit}, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		from, to, err := parseRange(in.From, in.To)
		if err != nil {
			return nil, err
		}
		if err := ordered(from, to); err != nil {
			return nil, err
		}
		rows, err := a.facts.HealthRecords(ctx, userID, in.TypeIdentifier, from, to, limit)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			payload := string(row.Payload)
			truncated := false
			if len(payload) > payloadLimit {
				payload = payload[:payloadLimit]
				truncated = true
			}
			item := map[string]any{
				"kind": row.Kind, "type_identifier": row.TypeIdentifier,
				"source": row.Source, "device": row.Device,
				"payload": payload, "truncated": truncated,
			}
			if row.Start != nil {
				item["start"] = formatTime(*row.Start)
			}
			if row.End != nil {
				item["end"] = formatTime(*row.End)
			}
			if row.TZOffsetMin != nil {
				item["tz_offset_min"] = *row.TZOffsetMin
			}
			items = append(items, item)
		}
		out := map[string]any{"records": items}
		return out, fit(out)
	})
	return nil, out, err
}

func parseRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	from, err := parseTime(fromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fail("invalid_query", "The query is not valid.")
	}
	to, err := parseTime(toRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fail("invalid_query", "The query is not valid.")
	}
	return from, to, nil
}

func putFloat(item map[string]any, key string, value *float64) {
	if value != nil {
		item[key] = *value
	}
}
