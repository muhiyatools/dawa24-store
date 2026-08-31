package ui

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) AdminSettingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "features"
	}
	policyKey := strings.TrimSpace(r.URL.Query().Get("key"))
	if policyKey == "" {
		policyKey = "terms"
	}

	values := pages.AdminSettingsValues{
		ActiveTab:                 tab,
		PolicyKey:                 policyKey,
		SupportEmail:              "support@dawa24.eg",
		CommissionRate:            "1.5",
		SessionIdleTimeoutMinutes: "30",
		FeatureFlags:              features.List(),
	}

	if h.adminSvc != nil {
		if s, err := h.adminSvc.GetSetting(ctx, settingSupportEmail); err == nil && s != nil {
			if v, ok := s.Value["value"].(string); ok && v != "" {
				values.SupportEmail = v
			}
		}
		if s, err := h.adminSvc.GetSetting(ctx, settingCommissionRate); err == nil && s != nil {
			if v, ok := s.Value["value"].(string); ok && v != "" {
				values.CommissionRate = v
			}
		}
		if s, err := h.adminSvc.GetSetting(ctx, settingSessionIdleTimeout); err == nil && s != nil {
			if v, ok := s.Value["value"].(string); ok && v != "" {
				values.SessionIdleTimeoutMinutes = v
			} else if vNum, ok := s.Value["value"].(float64); ok && vNum > 0 {
				values.SessionIdleTimeoutMinutes = strconv.Itoa(int(vNum))
			}
		}

		values.GatewaySettings, _ = h.adminSvc.GetGatewaySettings(ctx)
		values.SiteSettings, _ = h.adminSvc.GetSiteSettings(ctx)
		values.Policies, _ = h.adminSvc.ListPolicyVersions(ctx, "")
		values.ActivePolicyKey = policyKey
		if ap, err := h.adminSvc.GetActivePolicy(ctx, policyKey); err != nil {
			h.log.WarnContext(ctx, "admin settings: active policy", "key", policyKey, "error", err)
		} else {
			values.ActivePolicy = ap
		}
	}

	if values.GatewaySettings == nil || values.GatewaySettings.EndpointURL == "" {
		values.GatewaySettings = &platformadmin.GatewaySettings{
			EndpointURL:    "https://api.muhiya.com",
			Environment:    "production",
			TimeoutSeconds: 30,
			IsActive:       true,
		}
	}
	if values.SiteSettings == nil {
		values.SiteSettings = &platformadmin.SiteSettings{
			SiteName:    "Dawa24",
			SocialLinks: make(map[string]string),
		}
	} else if values.SiteSettings.SocialLinks == nil {
		values.SiteSettings.SocialLinks = make(map[string]string)
	}

	if h.billSvc != nil {
		values.PlatformPaymentMethods, _ = h.billSvc.ListPlatformPaymentMethods(ctx, false)
	}

	h.renderPage(ctx, w, "render admin settings page", pages.AdminSettings(values, lang, dir))
}

// AdminFeatureToggleSubmit toggles a platform feature flag in real-time.
func (h *UIHandler) AdminFeatureToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings", http.StatusSeeOther)
		return
	}

	key := strings.TrimSpace(r.PostFormValue("key"))
	enabledStr := strings.TrimSpace(r.PostFormValue("enabled"))
	enabled := enabledStr == "true" || enabledStr == "1"

	if key == "" {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", i18n.T(lang, "admin.settings.invalid_feature_key"))
		return
	}

	if err := features.GetEngine().Set(ctx, key, enabled, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "failed to toggle feature flag", "key", key, "error", err)
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", i18n.T(lang, "admin.settings.feature_save_failed"))
		return
	}

	msg := i18n.T(lang, "admin.settings.feature_disabled_success")
	if enabled {
		msg = i18n.T(lang, "admin.settings.feature_enabled_success")
	}
	h.redirectWithNotice(w, r, "/admin/settings?tab=features", "success", msg)
}

