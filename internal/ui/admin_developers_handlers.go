package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) getGatewayAdminClient(ctx context.Context) (*gateway.AdminClient, string, bool) {
	var endpointURL, secretKey string
	if h.adminSvc != nil {
		if aiSets, err := h.adminSvc.GetAISettings(ctx); err == nil && aiSets != nil {
			if aiSets.EndpointURL != "" {
				endpointURL = aiSets.EndpointURL
			}
			if aiSets.APIKey != "" {
				secretKey = aiSets.APIKey
			}
		}
		if endpointURL == "" || secretKey == "" {
			if gwSets, err := h.adminSvc.GetGatewaySettings(ctx); err == nil && gwSets != nil {
				if endpointURL == "" && gwSets.EndpointURL != "" {
					endpointURL = gwSets.EndpointURL
				}
				if secretKey == "" && gwSets.APIKey != "" {
					// Not gwSets.APIKey directly: AdminCredentials refuses to
					// hand back a value that does not look like a Gateway
					// credential, which is what stops a database password
					// stored in this field from being sent to the Gateway host.
					if user, pass := gwSets.AdminCredentials(); pass != "" {
						if user != "" && user != "admin" {
							secretKey = user + ":" + pass
						} else {
							secretKey = pass
						}
					}
				}
			}
		}
	}
	if endpointURL == "" {
		endpointURL = "https://api.muhiya.com"
	}
	client := gateway.NewAdminClient(endpointURL, "", secretKey)
	return client, endpointURL, secretKey != ""
}

// AdminDevelopersPage renders the unified developer portal with 4 tabs:
// 1. SQL Console, 2. AI Gateway, 3. Error Diagnostics, 4. Audit Trail.
func (h *UIHandler) AdminDevelopersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "sql"
	}

	var gateway *platformadmin.GatewaySettings
	var sqlLogs []*platformadmin.SQLLog
	var errorLogs []*platformadmin.ErrorLog
	var auditEntries []*platformadmin.AuditEntry

	values := pages.AdminDevelopersValues{
		ActiveTab:         tab,
		ErrorLevelFilter:  r.URL.Query().Get("err_level"),
		ErrorStatusFilter: r.URL.Query().Get("err_status"),
		ErrorSearch:       r.URL.Query().Get("err_q"),
	}

	if h.adminSvc != nil {
		gw, _ := h.adminSvc.GetGatewaySettings(ctx)
		gateway = gw
		if gateway == nil {
			gateway = &platformadmin.GatewaySettings{
				EndpointURL: "https://api.muhiya.com",
				IsActive:    true,
			}
		}

		sl, _ := h.adminSvc.ListSQLLogs(ctx, 30, 0)
		sqlLogs = sl

		filter := platformadmin.ErrorLogFilter{
			Level:  values.ErrorLevelFilter,
			Status: values.ErrorStatusFilter,
			Search: values.ErrorSearch,
			Limit:  50,
			Offset: 0,
		}
		el, _, _ := h.adminSvc.ListErrorLogs(ctx, filter)
		errorLogs = el

		tot, crit, unres, affUsers, _ := h.adminSvc.GetErrorDiagnosticsMetrics(ctx)
		values.ErrorMetrics.Total = tot
		values.ErrorMetrics.Critical24h = crit
		values.ErrorMetrics.Unresolved = unres
		values.ErrorMetrics.AffectedUsers = affUsers
		ae, _ := h.adminSvc.ListAuditLog(ctx, 50, 0)
		for _, e := range ae {
			localizeAuditEntry(e, lang)
		}
		auditEntries = ae

		ai, _ := h.adminSvc.GetAISettings(ctx)
		if ai == nil {
			ai = &platformadmin.AISettings{
				EndpointURL: "https://api.muhiya.com",
				IsActive:    true,
			}
		}
		values.AISettings = ai
	}

	if gateway == nil {
		gateway = &platformadmin.GatewaySettings{
			EndpointURL: "https://api.muhiya.com",
			IsActive:    true,
		}
	}

	values.GatewaySettings = gateway
	values.SQLLogs = sqlLogs
	values.ErrorLogs = errorLogs
	values.AuditEntries = auditEntries

	h.renderPage(ctx, w, "render admin developers page", pages.AdminDevelopersPage(values, lang, dir))
}

