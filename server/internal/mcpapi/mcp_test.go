package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"com.bestony.health-connect/internal/domain"
	"com.bestony.health-connect/internal/store/duckdb"
	"com.bestony.health-connect/internal/store/sqlite"
)

func TestMCPTools(t *testing.T) {
	logBuf := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	meta, facts, tokenA, tokenB := openPair(t, log)
	loadUser(t, facts, meta, tokenA, 100, "1990-01-01", true)
	loadUser(t, facts, meta, tokenB, 9999, "2001-01-01", false)
	handler := Handler(Deps{Log: log, Tokens: meta, NotFound: sqlite.ErrNotFound, Facts: facts, Meta: meta})

	missing := postMCP(t, handler, "", "tools/list", map[string]any{})
	if missing.Code != http.StatusUnauthorized || strings.Contains(missing.Body.String(), "hc_") {
		t.Fatalf("missing = %d %s", missing.Code, missing.Body.String())
	}
	secret := "hc_SECRET_TOKEN_VALUE"
	unknown := postMCP(t, handler, secret, "tools/list", map[string]any{})
	if unknown.Code != http.StatusUnauthorized || strings.Contains(unknown.Body.String(), secret) {
		t.Fatalf("unknown = %d %s", unknown.Code, unknown.Body.String())
	}

	listed := call(t, handler, tokenA, "tools/list", map[string]any{})
	names := toolNames(t, listed)
	for _, name := range []string{"list_catalog", "get_profile", "query_series", "query_records", "query_workouts", "query_activity", "query_health_records", "summarize_range", "compare_periods"} {
		if !names[name] {
			t.Fatalf("missing tool %s in %v", name, names)
		}
	}
	for name := range names {
		if strings.Contains(name, "delete") || strings.Contains(name, "import") {
			t.Fatalf("write tool %s", name)
		}
	}

	catA := tool(t, handler, tokenA, "list_catalog", map[string]any{})
	if catA["workouts"].(float64) != 2 || catA["health_records"].(float64) != 2 {
		t.Fatalf("catalog A = %s", mustJSON(catA))
	}
	catB := tool(t, handler, tokenB, "list_catalog", map[string]any{})
	if catB["workouts"].(float64) != 0 || catB["health_records"].(float64) != 0 {
		t.Fatalf("catalog B = %s", mustJSON(catB))
	}

	profileA := tool(t, handler, tokenA, "get_profile", map[string]any{})
	if strings.Contains(mustJSON(profileA), "2001-01-01") || !strings.Contains(mustJSON(profileA), "1990-01-01") {
		t.Fatalf("profile A = %s", mustJSON(profileA))
	}
	profileB := tool(t, handler, tokenB, "get_profile", map[string]any{})
	if strings.Contains(mustJSON(profileB), "1990-01-01") {
		t.Fatalf("profile B = %s", mustJSON(profileB))
	}

	series := tool(t, handler, tokenA, "query_series", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "from": "2024-01-02T00:00:00Z", "to": "2024-01-04T00:00:00Z",
		"grain": "day", "agg": "sum",
	})
	points := series["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("points = %s", mustJSON(series))
	}
	seriesB := tool(t, handler, tokenB, "query_series", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z",
		"grain": "day", "agg": "sum",
	})
	if got := seriesB["points"].([]any); len(got) != 1 || got[0].(map[string]any)["value"].(float64) != 9999 {
		t.Fatalf("series B = %s", mustJSON(seriesB))
	}

	bad := toolErr(t, handler, tokenA, "query_series", map[string]any{
		"type": "HKCategoryTypeIdentifierSleepAnalysis", "from": "2024-01-02T00:00:00Z", "to": "2024-01-04T00:00:00Z",
		"grain": "day", "agg": "sum",
	})
	if bad["error"] != "series_not_numeric" {
		t.Fatalf("category series = %s", mustJSON(bad))
	}
	long := toolErr(t, handler, tokenA, "query_series", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "from": "2024-01-01T00:00:00Z", "to": "2024-01-16T00:00:00Z",
		"grain": "hour", "agg": "sum",
	})
	if long["error"] != "invalid_query" {
		t.Fatalf("long hour = %s", mustJSON(long))
	}

	page := tool(t, handler, tokenA, "query_records", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z", "limit": 1,
	})
	if len(page["records"].([]any)) != 1 || page["next_cursor"] == "" {
		t.Fatalf("page = %s", mustJSON(page))
	}
	next := tool(t, handler, tokenA, "query_records", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z",
		"limit": 10, "cursor": page["next_cursor"],
	})
	if len(next["records"].([]any)) != 2 {
		t.Fatalf("next = %s", mustJSON(next))
	}
	if toolErr(t, handler, tokenA, "query_records", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z", "cursor": "nope",
	})["error"] != "invalid_query" {
		t.Fatal("cursor error")
	}

	runs := tool(t, handler, tokenA, "query_workouts", map[string]any{
		"from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z", "activity_type": "HKWorkoutActivityTypeRunning",
	})
	workouts := runs["workouts"].([]any)
	if len(workouts) != 1 || workouts[0].(map[string]any)["events"] != nil {
		t.Fatalf("runs = %s", mustJSON(runs))
	}
	withEvents := tool(t, handler, tokenA, "query_workouts", map[string]any{
		"from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z",
		"activity_type": "HKWorkoutActivityTypeRunning", "include_events": true,
	})
	events := withEvents["workouts"].([]any)[0].(map[string]any)["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events = %s", mustJSON(withEvents))
	}

	activity := tool(t, handler, tokenA, "query_activity", map[string]any{"from": "2024-01-02", "to": "2024-01-03"})
	if len(activity["activity_summaries"].([]any)) != 1 {
		t.Fatalf("activity = %s", mustJSON(activity))
	}

	clinical := tool(t, handler, tokenA, "query_health_records", map[string]any{
		"from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z",
	})
	records := clinical["records"].([]any)
	if len(records) != 2 {
		t.Fatalf("clinical = %s", mustJSON(clinical))
	}
	var sawTruncated bool
	for _, raw := range records {
		item := raw.(map[string]any)
		if item["truncated"].(bool) {
			sawTruncated = true
			if len(item["payload"].(string)) != payloadLimit {
				t.Fatalf("payload len = %d", len(item["payload"].(string)))
			}
		}
	}
	if !sawTruncated {
		t.Fatal("expected a truncated payload")
	}
	filtered := tool(t, handler, tokenA, "query_health_records", map[string]any{
		"type_identifier": "lab.small", "from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z",
	})
	if len(filtered["records"].([]any)) != 1 || filtered["records"].([]any)[0].(map[string]any)["truncated"].(bool) {
		t.Fatalf("filtered = %s", mustJSON(filtered))
	}
	if len(tool(t, handler, tokenB, "query_health_records", map[string]any{
		"from": "2024-01-02T00:00:00Z", "to": "2024-01-05T00:00:00Z",
	})["records"].([]any)) != 0 {
		t.Fatal("user B saw clinical records")
	}

	summary := tool(t, handler, tokenA, "summarize_range", map[string]any{
		"from": "2024-01-03T00:00:00Z", "to": "2024-01-04T00:00:00Z",
		"types": []string{"HKQuantityTypeIdentifierStepCount"},
	})
	unit := summary["quantities"].([]any)[0].(map[string]any)["units"].([]any)[0].(map[string]any)
	near(t, unit["avg"].(float64), 300)
	near(t, unit["previous_avg"].(float64), 100)
	near(t, unit["delta_pct"].(float64), 200)
	gaps := tool(t, handler, tokenA, "summarize_range", map[string]any{
		"from": "2024-01-01T00:00:00Z", "to": "2024-01-04T00:00:00Z",
		"types": []string{"HKQuantityTypeIdentifierStepCount"},
	})
	qty := gaps["quantities"].([]any)[0].(map[string]any)
	if qty["days_with_data"].(float64) != 2 || mustJSON(qty["gaps"]) != `["2024-01-01"]` {
		t.Fatalf("gaps = %s", mustJSON(qty))
	}
	if _, ok := qty["units"].([]any)[0].(map[string]any)["delta_pct"]; ok {
		t.Fatal("empty previous window has delta_pct")
	}
	work := gaps["workouts"].(map[string]any)
	if work["count"].(float64) != 2 || work["duration_sec"].(float64) != 1860 {
		t.Fatalf("workouts = %s", mustJSON(work))
	}

	compared := tool(t, handler, tokenA, "compare_periods", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "agg": "avg",
		"period_a": map[string]string{"from": "2024-01-02T00:00:00Z", "to": "2024-01-03T00:00:00Z"},
		"period_b": map[string]string{"from": "2024-01-03T00:00:00Z", "to": "2024-01-04T00:00:00Z"},
	})
	row := compared["rows"].([]any)[0].(map[string]any)
	near(t, row["delta"].(float64), 200)
	near(t, row["delta_pct"].(float64), 200)
	empty := tool(t, handler, tokenA, "compare_periods", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "agg": "avg",
		"period_a": map[string]string{"from": "2020-01-01T00:00:00Z", "to": "2020-01-02T00:00:00Z"},
		"period_b": map[string]string{"from": "2024-01-02T00:00:00Z", "to": "2024-01-03T00:00:00Z"},
	})
	emptyRow := empty["rows"].([]any)[0].(map[string]any)
	if _, ok := emptyRow["delta"]; ok || emptyRow["delta_pct"] != nil {
		t.Fatalf("empty delta = %s", mustJSON(emptyRow))
	}
	if toolErr(t, handler, tokenA, "compare_periods", map[string]any{
		"type": "HKCategoryTypeIdentifierSleepAnalysis", "agg": "avg",
		"period_a": map[string]string{"from": "2024-01-02T00:00:00Z", "to": "2024-01-03T00:00:00Z"},
		"period_b": map[string]string{"from": "2024-01-03T00:00:00Z", "to": "2024-01-04T00:00:00Z"},
	})["error"] != "series_not_numeric" {
		t.Fatal("compare category")
	}

	prevBytes, prevPoints := maxResultBytes, maxSeriesPoints
	t.Cleanup(func() {
		maxResultBytes = prevBytes
		maxSeriesPoints = prevPoints
	})
	maxResultBytes = 1
	if toolErr(t, handler, tokenA, "list_catalog", map[string]any{})["error"] != "result_too_large" {
		t.Fatal("expected result_too_large")
	}
	maxResultBytes = prevBytes
	maxSeriesPoints = 0
	if toolErr(t, handler, tokenA, "query_series", map[string]any{
		"type": "HKQuantityTypeIdentifierStepCount", "from": "2024-01-02T00:00:00Z", "to": "2024-01-03T00:00:00Z",
		"grain": "day", "agg": "sum",
	})["error"] != "result_too_large" {
		t.Fatal("expected series cap")
	}

	if strings.Contains(logBuf.String(), tokenA) || strings.Contains(logBuf.String(), tokenB) || strings.Contains(logBuf.String(), secret) {
		t.Fatalf("log contains a token: %s", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "list_catalog") {
		t.Fatalf("log = %s", logBuf.String())
	}
}

