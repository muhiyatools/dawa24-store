package ui

import (
	"net/http"
	"strconv"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/features"

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
		ActiveTab:      tab,
		PolicyKey:      policyKey,
		SupportEmail:   "support@dawa24.eg",
		CommissionRate: "1.5",
		FeatureFlags:   features.List(),
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
			SiteName:    "دواء 24",
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
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings", http.StatusSeeOther)
		return
	}

	key := strings.TrimSpace(r.PostFormValue("key"))
	enabledStr := strings.TrimSpace(r.PostFormValue("enabled"))
	enabled := enabledStr == "true" || enabledStr == "1"

	if key == "" {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", "مفتاح الميزة غير صالح.")
		return
	}

	if err := features.GetEngine().Set(ctx, key, enabled, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "failed to toggle feature flag", "key", key, "error", err)
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", "فشل تحديث حالة الميزة.")
		return
	}

	msg := "تم تعطيل الميزة بنجاح."
	if enabled {
		msg = "تم تفعيل الميزة بنجاح."
	}
	h.redirectWithNotice(w, r, "/admin/settings?tab=features", "success", msg)
}

// AdminSettingsSubmit persists the general platform settings.
func (h *UIHandler) AdminSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", "خدمة الإعدادات غير متاحة حالياً.")
		return
	}

	supportEmail := strings.TrimSpace(r.PostFormValue("support_email"))
	commissionRate := strings.TrimSpace(r.PostFormValue("commission_rate"))

	if supportEmail == "" || !strings.Contains(supportEmail, "@") {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", "بريد الدعم الفني غير صالح.")
		return
	}
	rate, err := strconv.ParseFloat(commissionRate, 64)
	if err != nil || rate < 0 || rate > 100 {
		h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", "نسبة العمولة يجب أن تكون بين 0 و 100.")
		return
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
	}
	for _, s := range settings {
		if err := h.adminSvc.SetSetting(ctx, s); err != nil {
			h.log.ErrorContext(ctx, "save platform setting", "error", err, "key", s.Key)
			h.redirectWithNotice(w, r, "/admin/settings?tab=features", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.log.InfoContext(ctx, "platform settings updated", "support_email", supportEmail, "commission_rate", commissionRate)
	h.redirectWithNotice(w, r, "/admin/settings?tab=features", "success", "تم حفظ إعدادات المنصة بنجاح.")
}

// AdminAISettingsSubmit updates AI assistant parameters.
func (h *UIHandler) AdminAISettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "error", "خدمة الذكاء الاصطناعي غير متاحة.")
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
		h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "success", "تم حفظ إعدادات الذكاء الاصطناعي بنجاح.")
}

// AdminGatewaySettingsSubmit updates AI Gateway endpoints and parameters.
func (h *UIHandler) AdminGatewaySettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "error", "خدمة البوابة غير متاحة.")
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
		h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "error", h.safeMessage(err, langOf(r)))
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

	h.redirectWithNotice(w, r, "/admin/settings?tab=ai", "success", "تم حفظ وتحديث إعدادات بوابة الذكاء الاصطناعي بنجاح.")
}
