package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeLookup struct {
	userID string
	err    error
}

func (f fakeLookup) UserIDByToken(context.Context, string) (string, error) {
	return f.userID, f.err
}

func TestRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	notFound := errors.New("not found")
	log := slog.New(slog.DiscardHandler)
	tests := []struct {
		name   string
		header string
		lookup fakeLookup
		want   int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "unknown", header: "Bearer nope", lookup: fakeLookup{err: notFound}, want: http.StatusUnauthorized},
		{name: "store down", header: "Bearer secret", lookup: fakeLookup{err: errors.New("db")}, want: http.StatusInternalServerError},
		{name: "ok", header: "Bearer secret", lookup: fakeLookup{userID: "user-1"}, want: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/v1/locked", RequireAuth(tt.lookup, notFound, log), func(c *gin.Context) {
				id, ok := UserID(c)
				if !ok || id != "user-1" {
					t.Errorf("user id = %q ok=%v", id, ok)
				}
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/v1/locked", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}
