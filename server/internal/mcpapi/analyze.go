package mcpapi

import (
	"context"
	"sort"
	"time"

	"com.bestony.health-connect/internal/store/duckdb"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type summaryInput struct {
	From  string   `json:"from" jsonschema:"RFC3339 start, inclusive"`
	To    string   `json:"to" jsonschema:"RFC3339 end, exclusive"`
	Types []string `json:"types,omitempty" jsonschema:"optional HealthKit type identifiers, at most 12"`
}

type periodInput struct {
	From string `json:"from" jsonschema:"RFC3339 start, inclusive"`
	To   string `json:"to" jsonschema:"RFC3339 end, exclusive"`
}

type compareInput struct {
	Type    string      `json:"type" jsonschema:"HealthKit quantity type identifier"`
	Agg     string      `json:"agg" jsonschema:"sum, avg, min, or max"`
	PeriodA periodInput `json:"period_a" jsonschema:"first window"`
	PeriodB periodInput `json:"period_b" jsonschema:"second window"`
}

func (a *api) summarizeRange(ctx context.Context, _ *mcp.CallToolRequest, in summaryInput) (*mcp.CallToolResult, any, error) {
	out, err := a.call(ctx, "summarize_range", []any{"from", in.From, "to", in.To, "types", len(in.Types)}, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		from, to, err := parseRange(in.From, in.To)
		if err != nil {
			return nil, err
		}
		if err := window(from, to, maxSummaryWindow); err != nil {
			return nil, err
		}
		types, err := a.summaryTypes(ctx, userID, in.Types)
		if err != nil {
			return nil, err
		}
		prevFrom := from.Add(-to.Sub(from))
		quantities := []map[string]any{}
		categories := []map[string]any{}
		for _, typeName := range types {
			table, err := a.facts.SampleTable(ctx, userID, typeName)
			if err != nil {
				return nil, err
			}
			switch table {
			case "category":
				item, err := a.categorySummary(ctx, userID, typeName, from, to)
				if err != nil {
					return nil, err
				}
				categories = append(categories, item)
			default:
				item, err := a.quantitySummary(ctx, userID, typeName, from, to, prevFrom)
				if err != nil {
					return nil, err
				}
				quantities = append(quantities, item)
			}
		}
		workouts, err := a.workoutSummary(ctx, userID, from, to)
		if err != nil {
			return nil, err
		}
		activity, err := a.activitySummary(ctx, userID, from, to)
		if err != nil {
			return nil, err
		}
		out := map[string]any{
			"quantities": quantities,
			"categories": categories,
			"workouts":   workouts,
			"activity":   activity,
		}
		return out, fit(out)
	})
	return nil, out, err
}

func (a *api) summaryTypes(ctx context.Context, userID string, requested []string) ([]string, error) {
	if len(requested) > 0 {
		if len(requested) > maxTypes {
			return nil, fail("invalid_query", "The query is not valid.")
		}
		seen := map[string]bool{}
		var out []string
		for _, typeName := range requested {
			if typeName == "" || seen[typeName] {
				continue
			}
			seen[typeName] = true
			out = append(out, typeName)
		}
		if len(out) == 0 {
			return nil, fail("invalid_query", "The query is not valid.")
		}
		return out, nil
	}
	items, err := a.facts.Catalog(ctx, userID)
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Type < items[j].Type
		}
		return items[i].Count > items[j].Count
	})
	if len(items) > maxTypes {
		items = items[:maxTypes]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Type)
	}
	return out, nil
}

