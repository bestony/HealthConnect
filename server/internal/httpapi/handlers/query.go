package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"com.bestony.health-connect/internal/httpapi/middleware"
	"com.bestony.health-connect/internal/store/duckdb"
	"com.bestony.health-connect/internal/store/sqlite"

	"github.com/gin-gonic/gin"
)

// Query serves stored health facts.
type Query struct {
	Facts *duckdb.DB
	Meta  *sqlite.DB
}

// Catalog lists stored types.
func (h Query) Catalog(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := h.Facts.Catalog(c.Request.Context(), userID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		units := item.Units
		if units == nil {
			units = []string{}
		}
		out = append(out, gin.H{
			"type": item.Type, "table": item.Table, "count": item.Count, "units": units,
			"start": item.Start.UTC().Format(time.RFC3339Nano),
			"end":   item.End.UTC().Format(time.RFC3339Nano),
		})
	}
	c.JSON(http.StatusOK, gin.H{"types": out})
}

// Series returns aggregated quantity samples. Buckets use the sample offset.
func (h Query) Series(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	typeName := c.Query("type")
	grain := c.Query("grain")
	agg := c.Query("agg")
	if typeName == "" || (grain != "hour" && grain != "day") || !validAgg(agg) {
		writeError(c, http.StatusBadRequest, "invalid_query")
		return
	}
	from, to, ok := timeRange(c)
	if !ok {
		return
	}
	table, err := h.Facts.SampleTable(c.Request.Context(), userID, typeName)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	if table == "category" {
		writeError(c, http.StatusBadRequest, "series_not_numeric")
		return
	}
	points, err := h.Facts.Series(c.Request.Context(), userID, typeName, grain, agg, from, to)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	out := make([]gin.H, 0, len(points))
	for _, point := range points {
		out = append(out, gin.H{
			"local_bucket": point.LocalBucket.UTC().Format("2006-01-02T15:04:05"),
			"unit":         point.Unit,
			"value":        point.Value,
		})
	}
	c.JSON(http.StatusOK, gin.H{"points": out})
}

// Records pages raw samples.
func (h Query) Records(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	typeName := c.Query("type")
	if typeName == "" {
		writeError(c, http.StatusBadRequest, "invalid_query")
		return
	}
	from, to, ok := timeRange(c)
	if !ok {
		return
	}
	limit := pageLimit(c, 100, 1000)
	var cursorAt *time.Time
	var cursorKey []byte
	if raw := c.Query("cursor"); raw != "" {
		at, key, err := duckdb.DecodeCursor(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_cursor")
			return
		}
		cursorAt = &at
		cursorKey = key
	}
	table, err := h.Facts.SampleTable(c.Request.Context(), userID, typeName)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	if table == "" {
		c.JSON(http.StatusOK, gin.H{"records": []any{}})
		return
	}
	tableName := "quantity_samples"
	if table == "category" {
		tableName = "category_samples"
	}
	records, err := h.Facts.Records(c.Request.Context(), userID, typeName, tableName, from, to, limit, cursorAt, cursorKey)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	out := make([]gin.H, 0, len(records))
	for _, rec := range records {
		item := gin.H{
			"type": rec.Type, "source_name": rec.SourceName,
			"start":         rec.Start.UTC().Format(time.RFC3339Nano),
			"end":           rec.End.UTC().Format(time.RFC3339Nano),
			"tz_offset_min": rec.TZOffsetMin,
		}
		if rec.Value != nil {
			item["value"] = *rec.Value
			item["unit"] = rec.Unit
		} else {
			item["value_text"] = rec.ValueText
		}
		if len(rec.Metadata) > 0 {
			item["metadata"] = jsonRaw(rec.Metadata)
		}
		out = append(out, item)
	}
	body := gin.H{"records": out}
	if len(records) == limit {
		last := records[len(records)-1]
		body["next_cursor"] = duckdb.EncodeCursor(last.Start, last.CursorKey())
	}
	c.JSON(http.StatusOK, body)
}

