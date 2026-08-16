package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RegisterAdminRoutes mounts administrative promo routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Get("/api/v1/admin/promo/ads", h.AdminListAds)
	r.Post("/api/v1/admin/promo/ads/{id}/approve", h.AdminApproveAd)
	r.Post("/api/v1/admin/promo/ads/{id}/reject", h.AdminRejectAd)
	r.Get("/api/v1/admin/promo/sponsorships", h.AdminListSponsorships)
	r.Post("/api/v1/admin/promo/sponsorships/{id}/review", h.AdminReviewSponsorship)
}

func (h *Handler) AdminListAds(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	ads, err := h.service.ListActiveAds(ctx, "")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ads": ads, "count": len(ads)})
}

func (h *Handler) AdminApproveAd(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid ad ID", nil))
		return
	}
	// TODO: implement audit log and state change
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "approved", "id": strconv.FormatInt(id, 10)})
}

func (h *Handler) AdminRejectAd(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid ad ID", nil))
		return
	}
	// TODO: implement audit log and state change
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "rejected", "id": strconv.FormatInt(id, 10)})
}

func (h *Handler) AdminListSponsorships(w http.ResponseWriter, r *http.Request) {
	// TODO: implement list sponsorships in service
	httpx.JSON(w, http.StatusOK, map[string]any{"sponsorships": []any{}})
}

func (h *Handler) AdminReviewSponsorship(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid ID", nil))
		return
	}
	// TODO: implement audit log and state change
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "reviewed", "id": strconv.FormatInt(id, 10)})
}