func TestGapDatesAndChange(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 20)
	var present []time.Time
	dates, truncated := gapDates(from, to, present)
	if !truncated || len(dates) != maxGaps {
		t.Fatalf("gaps = %v %v", dates, truncated)
	}
	if percentChange(10, 0, 1) != nil || percentChange(10, 5, 0) != nil {
		t.Fatal("percent change")
	}
	delta, pct := periodChange(0, 10, 1, 1)
	if delta == nil || *delta != 10 || pct != nil {
		t.Fatalf("zero base = %v %v", delta, pct)
	}
}

func openPair(t *testing.T, log *slog.Logger) (*sqlite.DB, *duckdb.DB, string, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	meta, err := sqlite.Open(ctx, filepath.Join(dir, "meta.sqlite"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = meta.Close() })
	_, tokenA, err := meta.MintToken(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	_, tokenB, err := meta.MintToken(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	facts, err := duckdb.Open(ctx, filepath.Join(dir, "health.duckdb"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = facts.Close() })
	return meta, facts, tokenA, tokenB
}

func loadUser(t *testing.T, facts *duckdb.DB, meta *sqlite.DB, token string, step float64, dob string, full bool) {
	t.Helper()
	ctx := context.Background()
	userID, err := meta.UserIDByToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	importID, err := domain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.ReplaceCharacteristics(ctx, userID, importID, map[string]string{
		"HKCharacteristicTypeIdentifierDateOfBirth": dob,
	}); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := facts.WithLoader(ctx, func(loader *duckdb.Loader) error {
		for i, value := range []float64{step, step + 200, 50} {
			if !full && i > 0 {
				break
			}
			at := start.Add(time.Duration(i) * 24 * time.Hour)
			if !full {
				value = step
			}
			if err := loader.AppendQuantity(userID, importID, domain.QuantitySample{
				Type: "HKQuantityTypeIdentifierStepCount", Value: value, ValueText: format(value),
				Unit: "count", SourceName: "Phone", Start: at, End: at.Add(time.Hour),
				StartRaw: at.Format(time.RFC3339), EndRaw: at.Add(time.Hour).Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
		if !full {
			return nil
		}
		if err := loader.AppendCategory(userID, importID, domain.CategorySample{
			Type: "HKCategoryTypeIdentifierSleepAnalysis", ValueText: "HKCategoryValueSleepAnalysisAsleepCore",
			SourceName: "Watch", Start: start, End: start.Add(8 * time.Hour),
			StartRaw: "sleep-start", EndRaw: "sleep-end",
		}); err != nil {
			return err
		}
		runDur := 1800.0
		walkDur := 60.0
		runID, err := domain.NewID()
		if err != nil {
			return err
		}
		walkID, err := domain.NewID()
		if err != nil {
			return err
		}
		if err := loader.AppendWorkout(userID, importID, domain.Workout{
			ID: runID, ActivityType: "HKWorkoutActivityTypeRunning", DurationSec: &runDur, SourceName: "Watch",
			Start: start.Add(time.Hour), End: start.Add(2 * time.Hour), StartRaw: "run-start", EndRaw: "run-end",
			Events: []domain.WorkoutEvent{{Type: "HKWorkoutEventTypePause", At: start.Add(90 * time.Minute), AtRaw: "pause"}},
		}); err != nil {
			return err
		}
		if err := loader.AppendWorkout(userID, importID, domain.Workout{
			ID: walkID, ActivityType: "HKWorkoutActivityTypeWalking", DurationSec: &walkDur, SourceName: "Watch",
			Start: start.Add(3 * time.Hour), End: start.Add(4 * time.Hour), StartRaw: "walk-start", EndRaw: "walk-end",
		}); err != nil {
			return err
		}
		energy, goal, exercise, stand, standGoal := 400.0, 500.0, 30.0, 8.0, 12.0
		if err := loader.AppendActivity(userID, importID, domain.ActivitySummary{
			Day: start, ActiveEnergy: &energy, ActiveEnergyGoal: &goal, ActiveEnergyUnit: "kcal",
			ExerciseMinutes: &exercise, ExerciseGoal: &exercise, StandHours: &stand, StandGoal: &standGoal,
		}); err != nil {
			return err
		}
		small := []byte(`{"ok":true}`)
		large := append(append([]byte(`{"note":"`), bytes.Repeat([]byte("x"), payloadLimit)...), []byte(`"}`)...)
		if err := loader.AppendHealthRecord(userID, importID, duckdb.HealthRecord{
			Kind: "clinical", TypeIdentifier: "lab.small", Start: &start, Source: "Clinic", Payload: small,
		}); err != nil {
			return err
		}
		later := start.Add(time.Hour)
		return loader.AppendHealthRecord(userID, importID, duckdb.HealthRecord{
			Kind: "clinical", TypeIdentifier: "lab.large", Start: &later, Source: "Clinic", Payload: large,
		})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := facts.CommitImport(ctx, userID, importID, "merge"); err != nil {
		t.Fatal(err)
	}
}

func format(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func postMCP(t *testing.T, handler http.Handler, token, method string, params any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func call(t *testing.T, handler http.Handler, token, method string, params any) map[string]any {
	t.Helper()
	rec := postMCP(t, handler, token, method, params)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d %s", method, rec.Code, rec.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("%s body = %s", method, rec.Body.String())
	}
	if env["error"] != nil {
		t.Fatalf("%s error = %s", method, rec.Body.String())
	}
	result, _ := env["result"].(map[string]any)
	if result == nil {
		t.Fatalf("%s result = %s", method, rec.Body.String())
	}
	return result
}

func tool(t *testing.T, handler http.Handler, token, name string, args any) map[string]any {
	t.Helper()
	result := call(t, handler, token, "tools/call", map[string]any{"name": name, "arguments": args})
	if result["isError"] == true {
		t.Fatalf("%s error = %s", name, mustJSON(result))
	}
	out, _ := result["structuredContent"].(map[string]any)
	if out == nil {
		t.Fatalf("%s content = %s", name, mustJSON(result))
	}
	return out
}

func toolErr(t *testing.T, handler http.Handler, token, name string, args any) map[string]any {
	t.Helper()
	result := call(t, handler, token, "tools/call", map[string]any{"name": name, "arguments": args})
	if result["isError"] != true {
		t.Fatalf("%s = %s", name, mustJSON(result))
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("%s content = %s", name, mustJSON(result))
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("%s text = %s", name, text)
	}
	return body
}

func toolNames(t *testing.T, result map[string]any) map[string]bool {
	t.Helper()
	raw, _ := result["tools"].([]any)
	names := map[string]bool{}
	for _, item := range raw {
		name, _ := item.(map[string]any)["name"].(string)
		names[name] = true
	}
	return names
}

func mustJSON(v any) string {
	body, _ := json.Marshal(v)
	return string(body)
}

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("got %v want %v", got, want)
	}
}
