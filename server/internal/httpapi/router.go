// Package httpapi is the Gin HTTP surface for HealthConnect.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"com.bestony.health-connect/internal/httpapi/handlers"
	"com.bestony.health-connect/internal/httpapi/middleware"

	"github.com/gin-gonic/gin"
)

// Deps holds the collaborators the router needs.
type Deps struct {
	Log      *slog.Logger
	Tokens   middleware.TokenLookup
	NotFound error
}

// New builds the HTTP handler.
func New(deps Deps) http.Handler {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	r := gin.New()
	r.MaxMultipartMemory = 8 << 20
	r.Use(accessLog(log), recovery(log))
	r.GET("/v1/healthz", handlers.Health)
	_ = deps.Tokens
	_ = deps.NotFound
	return r
}

func accessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		log.Info("http",
			"method", c.Request.Method,
			"route", route,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

func recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error("panic", "route", c.FullPath())
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	})
}