// Workouts pages workouts.
func (h Query) Workouts(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	from, to, ok := timeRange(c)
	if !ok {
		return
	}
	limit := pageLimit(c, 100, 1000)
	var cursorAt *time.Time
	var cursorID string
	if raw := c.Query("cursor"); raw != "" {
		at, key, err := duckdb.DecodeCursor(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_cursor")
			return
		}
		cursorAt = &at
		cursorID = string(key)
	}
	rows, err := h.Facts.Workouts(c.Request.Context(), userID, from, to, limit, cursorAt, cursorID, "")
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		events := make([]gin.H, 0, len(row.Events))
		for _, event := range row.Events {
			item := gin.H{
				"type": event.Type, "at": event.At.UTC().Format(time.RFC3339Nano),
				"tz_offset_min": event.TZOffsetMin,
			}
			if event.DurationSec != nil {
				item["duration_sec"] = *event.DurationSec
			}
			events = append(events, item)
		}
		stats := make([]gin.H, 0, len(row.Statistics))
		for _, stat := range row.Statistics {
			item := gin.H{"type": stat.Type, "unit": stat.Unit}
			if stat.Sum != nil {
				item["sum"] = *stat.Sum
			}
			if stat.Minimum != nil {
				item["minimum"] = *stat.Minimum
			}
			if stat.Maximum != nil {
				item["maximum"] = *stat.Maximum
			}
			if stat.Average != nil {
				item["average"] = *stat.Average
			}
			stats = append(stats, item)
		}
		item := gin.H{
			"id": row.ID, "activity_type": row.ActivityType, "source_name": row.SourceName,
			"start":         row.Start.UTC().Format(time.RFC3339Nano),
			"end":           row.End.UTC().Format(time.RFC3339Nano),
			"tz_offset_min": row.TZOffsetMin,
			"events":        events,
			"statistics":    stats,
		}
		if row.DurationSec != nil {
			item["duration_sec"] = *row.DurationSec
		}
		if row.Distance != nil {
			item["distance"] = *row.Distance
			item["distance_unit"] = row.DistanceUnit
		}
		if row.Energy != nil {
			item["energy"] = *row.Energy
			item["energy_unit"] = row.EnergyUnit
		}
		out = append(out, item)
	}
	body := gin.H{"workouts": out}
	if len(rows) == limit {
		last := rows[len(rows)-1]
		body["next_cursor"] = duckdb.EncodeCursor(last.Start, []byte(last.ID))
	}
	c.JSON(http.StatusOK, body)
}

// Activity returns daily activity summaries.
func (h Query) Activity(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	from, err := parseDay(c.Query("from"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_query")
		return
	}
	to, err := parseDay(c.Query("to"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_query")
		return
	}
	rows, err := h.Facts.ActivitySummaries(c.Request.Context(), userID, from, to)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := gin.H{"day": row.Day.UTC().Format("2006-01-02"), "active_energy_unit": row.ActiveEnergyUnit}
		putFloat(item, "active_energy", row.ActiveEnergy)
		putFloat(item, "active_energy_goal", row.ActiveEnergyGoal)
		putFloat(item, "exercise_minutes", row.ExerciseMinutes)
		putFloat(item, "exercise_goal", row.ExerciseGoal)
		putFloat(item, "stand_hours", row.StandHours)
		putFloat(item, "stand_goal", row.StandGoal)
		putFloat(item, "move_minutes", row.MoveMinutes)
		putFloat(item, "move_goal", row.MoveGoal)
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"activity_summaries": out})
}

// DeleteData removes the caller's facts and characteristics.
func (h Query) DeleteData(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.Facts.DeleteUser(c.Request.Context(), userID); err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.Meta.DeleteCharacteristics(c.Request.Context(), userID); err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	c.Status(http.StatusNoContent)
}

func validAgg(agg string) bool {
	switch agg {
	case "sum", "avg", "min", "max":
		return true
	default:
		return false
	}
}

func timeRange(c *gin.Context) (time.Time, time.Time, bool) {
	from, err := time.Parse(time.RFC3339, c.Query("from"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_query")
		return time.Time{}, time.Time{}, false
	}
	to, err := time.Parse(time.RFC3339, c.Query("to"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_query")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

func parseDay(raw string) (time.Time, error) {
	if len(raw) == 10 {
		return time.Parse("2006-01-02", raw)
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

func pageLimit(c *gin.Context, fallback, max int) int {
	raw := c.Query("limit")
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}

func putFloat(item gin.H, key string, value *float64) {
	if value != nil {
		item[key] = *value
	}
}

func jsonRaw(body []byte) any {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return string(body)
	}
	return v
}
