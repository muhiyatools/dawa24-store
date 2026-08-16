package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RegisterAdminRoutes mounts administrative organization routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Get("/api/v1/admin/org/pending", h.ListPendingOrgs)
	r.Post("/api/v1/admin/org/{id}/approve", h.AdminApproveOrg)
	r.Post("/api/v1/admin/org/{id}/reject", h.AdminRejectOrg)
	r.Post("/api/v1/admin/org/{id}/suspend", h.AdminSuspendOrg)
}

// ListPendingOrgs returns organizations awaiting platform approval.
func (h *Handler) ListPendingOrgs(w http.ResponseWriter, r *http.Request) {
	pendingStatus := org.StatusPending
	orgs, err := h.service.ListOrganizations(r.Context(), nil, &pendingStatus, 50, 0)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"pending_organizations": orgs, "count": len(orgs)})
}

// AdminApproveOrg approves a pending organization.
func (h *Handler) AdminApproveOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}
	if err := h.service.ApproveOrganization(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// AdminRejectOrg rejects an organization application.
func (h *Handler) AdminRejectOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}
	if err := h.service.RejectOrganization(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// AdminSuspendOrg suspends an organization.
func (h *Handler) AdminSuspendOrg(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}
	if err := h.service.SuspendOrganization(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}
