package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"com.bestony.health-connect/internal/ingest"
	"com.bestony.health-connect/internal/store/duckdb"
	"com.bestony.health-connect/internal/store/sqlite"
)

func TestImportExportAndRejects(t *testing.T) {
	h, token, facts, meta := newAPI(t, true)
	body := fixture(t)

	unauthorized := postImport(t, h, "", "replace", "", "export.xml", body)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized = %d", unauthorized.Code)
	}

	rec := postImport(t, h, token, "replace", "same-key", "export.xml", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	waitImport(t, h, token, id, "succeeded")

	userID, err := meta.UserIDByToken(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	factsN, err := facts.CountFacts(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if factsN != 8 {
		t.Fatalf("facts = %d", factsN)
	}

	replay := postImport(t, h, token, "replace", "same-key", "export.xml", body)
	if replay.Code != http.StatusAccepted || !strings.Contains(replay.Body.String(), id) {
		t.Fatalf("replay = %d %s", replay.Code, replay.Body.String())
	}
	conflict := postImport(t, h, token, "replace", "same-key", "export.xml", []byte("<HealthData></HealthData>"))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict = %d %s", conflict.Code, conflict.Body.String())
	}
}

func TestRetryFailedImport(t *testing.T) {
	h, token, _, _ := newAPI(t, true)
	body := []byte(`<HealthData><Record type="HKQuantityTypeIdentifierStepCount" value="1" unit="count" startDate="bad" endDate="2024-01-02 08:00:00 +0800"/></HealthData>`)
	rec := postImport(t, h, token, "replace", "", "export.xml", body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	waitImport(t, h, token, id, "failed")
	req := httptest.NewRequest(http.MethodPost, "/v1/imports/"+id+"/retry", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	retry := httptest.NewRecorder()
	h.ServeHTTP(retry, req)
	if retry.Code != http.StatusAccepted {
		t.Fatalf("retry = %d %s", retry.Code, retry.Body.String())
	}
}

func TestActiveImportConflict(t *testing.T) {
	h, token, _, _ := newAPI(t, false)
	body := fixture(t)
	first := postImport(t, h, token, "", "", "export.xml", body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first = %d %s", first.Code, first.Body.String())
	}
	second := postImport(t, h, token, "", "", "export.xml", body)
	if second.Code != http.StatusConflict {
		t.Fatalf("second = %d %s", second.Code, second.Body.String())
	}
}

func newAPI(t *testing.T, startWorker bool) (http.Handler, string, *duckdb.DB, *sqlite.DB) {
	t.Helper()
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	meta, err := sqlite.Open(ctx, filepath.Join(dir, "meta.sqlite"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = meta.Close() })
	tokenPath := filepath.Join(dir, "bootstrap.token")
	if _, err := meta.BootstrapToken(ctx, tokenPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(raw))
	facts, err := duckdb.Open(ctx, filepath.Join(dir, "health.duckdb"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = facts.Close() })
	inbox := filepath.Join(dir, "inbox")
	worker := ingest.NewWorker(meta, facts, inbox, ingest.Limits{MaxEntries: 100, MaxBytes: 1 << 20}, log)
	if startWorker {
		worker.Start(ctx)
	}
	svc := ingest.NewService(meta, worker, inbox, log)
	handler := New(Deps{Log: log, Tokens: meta, NotFound: sqlite.ErrNotFound, Imports: svc, MaxUpload: 1 << 20})
	return handler, token, facts, meta
}

func postImport(t *testing.T, h http.Handler, token, mode, key, filename string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if mode != "" {
		if err := w.WriteField("mode", mode); err != nil {
			t.Fatal(err)
		}
	}
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/imports", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func waitImport(t *testing.T, h http.Handler, token, id, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/v1/imports/"+id, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		last = rec.Body.String()
		if rec.Code == http.StatusOK && strings.Contains(last, `"status":"`+want+`"`) {
			return
		}
		if strings.Contains(last, `"status":"failed"`) {
			t.Fatalf("import failed: %s", last)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s: %s", want, last)
}

func fixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("../../testdata/export_min.xml")
	if err != nil {
		t.Fatal(err)
	}
	return body
}
