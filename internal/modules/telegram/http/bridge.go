// Package http serves the endpoints n8n calls.
//
// They are machine-to-machine: no cookie, no session, no CSRF token. The only
// credential is a shared secret presented as a Bearer token, and it is the
// only thing standing between the internet and "I am Telegram user N". It is
// therefore compared in constant time, against a hash, and a missing or wrong
// one is refused before a byte of the body is read.
package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/telegram"
)

// Prefix is where the bridge lives. It is registered as long-running in
// httpx, because an update carrying a question waits for the full answer.
const Prefix = "/api/v1/integrations/telegram"

const (
	maxUpdateBytes = 1 << 20
	maxReportBytes = 256 << 10
	maxReportItems = 200
)

// Bridge serves n8n.
type Bridge struct {
	svc       *telegram.Service
	tokenHash [32]byte
	log       *slog.Logger
}

// NewBridge constructs the bridge. token must be the configured shared secret;
// an empty token yields a bridge that refuses every call.
func NewBridge(svc *telegram.Service, token string, log *slog.Logger) *Bridge {
	if log == nil {
		log = slog.Default()
	}
	b := &Bridge{svc: svc, log: log.With("handler", "telegram_bridge")}
	if token != "" {
		b.tokenHash = sha256.Sum256([]byte(token))
	}
	return b
}

// RegisterRoutes mounts the bridge. Mount it OUTSIDE the session, CSRF and
// tenant middleware: none of them apply to n8n, and the CSRF check would
// refuse every call.
func (b *Bridge) RegisterRoutes(r chi.Router) {
	r.Route(Prefix, func(g chi.Router) {
		g.Use(b.authenticate)
		g.Post("/updates", b.Updates)
		g.Post("/outbox/claim", b.Claim)
		g.Post("/outbox/report", b.Report)
		g.Get("/exports/{token}", b.Export)
	})
}

// Export serves a spreadsheet Capsule produced, for n8n to send as a document.
func (b *Bridge) Export(w http.ResponseWriter, r *http.Request) {
	file, err := b.svc.Export(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		b.log.ErrorContext(r.Context(), "telegram bridge: export", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if file == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	w.Header().Set("Content-Type", file.MIMEType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename}))
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Content)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Content)
}

func (b *Bridge) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !b.authorized(r.Header.Get("Authorization")) {
			b.log.WarnContext(r.Context(), "telegram bridge: refused unauthenticated call",
				"path", r.URL.Path, "remote", r.RemoteAddr)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *Bridge) authorized(header string) bool {
	var zero [32]byte
	if b.tokenHash == zero {
		return false
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return subtle.ConstantTimeCompare(got[:], b.tokenHash[:]) == 1
}

// Updates receives one raw Telegram update and returns the messages to send.
func (b *Bridge) Updates(w http.ResponseWriter, r *http.Request) {
	var u telegram.Update
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUpdateBytes)).Decode(&u); err != nil || u.UpdateID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_update"})
		return
	}
	reply, err := b.svc.HandleUpdate(r.Context(), u)
	if err != nil {
		b.log.ErrorContext(r.Context(), "telegram bridge: handle update", "error", err, "update_id", u.UpdateID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if reply.Messages == nil {
		reply.Messages = []telegram.OutMessage{}
	}
	writeJSON(w, http.StatusOK, reply)
}

type claimRequest struct {
	Limit int `json:"limit"`
}

// Claim leases notification messages for n8n to send.
func (b *Bridge) Claim(w http.ResponseWriter, r *http.Request) {
	var req claimRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
	}
	out, err := b.svc.ClaimOutbox(r.Context(), req.Limit)
	if err != nil {
		b.log.ErrorContext(r.Context(), "telegram bridge: claim outbox", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if out == nil {
		out = []telegram.Outgoing{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

type reportRequest struct {
	Results []telegram.DeliveryResult `json:"results"`
}

// Report records the outcome of leased messages.
func (b *Bridge) Report(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBytes)).Decode(&req); err != nil || len(req.Results) > maxReportItems {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if err := b.svc.ReportDeliveries(r.Context(), req.Results); err != nil {
		b.log.ErrorContext(r.Context(), "telegram bridge: report deliveries", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"recorded": len(req.Results)})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
