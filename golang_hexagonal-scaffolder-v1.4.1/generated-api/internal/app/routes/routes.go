// Package routes contains the HTTP route handlers.
//
// HealthRoute implements the v1.3.0+ probe separation:
//   - /health/live  — liveness (Go runtime only)
//   - /health/ready — readiness (DB SELECT 1)
//   - /health       — combined (backwards-compatible)
//
// P0-7.1: /health no longer returns ok when the DB is down; it probes with
// SELECT 1 via DatabaseFactory.Ping.
//
// Since 1.4.0.
package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/example/generated-api/internal/infra/database"
	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HealthRoute holds the DB factory used for readiness probes. Since 1.4.0.
type HealthRoute struct {
	log *applogger.Logger
	db  *database.Factory
}

// NewHealthRoute constructs the route. db may be nil (readiness will report
// unavailable). Since 1.4.0.
func NewHealthRoute(log *applogger.Logger, db *database.Factory) *HealthRoute {
	if log == nil {
		log = applogger.New()
	}
	return &HealthRoute{log: log, db: db}
}

// Live handles GET /health/live. Since 1.4.0.
func (h *HealthRoute) Live(w http.ResponseWriter, r *http.Request) {
	_ = h.log.Logged("HealthRoute.Live", func() error {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"probe":      "live",
			"go_version": runtime.Version(),
			"goroutines": runtime.NumGoroutine(),
		})
		return nil
	})
}

// Ready handles GET /health/ready. P0-7.1: probes DB. Since 1.4.0.
func (h *HealthRoute) Ready(w http.ResponseWriter, r *http.Request) {
	_ = h.log.Logged("HealthRoute.Ready", func() error {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if h.db == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "unavailable",
				"probe":  "ready",
				"error":  "DB not initialised",
			})
			return nil
		}
		if err := h.db.Ping(ctx); err != nil {
			h.log.Error("HealthRoute.Ready: DB probe failed", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "unavailable",
				"probe":  "ready",
				"error":  err.Error(),
			})
			return nil
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"probe":  "ready",
		})
		return nil
	})
}

// Combined handles GET /health (backwards-compatible). Since 1.4.0.
func (h *HealthRoute) Combined(w http.ResponseWriter, r *http.Request) {
	_ = h.log.Logged("HealthRoute.Combined", func() error {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		dbOK := true
		if h.db != nil {
			if err := h.db.Ping(ctx); err != nil {
				h.log.Error("HealthRoute.Combined: DB probe failed", "err", err)
				dbOK = false
			}
		} else {
			dbOK = false
		}
		status := "ok"
		code := http.StatusOK
		if !dbOK {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]any{
			"status":     status,
			"probe":      "combined",
			"db":         dbOK,
			"go_version": runtime.Version(),
		})
		return nil
	})
}

// Metrics handles GET /metrics. Since 1.4.0.
func Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

// writeJSON renders a JSON response. Since 1.4.0.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// silence unused import warnings for hexerr (kept for future typed errors).
var _ = hexerr.CodeInternal