func (a *api) quantitySummary(ctx context.Context, userID, typeName string, from, to, prevFrom time.Time) (map[string]any, error) {
	stats, err := a.facts.QuantityStats(ctx, userID, typeName, from, to)
	if err != nil {
		return nil, err
	}
	prev, err := a.facts.QuantityStats(ctx, userID, typeName, prevFrom, from)
	if err != nil {
		return nil, err
	}
	days, err := a.facts.LocalDays(ctx, userID, typeName, from, to)
	if err != nil {
		return nil, err
	}
	gaps, truncated := gapDates(from, to, days)
	prevByUnit := map[string]duckdb.QuantityStat{}
	for _, stat := range prev {
		prevByUnit[stat.Unit] = stat
	}
	units := make([]map[string]any, 0, len(stats))
	for _, stat := range stats {
		item := map[string]any{
			"unit": stat.Unit, "count": stat.Count,
			"min": stat.Min, "max": stat.Max, "avg": stat.Avg,
		}
		if earlier, ok := prevByUnit[stat.Unit]; ok && earlier.Count > 0 {
			item["previous_avg"] = earlier.Avg
			if pct := percentChange(stat.Avg, earlier.Avg, earlier.Count); pct != nil {
				item["delta_pct"] = *pct
			}
		}
		units = append(units, item)
	}
	return map[string]any{
		"type": typeName, "units": units, "days_with_data": len(days),
		"gaps": gaps, "gaps_truncated": truncated,
	}, nil
}

func (a *api) categorySummary(ctx context.Context, userID, typeName string, from, to time.Time) (map[string]any, error) {
	counts, err := a.facts.CategoryCounts(ctx, userID, typeName, from, to, 5)
	if err != nil {
		return nil, err
	}
	values := make([]map[string]any, 0, len(counts))
	for _, count := range counts {
		values = append(values, map[string]any{"value": count.Value, "count": count.Count})
	}
	return map[string]any{"type": typeName, "values": values}, nil
}

