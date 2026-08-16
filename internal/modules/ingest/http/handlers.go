package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes ingest HTTP endpoints.
type Handler struct {
	service *ingest.Service
	log     *slog.Logger
}

// NewHandler creates an ingest HTTP handler.
func NewHandler(service *ingest.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers ingest routes on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/ingest/uploads", h.RegisterUpload)
	r.Post("/api/v1/ingest/sessions", h.StartSession)
	r.Get("/api/v1/ingest/sessions/{id}", h.GetSession)
}

// RegisterUpload registers a file uploaded to S3/MinIO.
func (h *Handler) RegisterUpload(w http.ResponseWriter, r *http.Request) {
	var f ingest.FileUpload
	if err := httpx.DecodeJSON(w, r, &f); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.RegisterUpload(r.Context(), &f)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

type StartSessionRequest struct {
	FileUploadID int64    `json:"file_upload_id"`
	Headers      []string `json:"headers"`
	MinScore     float64  `json:"min_score"`
}

// StartSession creates a session and runs column detection.
func (h *Handler) StartSession(w http.ResponseWriter, r *http.Request) {
	var req StartSessionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	session, err := h.service.StartSession(r.Context(), req.FileUploadID, req.Headers, req.MinScore)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, session)
}

// GetSession retrieves the progress of an import session.
func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid session ID", nil))
		return
	}

	session, err := h.service.GetSessionProgress(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, session)
}
