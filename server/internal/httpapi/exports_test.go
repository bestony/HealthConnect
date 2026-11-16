package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"com.bestony.health-connect/internal/export"
)

func TestExportUploadContract(t *testing.T) {
	stack := newStack(t, false)
	h, token := stack.handler, stack.token

	missing := postJSON(t, h, "", "/v1/exports", `{"startedAt":"2026-01-01T00:00:00.000Z"}`)
	if missing.Code != http.StatusUnauthorized || !strings.Contains(missing.Body.String(), `"error":"unauthorized"`) {
		t.Fatalf("unauthorized = %d %s", missing.Code, missing.Body.String())
	}
	badTime := postJSON(t, h, token, "/v1/exports", `{"startedAt":"yesterday"}`)
	if badTime.Code != http.StatusBadRequest || !strings.Contains(badTime.Body.String(), `"error":"invalid_body"`) {
		t.Fatalf("bad time = %d %s", badTime.Code, badTime.Body.String())
	}
	plain := postJSON(t, h, token, "/v1/exports", `{"startedAt":"2026-01-01T00:00:00Z"}`)
	if plain.Code != http.StatusCreated {
		t.Fatalf("plain time = %d %s", plain.Code, plain.Body.String())
	}

	created := postJSON(t, h, token, "/v1/exports", `{"startedAt":"2026-01-01T00:00:00.000Z"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var createBody map[string]string
	if err := json.Unmarshal(created.Body.Bytes(), &createBody); err != nil {
		t.Fatal(err)
	}
	id := createBody["exportId"]
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("exportId = %q", id)
	}

	negative := postJSON(t, h, token, "/v1/exports/"+id+"/complete", `{"sampleCount":-1,"batchCount":1}`)
	if negative.Code != http.StatusBadRequest {
		t.Fatalf("negative = %d %s", negative.Code, negative.Body.String())
	}
	empty := postRaw(t, h, token, "/v1/exports/"+id+"/batches", "application/x-ndjson", "")
	if empty.Code != http.StatusBadRequest || !strings.Contains(empty.Body.String(), `"error":"empty_body"`) {
		t.Fatalf("empty = %d %s", empty.Code, empty.Body.String())
	}
	unknown := postRaw(t, h, token, "/v1/exports/0123456789abcdef0123456789abcdef/batches", "application/x-ndjson", "{}\n")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown = %d %s", unknown.Code, unknown.Body.String())
	}
	badID := postRaw(t, h, token, "/v1/exports/not-an-id/batches", "application/x-ndjson", "{}\n")
	if badID.Code != http.StatusNotFound {
		t.Fatalf("bad id = %d %s", badID.Code, badID.Body.String())
	}

	body := exportFixture()
	batch := postRaw(t, h, token, "/v1/exports/"+id+"/batches", "application/x-ndjson", body)
	if batch.Code != http.StatusNoContent || batch.Body.Len() != 0 {
		t.Fatalf("batch = %d %s", batch.Code, batch.Body.String())
	}
	storedPath := filepath.Join(stack.dir, "exports", id, "0.ndjson")
	stored, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != body {
		t.Fatal("stored batch bytes differ from the request body")
	}
	info, err := os.Stat(storedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(storedPath))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o", dirInfo.Mode().Perm())
	}

	stack.exports.SetMaxBatchBytes(4)
	tooBig := postRaw(t, h, token, "/v1/exports/"+id+"/batches", "application/x-ndjson", "12345")
	stack.exports.SetMaxBatchBytes(export.MaxBatchBytes)
	if tooBig.Code != http.StatusRequestEntityTooLarge || !strings.Contains(tooBig.Body.String(), `"error":"upload_too_large"`) {
		t.Fatalf("too big = %d %s", tooBig.Code, tooBig.Body.String())
	}

	done := postJSON(t, h, token, "/v1/exports/"+id+"/complete", `{"sampleCount":9,"batchCount":1}`)
	if done.Code != http.StatusOK {
		t.Fatalf("complete = %d %s", done.Code, done.Body.String())
	}
	var manifest map[string]any
	if err := json.Unmarshal(done.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["sampleCount"] != float64(9) || manifest["batchCount"] != float64(1) || manifest["receivedBatchCount"] != float64(1) {
		t.Fatalf("manifest = %s", done.Body.String())
	}
	completedAt, _ := manifest["completedAt"].(string)
	completed, err := time.Parse(time.RFC3339Nano, completedAt)
	if err != nil || time.Since(completed) > time.Minute || time.Until(completed) > time.Second {
		t.Fatalf("completedAt = %q", completedAt)
	}

	again := postJSON(t, h, token, "/v1/exports/"+id+"/complete", `{"sampleCount":1,"batchCount":9}`)
	if again.Code != http.StatusOK || again.Body.String() != done.Body.String() {
		t.Fatalf("second complete = %d %s", again.Code, again.Body.String())
	}
	closed := postRaw(t, h, token, "/v1/exports/"+id+"/batches", "application/x-ndjson", "{}\n")
	if closed.Code != http.StatusConflict || !strings.Contains(closed.Body.String(), `"error":"export_closed"`) {
		t.Fatalf("closed = %d %s", closed.Code, closed.Body.String())
	}

	series := authed(t, h, token, http.MethodGet, "/v1/series?type=HKQuantityTypeIdentifierStepCount&from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z&grain=day&agg=sum")
	if series.Code != http.StatusOK || !strings.Contains(series.Body.String(), `"value":1200`) || !strings.Contains(series.Body.String(), `"unit":"count"`) {
		t.Fatalf("series = %d %s", series.Code, series.Body.String())
	}
	systolic := authed(t, h, token, http.MethodGet, "/v1/series?type=HKQuantityTypeIdentifierBloodPressureSystolic&from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z&grain=day&agg=sum")
	if systolic.Code != http.StatusOK || !strings.Contains(systolic.Body.String(), `"value":120`) {
		t.Fatalf("systolic = %d %s", systolic.Code, systolic.Body.String())
	}
	sleep := authed(t, h, token, http.MethodGet, "/v1/records?type=HKCategoryTypeIdentifierSleepAnalysis&from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z")
	if sleep.Code != http.StatusOK || !strings.Contains(sleep.Body.String(), `"value_text":"1"`) {
		t.Fatalf("sleep = %d %s", sleep.Code, sleep.Body.String())
	}
	workouts := authed(t, h, token, http.MethodGet, "/v1/workouts?from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z")
	if workouts.Code != http.StatusOK || !strings.Contains(workouts.Body.String(), `"activity_type":"37"`) || !strings.Contains(workouts.Body.String(), `"duration_sec":1800`) || !strings.Contains(workouts.Body.String(), `"sum":100`) {
		t.Fatalf("workouts = %d %s", workouts.Code, workouts.Body.String())
	}
	activity := authed(t, h, token, http.MethodGet, "/v1/activity-summaries?from=2024-01-02&to=2024-01-03")
	if activity.Code != http.StatusOK || !strings.Contains(activity.Body.String(), `"move_minutes":20`) || !strings.Contains(activity.Body.String(), `"active_energy":400`) {
		t.Fatalf("activity = %d %s", activity.Code, activity.Body.String())
	}
	userID, err := stack.meta.UserIDByToken(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	sex, err := stack.meta.Characteristic(context.Background(), userID, "HKCharacteristicTypeIdentifierBiologicalSex")
	if err != nil || sex != "2" {
		t.Fatalf("sex = %q %v", sex, err)
	}
	dob, err := stack.meta.Characteristic(context.Background(), userID, "HKCharacteristicTypeIdentifierDateOfBirth")
	if err != nil || dob != "1990-01-02" {
		t.Fatalf("dob = %q %v", dob, err)
	}

	emptyExport := postJSON(t, h, token, "/v1/exports", `{"startedAt":"2026-01-02T00:00:00Z"}`)
	if emptyExport.Code != http.StatusCreated {
		t.Fatalf("empty export = %d %s", emptyExport.Code, emptyExport.Body.String())
	}
	var emptyBody map[string]string
	if err := json.Unmarshal(emptyExport.Body.Bytes(), &emptyBody); err != nil {
		t.Fatal(err)
	}
	kept := postJSON(t, h, token, "/v1/exports/"+emptyBody["exportId"]+"/complete", `{"sampleCount":0,"batchCount":0}`)
	if kept.Code != http.StatusOK || !strings.Contains(kept.Body.String(), `"receivedBatchCount":0`) {
		t.Fatalf("empty complete = %d %s", kept.Code, kept.Body.String())
	}
	series = authed(t, h, token, http.MethodGet, "/v1/series?type=HKQuantityTypeIdentifierStepCount&from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z&grain=day&agg=sum")
	if series.Code != http.StatusOK || !strings.Contains(series.Body.String(), `"value":1200`) {
		t.Fatalf("series after empty complete = %d %s", series.Code, series.Body.String())
	}

	partial := postJSON(t, h, token, "/v1/exports", `{"startedAt":"2026-01-03T00:00:00Z"}`)
	if partial.Code != http.StatusCreated {
		t.Fatalf("partial create = %d %s", partial.Code, partial.Body.String())
	}
	var partialBody map[string]string
	if err := json.Unmarshal(partial.Body.Bytes(), &partialBody); err != nil {
		t.Fatal(err)
	}
	partialID := partialBody["exportId"]
	badBatch := postRaw(t, h, token, "/v1/exports/"+partialID+"/batches", "application/x-ndjson", "{not-json}\n")
	if badBatch.Code != http.StatusNoContent {
		t.Fatalf("bad batch store = %d %s", badBatch.Code, badBatch.Body.String())
	}
	failed := postJSON(t, h, token, "/v1/exports/"+partialID+"/complete", `{"sampleCount":1,"batchCount":1}`)
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("bad complete = %d %s", failed.Code, failed.Body.String())
	}
	stillOpen := postRaw(t, h, token, "/v1/exports/"+partialID+"/batches", "application/x-ndjson", "{}\n")
	if stillOpen.Code != http.StatusNoContent {
		t.Fatalf("retry batch = %d %s", stillOpen.Code, stillOpen.Body.String())
	}
}

func exportFixture() string {
	lines := []string{
		`{"day":2,"kind":"characteristic","month":1,"typeIdentifier":"HKCharacteristicTypeIdentifierDateOfBirth","year":1990}`,
		`{"kind":"characteristic","typeIdentifier":"HKCharacteristicTypeIdentifierBiologicalSex","value":2}`,
		`{"device":"iPhone","end":"2024-01-02T01:00:00.000Z","kind":"sample","source":"com.apple.Health","start":"2024-01-02T00:00:00.000Z","typeIdentifier":"HKQuantityTypeIdentifierStepCount","unit":"count","uuid":"E621E1F8-C36C-495A-93FC-0C247A3E6E5F","value":1200}`,
		`{"end":"2024-01-02T08:00:00.000Z","kind":"sample","source":"com.apple.health","start":"2024-01-02T00:00:00.000Z","typeIdentifier":"HKCategoryTypeIdentifierSleepAnalysis","uuid":"1721E1F8-C36C-495A-93FC-0C247A3E6E5F","value":1}`,
		`{"end":"2024-01-02T00:01:00.000Z","kind":"sample","objects":[{"end":"2024-01-02T00:01:00.000Z","kind":"sample","start":"2024-01-02T00:00:00.000Z","typeIdentifier":"HKQuantityTypeIdentifierBloodPressureSystolic","unit":"mmHg","uuid":"D621E1F8-C36C-495A-93FC-0C247A3E6E5F","value":120},{"end":"2024-01-02T00:01:00.000Z","kind":"sample","start":"2024-01-02T00:00:00.000Z","typeIdentifier":"HKQuantityTypeIdentifierBloodPressureDiastolic","unit":"mmHg","uuid":"E721E1F8-C36C-495A-93FC-0C247A3E6E5F","value":80}],"source":"com.example.monitor","start":"2024-01-02T00:00:00.000Z","typeIdentifier":"HKCorrelationTypeIdentifierBloodPressure","uuid":"F621E1F8-C36C-495A-93FC-0C247A3E6E5F"}`,
		`{"activityType":37,"duration":1800,"end":"2024-01-02T01:00:00.000Z","kind":"sample","source":"com.apple.health","start":"2024-01-02T00:00:00.000Z","statistics":[{"sum":100,"type":"HKQuantityTypeIdentifierActiveEnergyBurned","unit":"kcal"}],"typeIdentifier":"HKWorkoutTypeIdentifier","uuid":"A621E1F8-C36C-495A-93FC-0C247A3E6E5F"}`,
		`{"activeEnergyBurnedGoalKcal":500,"activeEnergyBurnedKcal":400,"appleExerciseTimeMin":30,"appleMoveTimeGoalMin":30,"appleMoveTimeMin":20,"appleStandHours":8,"day":2,"exerciseTimeGoalMin":30,"kind":"activitySummary","month":1,"paused":false,"standHoursGoal":12,"typeIdentifier":"HKActivitySummaryTypeIdentifier","year":2024}`,
		`{"displayText":"Vitamin D","domain":"medication","hasSchedule":true,"isArchived":false,"kind":"medication","nickname":"Vitamin D","typeIdentifier":"HKUserAnnotatedMedicationTypeIdentifier"}`,
		`{"end":"2024-01-02T00:00:10.000Z","kind":"sample","start":"2024-01-02T00:00:00.000Z","typeIdentifier":"HKDataTypeIdentifierElectrocardiogram","uuid":"C621E1F8-C36C-495A-93FC-0C247A3E6E5F","voltages":[{"millivolts":0.1,"timeSinceStart":0}]}`,
	}
	return strings.Join(lines, "\n") + "\n"
}

func postJSON(t *testing.T, h http.Handler, token, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postRaw(t, h, token, target, "application/json", body)
}

func postRaw(t *testing.T, h http.Handler, token, target, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
