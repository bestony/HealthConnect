package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSeriesReplaceAndDelete(t *testing.T) {
	h, token, _, _ := newAPI(t, true)
	body := fixture(t)
	rec := postImport(t, h, token, "replace", "", "export.xml", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	waitImport(t, h, token, created["id"].(string), "succeeded")

	series := authed(t, h, token, http.MethodGet, "/v1/series?type=HKQuantityTypeIdentifierStepCount&from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z&grain=day&agg=sum")
	if series.Code != http.StatusOK || !strings.Contains(series.Body.String(), `"value":1200`) {
		t.Fatalf("series = %d %s", series.Code, series.Body.String())
	}
	if !strings.Contains(series.Body.String(), `"unit":"count"`) {
		t.Fatalf("series unit = %s", series.Body.String())
	}

	missingAgg := authed(t, h, token, http.MethodGet, "/v1/series?type=HKQuantityTypeIdentifierStepCount&from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z&grain=day")
	if missingAgg.Code != http.StatusBadRequest {
		t.Fatalf("missing agg = %d", missingAgg.Code)
	}

	workouts := authed(t, h, token, http.MethodGet, "/v1/workouts?from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z")
	if workouts.Code != http.StatusOK || !strings.Contains(workouts.Body.String(), "HKWorkoutActivityTypeRunning") {
		t.Fatalf("workouts = %d %s", workouts.Code, workouts.Body.String())
	}
	catalog := authed(t, h, token, http.MethodGet, "/v1/catalog")
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), "HKQuantityTypeIdentifierStepCount") {
		t.Fatalf("catalog = %d %s", catalog.Code, catalog.Body.String())
	}
	activity := authed(t, h, token, http.MethodGet, "/v1/activity-summaries?from=2024-01-02&to=2024-01-03")
	if activity.Code != http.StatusOK || !strings.Contains(activity.Body.String(), `"move_minutes":20`) {
		t.Fatalf("activity = %d %s", activity.Code, activity.Body.String())
	}

	replacement := []byte(`<HealthData><Record type="HKQuantityTypeIdentifierStepCount" sourceName="iPhone" unit="count" startDate="2024-01-02 08:00:00 +0800" endDate="2024-01-02 09:00:00 +0800" value="50"/></HealthData>`)
	again := postImport(t, h, token, "replace", "", "export.xml", replacement)
	if again.Code != http.StatusAccepted {
		t.Fatalf("replace = %d %s", again.Code, again.Body.String())
	}
	var second map[string]any
	if err := json.Unmarshal(again.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	waitImport(t, h, token, second["id"].(string), "succeeded")
	series = authed(t, h, token, http.MethodGet, "/v1/series?type=HKQuantityTypeIdentifierStepCount&from=2024-01-01T00:00:00Z&to=2024-01-03T00:00:00Z&grain=day&agg=sum")
	if series.Code != http.StatusOK || !strings.Contains(series.Body.String(), `"value":50`) {
		t.Fatalf("replaced series = %d %s", series.Code, series.Body.String())
	}

	deleted := authed(t, h, token, http.MethodDelete, "/v1/data")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", deleted.Code, deleted.Body.String())
	}
	catalog = authed(t, h, token, http.MethodGet, "/v1/catalog")
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), `"types":[]`) {
		t.Fatalf("catalog = %d %s", catalog.Code, catalog.Body.String())
	}
}

func authed(t *testing.T, h http.Handler, token, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
