// Package httpapi is the Gin HTTP surface for HealthConnect.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"com.bestony.health-connect/internal/export"
	"com.bestony.health-connect/internal/httpapi/handlers"
	"com.bestony.health-connect/internal/httpapi/middleware"
	"com.bestony.health-connect/internal/ingest"
	"com.bestony.health-connect/internal/mcpapi"
	"com.bestony.health-connect/internal/store/duckdb"
	"com.bestony.health-connect/internal/store/sqlite"

	"github.com/gin-gonic/gin"
)

// Deps holds the collaborators the router needs.
type Deps struct {
	Log        *slog.Logger
	Tokens     middleware.TokenLookup
	NotFound   error
	Imports    *ingest.Service
	Exports    *export.Service
	Facts      *duckdb.DB
	Meta       *sqlite.DB
	MaxUpload  int64
	MCPEnabled bool
}

// New builds the HTTP handler.
func New(deps Deps) http.Handler {
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.MaxMultipartMemory = 8 << 20
	r.Use(accessLog(log), recovery(log))
	r.GET("/v1/healthz", handlers.Health)
	if deps.MCPEnabled && deps.Facts != nil && deps.Meta != nil && deps.Tokens != nil {
		mcpHandler := mcpapi.Handler(mcpapi.Deps{
			Log: log, Tokens: deps.Tokens, NotFound: deps.NotFound, Facts: deps.Facts, Meta: deps.Meta,
		})
		r.Any("/mcp", gin.WrapH(mcpHandler))
		log.Info("mcp enabled", "path", "/mcp")
	}
	if deps.Imports != nil {
		api := handlers.Imports{Svc: deps.Imports, MaxUpload: deps.MaxUpload}
		auth := middleware.RequireAuth(deps.Tokens, deps.NotFound, log)
		v1 := r.Group("/v1", auth)
		v1.POST("/imports", api.Create)
		v1.GET("/imports", api.List)
		v1.GET("/imports/:id", api.Get)
		v1.POST("/imports/:id/retry", api.Retry)
		if deps.Exports != nil {
			exp := handlers.Exports{Svc: deps.Exports}
			v1.POST("/exports", exp.Create)
			v1.POST("/exports/:id/batches", exp.Batch)
			v1.POST("/exports/:id/complete", exp.Complete)
			v1.POST("/exports/:id/abort", exp.Abort)
		}
		if deps.Facts != nil && deps.Meta != nil {
			query := handlers.Query{Facts: deps.Facts, Meta: deps.Meta}
			v1.GET("/catalog", query.Catalog)
			v1.GET("/series", query.Series)
			v1.GET("/records", query.Records)
			v1.GET("/workouts", query.Workouts)
			v1.GET("/activity-summaries", query.Activity)
			v1.DELETE("/data", query.DeleteData)
		}
	}
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
