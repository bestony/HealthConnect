package mcpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"com.bestony.health-connect/internal/httpapi/middleware"
)

type userKey struct{}

func withUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey{}, userID)
}

func userFrom(ctx context.Context) (string, error) {
	id, _ := ctx.Value(userKey{}).(string)
	if id == "" {
		return "", fail("internal", "The query failed.")
	}
	return id, nil
}

func protect(lookup middleware.TokenLookup, notFound error, log *slog.Logger, next http.Handler) http.Handler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := middleware.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			log.Debug("mcp auth rejected", "reason", "missing_bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := lookup.UserIDByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, notFound) {
				log.Debug("mcp auth rejected", "reason", "unknown_token")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			log.Error("mcp auth lookup failed")
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if userID == "" {
			log.Debug("mcp auth rejected", "reason", "unknown_token")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), userID)))
	})
}