// AdminSQLExecuteSubmit executes a SQL query from the Developer SQL Console and returns JSON.
func (h *UIHandler) AdminSQLExecuteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	w.Header().Set("Content-Type", "application/json")

	if h.adminSvc == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": i18n.T(lang, "admin.dev.admin_service_unavailable")})
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if query == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": i18n.T(lang, "admin.dev.empty_sql_query")})
		return
	}

	actor, ok := authctx.From(ctx)
	var actorID *int64
	actorName := "System Admin"
	if ok && actor.UserID > 0 {
		actorID = &actor.UserID
		if actor.Name != "" {
			actorName = actor.Name
		}
	}

	res, err := h.adminSvc.ExecuteSQL(ctx, actorID, actorName, query)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(res)
}

// AdminDeveloperAISettingsSubmit updates AI Gateway settings from the Developer section.
func (h *UIHandler) AdminDeveloperAISettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	endpoint := strings.TrimSpace(r.FormValue("endpoint_url"))
	adminUser := strings.TrimSpace(r.FormValue("admin_username"))
	if adminUser == "" {
		adminUser = "admin"
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	isActive := r.FormValue("is_active") == "true"
	systemPrompt := strings.TrimSpace(r.FormValue("system_prompt"))

	// If apiKey is empty in the submission, preserve existing saved key
	if apiKey == "" {
		if existingGW, _ := h.adminSvc.GetGatewaySettings(ctx); existingGW != nil && existingGW.APIKey != "" {
			apiKey = existingGW.APIKey
		}
	}
	if apiKey == "" {
		if existingAI, _ := h.adminSvc.GetAISettings(ctx); existingAI != nil && existingAI.APIKey != "" {
			apiKey = existingAI.APIKey
		}
	}

	// Format combined credentials if username is custom
	combinedKey := apiKey
	if apiKey != "" && !strings.Contains(apiKey, ":") && adminUser != "admin" {
		combinedKey = adminUser + ":" + apiKey
	}

	// Load and mutate rather than rebuild. The struct also carries the virtual
	// key provisioned from these very credentials, plus the model choices;
	// constructing a fresh one here silently discarded all of it and switched
	// AI back off every time an operator pressed save.
	gw, err := h.adminSvc.GetGatewaySettings(ctx)
	if err != nil || gw == nil {
		gw = &platformadmin.GatewaySettings{Environment: "production"}
	}

	credentialsChanged := gw.EndpointURL != endpoint || gw.APIKey != combinedKey
	gw.EndpointURL = endpoint
	gw.APIKey = combinedKey
	gw.IsActive = isActive
	if gw.Environment == "" {
		gw.Environment = "production"
	}
	if model := strings.TrimSpace(r.FormValue("fast_model")); model != "" {
		gw.FastModel = model
	}
	if model := strings.TrimSpace(r.FormValue("quality_model")); model != "" {
		gw.QualityModel = model
	}
	if plan := strings.TrimSpace(r.FormValue("ai_plan_id")); plan != "" {
		gw.AIPlanID = plan
	}

	// New credentials invalidate the key they issued: it may belong to a
	// different Gateway entirely. Clearing it makes the next AI call provision
	// a fresh one against the endpoint now configured.
	if credentialsChanged {
		gw.VirtualKey, gw.AIUserID = "", ""
	}

	if err := h.adminSvc.SaveGatewaySettings(ctx, gw); err != nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", h.safeMessage(err, lang))
		return
	}
	if h.gatewayKeys != nil {
		h.gatewayKeys.Invalidate()
	}

	// Also sync to AISettings
	ai, _ := h.adminSvc.GetAISettings(ctx)
	if ai == nil {
		ai = &platformadmin.AISettings{}
	}
	ai.EndpointURL = endpoint
	ai.APIKey = combinedKey
	ai.IsActive = isActive
	if systemPrompt != "" {
		ai.SystemPrompt = systemPrompt
	}
	_ = h.adminSvc.SaveAISettings(ctx, ai)

	h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "success", i18n.T(lang, "admin.dev.saved_ai_settings_success"))
}

// AdminAIFetchModelsAPI contacts the AI gateway to list available models live.
func (h *UIHandler) AdminAIFetchModelsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"models": []string{
		"assistant.primary",
		"assistant.attachment",
		"assistant.transcribe",
	}})
}

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
