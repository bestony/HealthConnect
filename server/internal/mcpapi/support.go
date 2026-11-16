package mcpapi

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	payloadLimit     = 8 << 10
	maxTypes         = 12
	maxGaps          = 14
	maxWorkoutPages  = 50
	pageSize         = 100
	defaultRecords   = 50
	maxRecords       = 200
	defaultWorkouts  = 20
	maxWorkouts      = 100
	defaultClinical  = 5
	maxClinical      = 20
	maxHourWindow    = 14 * 24 * time.Hour
	maxDayWindow     = 400 * 24 * time.Hour
	maxSummaryWindow = 366 * 24 * time.Hour
	maxActivityDays  = 400
)

var (
	maxResultBytes  = 256 << 10
	maxSeriesPoints = 800
)

type toolError struct {
	Code string
	Msg  string
}

func (e *toolError) Error() string {
	body, err := json.Marshal(struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}{e.Code, e.Msg})
	if err != nil {
		return `{"error":"internal","message":"The query failed."}`
	}
	return string(body)
}

func fail(code, msg string) error {
	return &toolError{Code: code, Msg: msg}
}

func fit(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fail("internal", "The query failed.")
	}
	if len(body) > maxResultBytes {
		return fail("result_too_large", "The result is too large. Use a shorter range or a coarser grain.")
	}
	return nil
}

func clampLimit(n, fallback, max int) int {
	if n <= 0 {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}

func validAgg(agg string) bool {
	switch agg {
	case "sum", "avg", "min", "max":
		return true
	default:
		return false
	}
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
}

func parseDay(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 10 {
		return time.Parse("2006-01-02", raw)
	}
	t, err := parseTime(raw)
	if err != nil {
		return time.Time{}, err
	}
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

func ordered(from, to time.Time) error {
	if !from.Before(to) {
		return fail("invalid_query", "The time range is not valid.")
	}
	return nil
}

func window(from, to time.Time, max time.Duration) error {
	if err := ordered(from, to); err != nil {
		return err
	}
	if to.Sub(from) > max {
		return fail("invalid_query", "The time range is too long.")
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func formatCivil(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05")
}

func gapDates(from, to time.Time, present []time.Time) (dates []string, truncated bool) {
	have := map[string]bool{}
	for _, day := range present {
		have[day.UTC().Format("2006-01-02")] = true
	}
	start := time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.UTC().Year(), to.UTC().Month(), to.UTC().Day(), 0, 0, 0, 0, time.UTC)
	var all []string
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		if !have[key] {
			all = append(all, key)
		}
	}
	if len(all) > maxGaps {
		return all[:maxGaps], true
	}
	if all == nil {
		all = []string{}
	}
	return all, false
}

func percentChange(current, previous float64, previousCount int64) *float64 {
	if previousCount == 0 || previous == 0 {
		return nil
	}
	value := (current - previous) / previous * 100
	return &value
}

func periodChange(a, b float64, aCount, bCount int64) (delta, pct *float64) {
	if aCount == 0 || bCount == 0 {
		return nil, nil
	}
	diff := b - a
	delta = &diff
	if a == 0 {
		return delta, nil
	}
	value := (b - a) / a * 100
	return delta, &value
}
