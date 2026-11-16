// Package middleware holds HTTP middleware for the HealthConnect API.
package middleware

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
)

// TokenLookup resolves a raw bearer token to a user id.
// A missing token returns an error that errors.Is matches notFound.
type TokenLookup interface {
	UserIDByToken(ctx context.Context, token string) (string, error)
}

// RequireAuth rejects requests that do not carry a known bearer token.
// notFound is the sentinel for an unknown token. Other errors become HTTP 500.
func RequireAuth(lookup TokenLookup, notFound error, log *slog.Logger) gin.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			log.Debug("auth rejected", "reason", "missing_bearer")
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		userID, err := lookup.UserIDByToken(c.Request.Context(), token)
		if errors.Is(err, notFound) || (userID == "" && err == nil) {
			log.Debug("auth rejected", "reason", "unknown_token")
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		if err != nil {
			log.Error("auth lookup failed")
			c.AbortWithStatusJSON(500, gin.H{"error": "internal"})
			return
		}
		c.Set(userKey, userID)
		c.Next()
	}
}

// UserID returns the authenticated user id.
func UserID(c *gin.Context) (string, bool) {
	raw, ok := c.Get(userKey)
	if !ok {
		return "", false
	}
	id, ok := raw.(string)
	return id, ok && id != ""
}

const userKey = "user_id"

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.Contains(token, " ") {
		return "", false
	}
	return token, true
}
