package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"com.bestony.health-connect/internal/store/sqlite"
)

func TestMCPRoute(t *testing.T) {
	off := New(Deps{Log: slog.New(slog.DiscardHandler)})
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	off.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled = %d", rec.Code)
	}

	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	stack := newStack(t, false)
	on := New(Deps{
		Log: log, Tokens: stack.meta, NotFound: sqlite.ErrNotFound,
		Facts: stack.facts, Meta: stack.meta, MCPEnabled: true,
	})
	health := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	healthRec := httptest.NewRecorder()
	on.ServeHTTP(healthRec, health)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", healthRec.Code)
	}

	secret := "hc_SECRET_TOKEN_VALUE"
	denied := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
	denied.Header.Set("Content-Type", "application/json")
	denied.Header.Set("Accept", "application/json, text/event-stream")
	denied.Header.Set("Authorization", "Bearer "+secret)
	deniedRec := httptest.NewRecorder()
	on.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusUnauthorized || strings.Contains(deniedRec.Body.String(), secret) {
		t.Fatalf("denied = %d %s", deniedRec.Code, deniedRec.Body.String())
	}

	ok := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
	ok.Header.Set("Content-Type", "application/json")
	ok.Header.Set("Accept", "application/json, text/event-stream")
	ok.Header.Set("Authorization", "Bearer "+stack.token)
	okRec := httptest.NewRecorder()
	on.ServeHTTP(okRec, ok)
	if okRec.Code != http.StatusOK || !strings.Contains(okRec.Body.String(), "health-connect") || !strings.Contains(okRec.Body.String(), "Do not give a diagnosis") {
		t.Fatalf("initialize = %d %s", okRec.Code, okRec.Body.String())
	}
	if strings.Contains(logs.String(), stack.token) {
		t.Fatalf("log contains token: %s", logs.String())
	}
}
