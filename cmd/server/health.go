package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/cache"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// buildVersion is stamped at build time via -ldflags. It makes "which version is
// actually running" answerable from the outside, which matters most during a
// cutover when two versions may briefly coexist.
var (
	buildVersion = "dev"
	buildCommit  = "unknown"
)

type healthHandler struct {
	db         *database.DB
	cache      *cache.Cache
	ai         gateway.Client
	migrations []database.Migration
	env        string
	log        *slog.Logger
}

type componentStatus struct {
	Status  string `json:"status"` // "ok" | "degraded" | "down" | "disabled"
	Detail  string `json:"detail,omitempty"`
	Latency int64  `json:"latency_ms,omitempty"`
}

type statusResponse struct {
	Status     string                     `json:"status"`
	Version    string                     `json:"version"`
	Commit     string                     `json:"commit"`
	Env        string                     `json:"env"`
	Time       string                     `json:"time"`
	Pending    int                        `json:"pending_migrations"`
	Components map[string]componentStatus `json:"components"`
}

// live answers whether the process is alive. No dependencies are touched.
func (h *healthHandler) live(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": buildVersion,
	})
}

// ready answers whether this instance should serve traffic.
//
// The AI Gateway is explicitly excluded from the readiness decision. It is an
// enhancement, not a dependency: taking the marketplace out of rotation because
// an LLM provider is slow would be a self-inflicted outage.
func (h *healthHandler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	components := map[string]componentStatus{
		"database": check(ctx, h.db.Health),
		"cache":    check(ctx, h.cache.Health),
	}

	pending, err := h.db.PendingCount(ctx, h.migrations)
	if err != nil {
		components["migrations"] = componentStatus{Status: "down", Detail: err.Error()}
	} else if pending > 0 {
		components["migrations"] = componentStatus{
			Status: "down",
			Detail: "pending migrations; this build expects a newer schema",
		}
	} else {
		components["migrations"] = componentStatus{Status: "ok"}
	}

	status := http.StatusOK
	overall := "ok"
	for _, c := range components {
		if c.Status == "down" {
			status = http.StatusServiceUnavailable
			overall = "down"
			break
		}
	}

	if status != http.StatusOK {
		h.log.WarnContext(ctx, "readiness check failed", "components", components)
	}

	httpx.JSON(w, status, statusResponse{
		Status:     overall,
		Version:    buildVersion,
		Commit:     buildCommit,
		Env:        h.env,
		Time:       time.Now().UTC().Format(time.RFC3339),
		Pending:    pending,
		Components: components,
	})
}

// status is the operator view: everything ready reports, plus the Gateway, which
// is shown as a component but never affects the overall verdict.
func (h *healthHandler) status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	components := map[string]componentStatus{
		"database": check(ctx, h.db.Health),
		"cache":    check(ctx, h.cache.Health),
	}

	switch {
	case !h.ai.Enabled():
		components["ai_gateway"] = componentStatus{
			Status: "disabled",
			Detail: "GATEWAY_ENABLED is false; capabilities are serving deterministic fallbacks",
		}
	default:
		c := check(ctx, h.ai.Health)
		if c.Status == "down" {
			// Degraded, not down: fallbacks cover it.
			c.Status = "degraded"
		}
		components["ai_gateway"] = c
	}

	pending, _ := h.db.PendingCount(ctx, h.migrations)

	overall := "ok"
	for name, c := range components {
		if name == "ai_gateway" {
			continue
		}
		if c.Status == "down" {
			overall = "down"
		}
	}

	httpx.JSON(w, http.StatusOK, statusResponse{
		Status:     overall,
		Version:    buildVersion,
		Commit:     buildCommit,
		Env:        h.env,
		Time:       time.Now().UTC().Format(time.RFC3339),
		Pending:    pending,
		Components: components,
	})
}

func check(ctx context.Context, probe func(context.Context) error) componentStatus {
	start := time.Now()
	if err := probe(ctx); err != nil {
		return componentStatus{
			Status:  "down",
			Detail:  err.Error(),
			Latency: time.Since(start).Milliseconds(),
		}
	}
	return componentStatus{Status: "ok", Latency: time.Since(start).Milliseconds()}
}
