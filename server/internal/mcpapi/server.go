// Package mcpapi serves a read-only MCP endpoint for one user's health data.
package mcpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"com.bestony.health-connect/internal/httpapi/middleware"
	"com.bestony.health-connect/internal/store/duckdb"
	"com.bestony.health-connect/internal/store/sqlite"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const instructions = `Call list_catalog before you use a HealthKit type identifier.
Prefer query_series and summarize_range. Call query_records only when you need a raw sample.
Time windows are half-open. from is included. to is excluded.
Series buckets use local civil time. The server adds the sample time-zone offset to the UTC start.
Report the unit with each number.
Describe counts, changes, and gaps. Do not give a diagnosis, a treatment, or a medication instruction.
The token is not a tool argument. You can read only the user for this connection.`

// Deps holds the stores and the token lookup for the MCP endpoint.
type Deps struct {
	Log      *slog.Logger
	Tokens   middleware.TokenLookup
	NotFound error
	Facts    *duckdb.DB
	Meta     *sqlite.DB
}

type api struct {
	facts *duckdb.DB
	meta  *sqlite.DB
	log   *slog.Logger
}

// Handler returns the authenticated Streamable HTTP MCP handler.
func Handler(deps Deps) http.Handler {
	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	a := &api{facts: deps.Facts, meta: deps.Meta, log: log}
	server := mcp.NewServer(&mcp.Implementation{Name: "health-connect", Version: "0.1.0"}, &mcp.ServerOptions{
		Instructions: instructions,
	})
	a.register(server)
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
	return protect(deps.Tokens, deps.NotFound, log, stream)
}

func (a *api) register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_catalog",
		Description: "List stored HealthKit types for the authenticated user, plus counts of workouts, activity days, and clinical records.",
	}, a.listCatalog)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_profile",
		Description: "List stored profile attributes for the authenticated user. Values can identify the person.",
	}, a.getProfile)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_series",
		Description: "Aggregate one numeric HealthKit type. grain is hour or day. agg is sum, avg, min, or max. An hour window is at most 14 days. A day window is at most 400 days. local_bucket is civil time and has no zone suffix.",
	}, a.querySeries)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_records",
		Description: "Page raw samples for one HealthKit type. limit defaults to 50 and the maximum is 200.",
	}, a.queryRecords)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_workouts",
		Description: "Page workouts. limit defaults to 20 and the maximum is 100. Events are omitted unless include_events is true.",
	}, a.queryWorkouts)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_activity",
		Description: "List daily activity summaries. from and to are dates. The maximum span is 400 days.",
	}, a.queryActivity)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_health_records",
		Description: "List clinical records. The server does not interpret the payload. A payload longer than 8 KiB is a byte prefix and truncated is true. limit defaults to 5 and the maximum is 20.",
	}, a.queryHealthRecords)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "summarize_range",
		Description: "Summarize stored data for at most 366 days. Quantity rows include count, min, max, avg, local days, and the percent change from the previous window of equal length. delta_pct is null when the previous window has no data or a zero average. Gaps are UTC dates in the window that have no local-day bucket, at most 14. This describes stored numbers. It is not a diagnosis.",
	}, a.summarizeRange)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "compare_periods",
		Description: "Compare one numeric type across two windows. Each window is at most 366 days. agg is sum, avg, min, or max. delta is period_b minus period_a. delta_pct is null when either window has no data or period_a is zero.",
	}, a.comparePeriods)
}

func (a *api) call(ctx context.Context, tool string, debug []any, fn func() (any, error)) (any, error) {
	start := time.Now()
	out, err := fn()
	userID, _ := userFrom(ctx)
	code := ""
	status := "ok"
	if err != nil {
		status = "error"
		var te *toolError
		if errors.As(err, &te) {
			code = te.Code
		} else {
			code = "internal"
			a.log.Debug("mcp query failed", "tool", tool, "err", err.Error())
			err = fail("internal", "The query failed.")
		}
	}
	args := []any{"tool", tool, "status", status, "duration_ms", time.Since(start).Milliseconds(), "user_id", userID}
	if code != "" {
		args = append(args, "error", code)
	}
	a.log.Info("mcp", args...)
	if len(debug) > 0 && a.log.Enabled(ctx, slog.LevelDebug) {
		a.log.Debug("mcp", append([]any{"tool", tool}, debug...)...)
	}
	return out, err
}