// AdminSettingsSubmit persists the general platform settings.
func (h *UIHandler) AdminSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", i18n.T(lang, "admin.settings.service_unavailable"))
		return
	}

	supportEmail := strings.TrimSpace(r.PostFormValue("support_email"))
	commissionRate := strings.TrimSpace(r.PostFormValue("commission_rate"))
	sessionIdleTimeout := strings.TrimSpace(r.PostFormValue("session_idle_timeout_minutes"))

	if supportEmail == "" || !strings.Contains(supportEmail, "@") {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", i18n.T(lang, "admin.settings.invalid_email"))
		return
	}
	rate, err := strconv.ParseFloat(commissionRate, 64)
	if err != nil || rate < 0 || rate > 100 {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", i18n.T(lang, "admin.settings.invalid_commission"))
		return
	}
	idleMins, err := strconv.Atoi(sessionIdleTimeout)
	if err != nil || idleMins < 1 || idleMins > 10080 {
		idleMins = 30
	}

	settings := []*platformadmin.SystemSetting{
		{
			Key:         settingSupportEmail,
			Value:       map[string]any{"value": supportEmail},
			Description: "Support and notification email address",
			IsPublic:    true,
		},
		{
			Key:         settingCommissionRate,
			Value:       map[string]any{"value": commissionRate},
			Description: "Default platform commission rate, percent",
			IsPublic:    false,
		},
		{
			Key:         settingSessionIdleTimeout,
			Value:       map[string]any{"value": strconv.Itoa(idleMins)},
			Description: "Session idle timeout in minutes before automatic logout",
			IsPublic:    true,
		},
	}
	for _, s := range settings {
		if err := h.adminSvc.SetSetting(ctx, s); err != nil {
			h.log.ErrorContext(ctx, "save platform setting", "error", err, "key", s.Key)
			h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", h.safeMessage(err, lang))
			return
		}
	}

	if h.idSvc != nil {
		h.idSvc.SetIdleTimeout(time.Duration(idleMins) * time.Minute)
	}
	if ss, err := h.adminSvc.GetSiteSettings(ctx); err == nil && ss != nil {
		ss.SessionIdleTimeoutMinutes = idleMins
		_ = h.adminSvc.SaveSiteSettings(ctx, ss)
	}
	InvalidateSiteSettingsCache()

	h.log.InfoContext(ctx, "platform settings updated", "support_email", supportEmail, "commission_rate", commissionRate, "session_idle_timeout_minutes", idleMins)
	h.redirectWithNotice(w, r, "/admin/settings?tab=features", "success", i18n.T(lang, "admin.settings.saved_general_success"))
}

// AdminAISettingsSubmit updates AI assistant parameters.
func (h *UIHandler) AdminAISettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "error", i18n.T(lang, "admin.settings.service_unavailable"))
		return
	}

	temp, _ := strconv.ParseFloat(r.FormValue("temperature"), 64)
	maxTokens, _ := strconv.Atoi(r.FormValue("max_tokens"))
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	ai := &platformadmin.AISettings{
		APIKey:       strings.TrimSpace(r.FormValue("api_key")),
		EndpointURL:  strings.TrimSpace(r.FormValue("endpoint_url")),
		Temperature:  temp,
		MaxTokens:    maxTokens,
		SystemPrompt: strings.TrimSpace(r.FormValue("system_prompt")),
		IsActive:     r.FormValue("is_active") == "true",
	}

	if err := h.adminSvc.SaveAISettings(ctx, ai); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "success", i18n.T(lang, "admin.settings.saved_ai_success"))
}

// AdminGatewaySettingsSubmit updates AI Gateway endpoints and parameters.
func (h *UIHandler) AdminGatewaySettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "error", i18n.T(lang, "admin.settings.service_unavailable"))
		return
	}

	endpoint := strings.TrimSpace(r.FormValue("endpoint_url"))
	if endpoint == "" {
		endpoint = "https://api.muhiya.com"
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	systemPrompt := strings.TrimSpace(r.FormValue("system_prompt"))
	isActive := r.FormValue("is_active") == "true"

	gw := &platformadmin.GatewaySettings{
		EndpointURL:    endpoint,
		Environment:    "production",
		TimeoutSeconds: 30,
		APIKey:         apiKey,
		IsActive:       isActive,
	}

	if err := h.adminSvc.SaveGatewaySettings(ctx, gw); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "error", h.safeMessage(err, lang))
		return
	}

	ai := &platformadmin.AISettings{
		APIKey:       apiKey,
		EndpointURL:  endpoint,
		Temperature:  0.7,
		MaxTokens:    2048,
		SystemPrompt: systemPrompt,
		IsActive:     isActive,
	}
	_ = h.adminSvc.SaveAISettings(ctx, ai)

	h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "success", i18n.T(lang, "admin.settings.saved_system_prompt_success"))
}
