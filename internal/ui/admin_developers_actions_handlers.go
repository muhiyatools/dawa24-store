package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminGatewayTestConnection probes Gateway readiness live.
func (h *UIHandler) AdminGatewayTestConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	w.Header().Set("Content-Type", "application/json")

	_ = r.ParseForm()
	reqEndpoint := strings.TrimSpace(r.FormValue("endpoint_url"))
	reqUser := strings.TrimSpace(r.FormValue("admin_username"))
	if reqUser == "" {
		reqUser = "admin"
	}
	reqKey := strings.TrimSpace(r.FormValue("api_key"))

	// A credential typed into the connection test travels to the Gateway host
	// exactly like a saved one, so it is validated on the same terms.
	if err := platformadmin.ValidateAdminCredential(reqKey); err != nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "rejected",
			"message": credentialRejectionMessage(err),
		})
		return
	}

	adminClient, endpoint, _ := h.getGatewayAdminClient(ctx)
	if reqEndpoint != "" {
		endpoint = reqEndpoint
		adminClient = gateway.NewAdminClient(reqEndpoint, reqUser, reqKey)
	}

	plans, err := adminClient.ListPlans(ctx)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		msg := fmt.Sprintf(i18n.T(lang, "admin.dev.connection_failed_format"), endpoint, err)
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
			msg = fmt.Sprintf(i18n.T(lang, "admin.dev.unauthorized_format"), endpoint)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "unreachable",
			"error":   err.Error(),
			"message": msg,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "healthy",
		"message": fmt.Sprintf(i18n.T(lang, "admin.dev.connection_healthy_format"), endpoint, len(plans)),
		"count":   len(plans),
	})
}

// AdminErrorLogStatusSubmit updates the status of an error record.
func (h *UIHandler) AdminErrorLogStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", i18n.T(lang, "admin.dev.invalid_log_id"))
		return
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "RESOLVED"
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	if err := h.adminSvc.UpdateErrorLogStatus(ctx, id, status); err != nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "success", i18n.T(lang, "admin.dev.error_status_updated_success"))
}