func (a *api) workoutSummary(ctx context.Context, userID string, from, to time.Time) (map[string]any, error) {
	rows, err := a.allWorkouts(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	type group struct {
		count    int
		duration float64
	}
	byType := map[string]*group{}
	var duration float64
	for _, row := range rows {
		item := byType[row.ActivityType]
		if item == nil {
			item = &group{}
			byType[row.ActivityType] = item
		}
		item.count++
		if row.DurationSec != nil {
			item.duration += *row.DurationSec
			duration += *row.DurationSec
		}
	}
	names := make([]string, 0, len(byType))
	for name := range byType {
		names = append(names, name)
	}
	sort.Strings(names)
	groups := make([]map[string]any, 0, len(names))
	for _, name := range names {
		item := byType[name]
		groups = append(groups, map[string]any{
			"activity_type": name, "count": item.count, "duration_sec": item.duration,
		})
	}
	return map[string]any{"count": len(rows), "duration_sec": duration, "by_type": groups}, nil
}

func (a *api) allWorkouts(ctx context.Context, userID string, from, to time.Time) ([]duckdb.WorkoutRow, error) {
	var cursorAt *time.Time
	var cursorID string
	var out []duckdb.WorkoutRow
	for page := 0; page < maxWorkoutPages; page++ {
		rows, err := a.facts.Workouts(ctx, userID, from, to, pageSize, cursorAt, cursorID, "")
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if len(rows) < pageSize {
			return out, nil
		}
		last := rows[len(rows)-1]
		at := last.Start
		cursorAt = &at
		cursorID = last.ID
	}
	return nil, fail("result_too_large", "The result is too large. Use a shorter range or a coarser grain.")
}

func (a *api) activitySummary(ctx context.Context, userID string, from, to time.Time) (map[string]any, error) {
	rows, err := a.facts.ActivitySummaries(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	var energy, energyGoal, exercise, exerciseGoal, stand, standGoal, move, moveGoal accumulator
	unit := ""
	for _, row := range rows {
		energy.add(row.ActiveEnergy)
		energyGoal.add(row.ActiveEnergyGoal)
		exercise.add(row.ExerciseMinutes)
		exerciseGoal.add(row.ExerciseGoal)
		stand.add(row.StandHours)
		standGoal.add(row.StandGoal)
		move.add(row.MoveMinutes)
		moveGoal.add(row.MoveGoal)
		if unit == "" {
			unit = row.ActiveEnergyUnit
		}
	}
	out := map[string]any{
		"days":                   len(rows),
		"active_energy_avg":      energy.mean(),
		"active_energy_goal_avg": energyGoal.mean(),
		"exercise_minutes_avg":   exercise.mean(),
		"exercise_goal_avg":      exerciseGoal.mean(),
		"stand_hours_avg":        stand.mean(),
		"stand_goal_avg":         standGoal.mean(),
		"move_minutes_avg":       move.mean(),
		"move_goal_avg":          moveGoal.mean(),
	}
	if unit != "" {
		out["active_energy_unit"] = unit
	}
	return out, nil
}

type accumulator struct {
	sum float64
	n   int
}

func (a *accumulator) add(value *float64) {
	if value == nil {
		return
	}
	a.sum += *value
	a.n++
}

func (a accumulator) mean() any {
	if a.n == 0 {
		return nil
	}
	return a.sum / float64(a.n)
}

func (a *api) comparePeriods(ctx context.Context, _ *mcp.CallToolRequest, in compareInput) (*mcp.CallToolResult, any, error) {
	out, err := a.call(ctx, "compare_periods", []any{"type", in.Type, "agg", in.Agg}, func() (any, error) {
		userID, err := userFrom(ctx)
		if err != nil {
			return nil, err
		}
		if in.Type == "" || !validAgg(in.Agg) {
			return nil, fail("invalid_query", "The query is not valid.")
		}
		aFrom, aTo, err := parseRange(in.PeriodA.From, in.PeriodA.To)
		if err != nil {
			return nil, err
		}
		bFrom, bTo, err := parseRange(in.PeriodB.From, in.PeriodB.To)
		if err != nil {
			return nil, err
		}
		if err := window(aFrom, aTo, maxSummaryWindow); err != nil {
			return nil, err
		}
		if err := window(bFrom, bTo, maxSummaryWindow); err != nil {
			return nil, err
		}
		table, err := a.facts.SampleTable(ctx, userID, in.Type)
		if err != nil {
			return nil, err
		}
		if table == "category" {
			return nil, fail("series_not_numeric", "This type is not numeric.")
		}
		left, err := a.facts.QuantityStats(ctx, userID, in.Type, aFrom, aTo)
		if err != nil {
			return nil, err
		}
		right, err := a.facts.QuantityStats(ctx, userID, in.Type, bFrom, bTo)
		if err != nil {
			return nil, err
		}
		rows := compareRows(in.Agg, left, right)
		out := map[string]any{"type": in.Type, "agg": in.Agg, "rows": rows}
		return out, fit(out)
	})
	return nil, out, err
}

func compareRows(agg string, left, right []duckdb.QuantityStat) []map[string]any {
	byUnit := map[string][2]*duckdb.QuantityStat{}
	for i := range left {
		pair := byUnit[left[i].Unit]
		pair[0] = &left[i]
		byUnit[left[i].Unit] = pair
	}
	for i := range right {
		pair := byUnit[right[i].Unit]
		pair[1] = &right[i]
		byUnit[right[i].Unit] = pair
	}
	units := make([]string, 0, len(byUnit))
	for unit := range byUnit {
		units = append(units, unit)
	}
	sort.Strings(units)
	rows := make([]map[string]any, 0, len(units))
	for _, unit := range units {
		pair := byUnit[unit]
		aValue, aCount := statValue(pair[0], agg)
		bValue, bCount := statValue(pair[1], agg)
		delta, pct := periodChange(aValue, bValue, aCount, bCount)
		row := map[string]any{
			"unit":     unit,
			"period_a": side(aValue, aCount),
			"period_b": side(bValue, bCount),
		}
		if delta != nil {
			row["delta"] = *delta
		}
		if pct != nil {
			row["delta_pct"] = *pct
		}
		rows = append(rows, row)
	}
	return rows
}

func side(value float64, count int64) map[string]any {
	item := map[string]any{"count": count}
	if count > 0 {
		item["value"] = value
	}
	return item
}

func statValue(stat *duckdb.QuantityStat, agg string) (float64, int64) {
	if stat == nil || stat.Count == 0 {
		return 0, 0
	}
	switch agg {
	case "sum":
		return stat.Sum, stat.Count
	case "min":
		return stat.Min, stat.Count
	case "max":
		return stat.Max, stat.Count
	default:
		return stat.Avg, stat.Count
	}
}
