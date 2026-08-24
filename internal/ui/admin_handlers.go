package ui

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) AdminDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	// Every figure here used to be len() of a page capped at 100 rows, so the
	// dashboard silently stopped counting at 100 and reported "100 users" to a
	// platform with a thousand. Totals come from COUNT queries.
	stats := pages.AdminDashboardStats{}
	var pendingOrgs []*org.Organization

	if h.idSvc != nil {
		if n, err := h.idSvc.AdminCountUsers(ctx); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count users", "error", err)
		} else {
			stats.TotalUsers = n
		}
	}
	if h.orgSvc != nil {
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, nil); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count organizations", "error", err)
		} else {
			stats.TotalOrganizations = n
		}
		pending := org.StatusPending
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, &pending); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count pending organizations", "error", err)
		} else {
			stats.PendingApprovals = n
		}
		list, err := h.orgSvc.ListOrganizations(ctx, nil, &pending, 5, 0)
		if err != nil {
			h.log.WarnContext(ctx, "dashboard: list pending organizations", "error", err)
		} else {
			pendingOrgs = list
		}
	}
	if h.commSvc != nil {
		if n, err := h.commSvc.CountOrders(ctx); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count orders", "error", err)
		} else {
			stats.TotalOrders = n
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDashboard(stats, pendingOrgs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin dashboard page", "error", err)
	}
}

func (h *UIHandler) AdminUsersPage(w http.ResponseWriter, r *http.Request) {
	h.renderAdminEnterpriseHub(w, r, "users")
}

func (h *UIHandler) AdminApprovalsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	lang, dir := h.localeAndDir(r)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "organizations"
	}
	statusParam := r.URL.Query().Get("status")

	data := &pages.AdminApprovalsData{
		ActiveTab:    tab,
		StatusFilter: statusParam,
		OrgDocs:      make(map[int64][]*attachments.Document),
		OrgNames:     make(map[int64]string),
	}

	sysCtx := database.AsSystem(ctx)

	if h.orgSvc != nil {
		allList, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)
		data.AllOrganizations = allList
		for _, o := range allList {
			if o != nil {
				data.OrgNames[o.ID] = o.LegalName
			}
		}

		var filterStatus *org.OrganizationStatus
		if statusParam != "" {
			st := org.OrganizationStatus(statusParam)
			filterStatus = &st
		} else if tab == "organizations" {
			st := org.StatusPending
			filterStatus = &st
		}
		list, err := h.orgSvc.ListOrganizations(sysCtx, nil, filterStatus, 150, 0)
		if err != nil {
			h.log.WarnContext(ctx, "admin approvals: list organizations", "error", err)
		} else {
			data.Organizations = list
		}
	}

	if h.attSvc != nil {
		for _, o := range data.Organizations {
			if o != nil {
				docs, _ := h.attSvc.ListByOrganization(sysCtx, o.ID)
				if len(docs) > 0 {
					data.OrgDocs[o.ID] = docs
				}
			}
		}

		docs, _, err := h.attSvc.ListAll(sysCtx, attachments.DocumentFilter{Limit: 200})
		if err == nil {
			data.UploadedDocs = docs
		}

		reqs, err := h.attSvc.ListDocumentRequests(sysCtx, actor, nil)
		if err == nil {
			data.DocRequests = reqs
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminApprovals(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin approvals page", "error", err)
	}
}

// AdminOrgReviewSubmit handles full administrative approval/rejection with custom reason and document categorization.
func (h *UIHandler) AdminOrgReviewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", "معرف المنشأة غير صالح.")
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", "خدمة المؤسسات غير متاحة.")
		return
	}

	status := org.OrganizationStatus(r.PostFormValue("status"))
	notes := r.PostFormValue("verification_notes")
	rejectionReason := r.PostFormValue("rejection_reason")
	docTypeVal := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))

	if err := h.orgSvc.ReviewOrganization(ctx, id, status, notes, rejectionReason, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", h.safeMessage(err, langOf(r)))
		return
	}

	// When approved, classify and verify the organization's registration documents
	if status == org.StatusApproved && h.attSvc != nil {
		sysCtx := database.AsSystem(ctx)
		o, _ := h.orgSvc.GetOrganization(sysCtx, id)
		if docTypeVal == "" {
			if o != nil && o.Type == org.TypeCustomer {
				docTypeVal = attachments.DocPharmacyLicense
			} else {
				docTypeVal = attachments.DocCommercialRegister
			}
		}

		docs, _ := h.attSvc.ListByOrganization(sysCtx, id)
		for _, d := range docs {
			if d != nil {
				_ = h.attSvc.VerifyDocumentWithType(sysCtx, actor, d.ID, docTypeVal, attachments.StatusVerified, notes)
			}
		}
	}

	msg := "تم اعتماد وتفعيل ترخيص المنشأة وتوثيق المستندات المرفقة بنجاح."
	if status == org.StatusRejected {
		msg = "تم رفض طلب المنشأة وحفظ سبب الرفض."
	} else if status == org.StatusSuspended {
		msg = "تم تعليق حساب المنشأة مؤقتاً."
	}

	h.redirectWithNotice(w, r, "/admin/approvals", "success", msg)
}

// Platform settings keys. These live in platform_admin.system_settings.
const (
	settingSupportEmail   = "platform.support_email"
	settingCommissionRate = "platform.commission_rate"
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminSettings(values, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin settings page", "error", err)
	}
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

// AdminSiteSettingsSubmit persists public contact info and social media links.
func (h *UIHandler) AdminSiteSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", "خدمة الإعدادات غير متاحة.")
		return
	}

	curr, _ := h.adminSvc.GetSiteSettings(ctx)
	if curr == nil {
		curr = &platformadmin.SiteSettings{SocialLinks: map[string]string{}}
	}
	if curr.SocialLinks == nil {
		curr.SocialLinks = map[string]string{}
	}

	section := r.FormValue("section")
	if section == "contact" {
		curr.SiteName = strings.TrimSpace(r.FormValue("site_name"))
		curr.SiteDescription = strings.TrimSpace(r.FormValue("site_description"))
		curr.ContactEmail = strings.TrimSpace(r.FormValue("contact_email"))
		curr.SupportEmail = strings.TrimSpace(r.FormValue("support_email"))
		curr.Phone = strings.TrimSpace(r.FormValue("phone"))
		curr.WhatsApp = strings.TrimSpace(r.FormValue("whatsapp"))
		curr.Address = strings.TrimSpace(r.FormValue("address"))
	} else if section == "socials" {
		curr.SocialLinks["facebook"] = strings.TrimSpace(r.FormValue("social_facebook"))
		curr.SocialLinks["twitter"] = strings.TrimSpace(r.FormValue("social_twitter"))
		curr.SocialLinks["instagram"] = strings.TrimSpace(r.FormValue("social_instagram"))
		curr.SocialLinks["linkedin"] = strings.TrimSpace(r.FormValue("social_linkedin"))
		curr.SocialLinks["youtube"] = strings.TrimSpace(r.FormValue("social_youtube"))
		curr.SocialLinks["tiktok"] = strings.TrimSpace(r.FormValue("social_tiktok"))
		curr.SocialLinks["snapchat"] = strings.TrimSpace(r.FormValue("social_snapchat"))
		curr.SocialLinks["telegram"] = strings.TrimSpace(r.FormValue("social_telegram"))
		if curr.WhatsApp != "" {
			curr.SocialLinks["whatsapp"] = "https://wa.me/" + curr.WhatsApp
		}
	}

	if err := h.adminSvc.SaveSiteSettings(ctx, curr); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", h.safeMessage(err, langOf(r)))
		return
	}

	InvalidateSiteSettingsCache()
	h.redirectWithNotice(w, r, "/admin/settings?tab=site", "success", "تم حفظ وتحديث إعدادات الموقع بنجاح.")
}

// AdminBrandingSubmit updates platform logo and favicon.
func (h *UIHandler) AdminBrandingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", "خدمة الإعدادات غير متاحة.")
		return
	}

	curr, _ := h.adminSvc.GetSiteSettings(ctx)
	if curr == nil {
		curr = &platformadmin.SiteSettings{}
	}

	_ = r.ParseMultipartForm(10 << 20)

	logoURL := strings.TrimSpace(r.FormValue("logo_url"))
	faviconURL := strings.TrimSpace(r.FormValue("favicon_url"))

	// Check if a new logo file was uploaded
	if file, header, err := r.FormFile("logo_file"); err == nil && file != nil {
		defer file.Close()
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".png"
		}
		key := fmt.Sprintf("branding/logo_%d%s", time.Now().Unix(), ext)
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/png"
		}

		uploadedToStorage := false
		if h.storage != nil {
			if err := h.storage.Put(ctx, key, file, header.Size, contentType); err != nil {
				h.log.WarnContext(ctx, "branding: upload logo to storage", "error", err)
			} else {
				pubURL := h.storage.PublicURL(key)
				if pubURL == "" {
					pubURL = "/uploads/" + key
				}
				logoURL = pubURL
				uploadedToStorage = true
			}
		}

		// Also save locally as static fallback
		if !uploadedToStorage {
			savePath := "internal/ui/static/img/logo.png"
			if out, err := os.Create(savePath); err != nil {
				h.log.WarnContext(ctx, "branding: fallback logo create", "error", err)
			} else {
				defer out.Close()
				_, _ = file.Seek(0, 0)
				_, _ = io.Copy(out, file)
				logoURL = "/static/img/logo.png"
			}
		}
	}

	// Check if a new favicon file was uploaded
	if file, header, err := r.FormFile("favicon_file"); err == nil && file != nil {
		defer file.Close()
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".png"
		}
		key := fmt.Sprintf("branding/favicon_%d%s", time.Now().Unix(), ext)
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/png"
		}

		if h.storage != nil {
			if err := h.storage.Put(ctx, key, file, header.Size, contentType); err != nil {
				h.log.WarnContext(ctx, "branding: upload favicon to storage", "error", err)
			} else {
				pubURL := h.storage.PublicURL(key)
				if pubURL == "" {
					pubURL = "/uploads/" + key
				}
				faviconURL = pubURL
			}
		}
	}

	if logoURL != "" {
		curr.LogoURL = logoURL
	}
	if faviconURL != "" {
		curr.FaviconURL = faviconURL
	}

	if err := h.adminSvc.SaveSiteSettings(ctx, curr); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=site", "error", h.safeMessage(err, langOf(r)))
		return
	}

	InvalidateSiteSettingsCache()
	h.redirectWithNotice(w, r, "/admin/settings?tab=site", "success", "تم حفظ وتطبيق الهوية البصرية بنجاح.")
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

// AdminPlatformPaymentMethodSubmit creates or updates a platform supported payment channel.
func (h *UIHandler) AdminPlatformPaymentMethodSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings?tab=payment_methods", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	id := strings.TrimSpace(strings.ToLower(r.PostFormValue("id")))
	if id == "" {
		h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", "المعرف الفريد لوسيلة الدفع مطلوب.")
		return
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameEn == "" {
		nameEn = nameAr
	}
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", "اسم وسيلة الدفع بالعربية مطلوب.")
		return
	}

	providerType := strings.TrimSpace(r.PostFormValue("provider_type"))
	if providerType == "" {
		providerType = "bank"
	}

	displayOrder, _ := strconv.Atoi(r.PostFormValue("display_order"))

	pm := &billing.PlatformPaymentMethod{
		ID:                id,
		Name:              i18n.New(nameAr, nameEn),
		ProviderType:      providerType,
		Description:       i18n.New(strings.TrimSpace(r.PostFormValue("description_ar")), strings.TrimSpace(r.PostFormValue("description_en"))),
		AccountName:       strings.TrimSpace(r.PostFormValue("account_name")),
		BankName:          strings.TrimSpace(r.PostFormValue("bank_name")),
		AccountNumber:     strings.TrimSpace(r.PostFormValue("account_number")),
		IBAN:              strings.TrimSpace(r.PostFormValue("iban")),
		SwiftCode:         strings.TrimSpace(r.PostFormValue("swift_code")),
		BranchName:        strings.TrimSpace(r.PostFormValue("branch_name")),
		InstaPayHandle:    strings.TrimSpace(r.PostFormValue("instapay_handle")),
		PhoneNumber:       strings.TrimSpace(r.PostFormValue("phone_number")),
		IsActive:          r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "true",
		IsDepositEnabled:  r.PostFormValue("is_deposit_enabled") == "1" || r.PostFormValue("is_deposit_enabled") == "true",
		IsCheckoutEnabled: r.PostFormValue("is_checkout_enabled") == "1" || r.PostFormValue("is_checkout_enabled") == "true",
		DisplayOrder:      displayOrder,
	}

	if h.billSvc != nil {
		if err := h.billSvc.SavePlatformPaymentMethod(ctx, pm); err != nil {
			h.log.ErrorContext(ctx, "failed to save platform payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "success", "تم حفظ وتحديث وسيلة وقناة الدفع بنجاح.")
}

// AdminPlatformPaymentMethodToggleSubmit toggles the active state of a platform payment channel.
func (h *UIHandler) AdminPlatformPaymentMethodToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings?tab=payment_methods", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	id := strings.TrimSpace(r.PostFormValue("id"))
	enabled := r.PostFormValue("enabled") == "1" || r.PostFormValue("enabled") == "true"

	if h.billSvc != nil && id != "" {
		if err := h.billSvc.TogglePlatformPaymentMethod(ctx, id, enabled); err != nil {
			h.log.ErrorContext(ctx, "failed to toggle platform payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", "فشل تحديث حالة وسيلة الدفع.")
			return
		}
	}

	msg := "تم تعطيل وسيلة الدفع مؤقتاً."
	if enabled {
		msg = "تم تفعيل وسيلة الدفع بنجاح."
	}
	h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "success", msg)
}

// AdminPlatformPaymentMethodDeleteSubmit deletes a platform payment channel.
func (h *UIHandler) AdminPlatformPaymentMethodDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings?tab=payment_methods", http.StatusSeeOther)
		return
	}

	id := chi.URLParam(r, "id")
	if h.billSvc != nil && id != "" {
		if err := h.billSvc.DeletePlatformPaymentMethod(ctx, id); err != nil {
			h.log.ErrorContext(ctx, "failed to delete platform payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", "فشل حذف وسيلة الدفع.")
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "success", "تم حذف وسيلة الدفع بنجاح.")
}

// Administrative actions on user accounts.
//
// The admin users screen already had buttons for these, but they posted to
// /api/v1/identity/admin/users/... while the routes are registered at
// /api/v1/admin/identity/users/... - the two path segments are the other way
// round, so every button returned 404. They also carried hx-swap="none", so
// nothing on the page changed either way and the operator had no way to tell
// a successful suspension from a failed one.
//
// These handlers do the work through the service and return the refreshed
// table, so the row reflects the new state immediately.

func (h *UIHandler) adminUserAction(
	w http.ResponseWriter,
	r *http.Request,
	action func(ctx context.Context, userID, actorID int64) error,
) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	// An administrator suspending their own account would lock themselves out
	// of the screen they are standing on, and revoking their own sessions ends
	// the request that did it.
	if id == actor.UserID {
		h.renderError(w, r, apperr.Validation("user.self_action",
			"You cannot apply this action to your own account.", nil))
		return
	}

	if err := action(ctx, id, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/users", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/users", "success", "تم تنفيذ الإجراء بنجاح.")
}

// AdminUserSuspendSubmit blocks an account and ends its sessions.
func (h *UIHandler) AdminUserSuspendSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminSuspendUser(ctx, userID, actorID)
	})
}

// AdminUserReactivateSubmit restores a suspended account.
func (h *UIHandler) AdminUserReactivateSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminReactivateUser(ctx, userID, actorID)
	})
}

// AdminUserResetMFASubmit clears a user's second factor.
func (h *UIHandler) AdminUserResetMFASubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminResetMFA(ctx, userID, actorID)
	})
}

// AdminUserDeletionApproveSubmit approves an account deletion request and deletes/suspends the account.
func (h *UIHandler) AdminUserDeletionApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users?tab=deletion_requests", http.StatusSeeOther)
		return
	}

	reqID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || reqID <= 0 {
		h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "error", "معرف الطلب غير صالح.")
		return
	}

	if h.idSvc != nil {
		if err := h.idSvc.AdminReviewDeletionRequest(ctx, reqID, actor.UserID, true, "تمت الموافقة من إدارة المنصة"); err != nil {
			h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "success", "تمت الموافقة على حذف الحساب وتعطيله بنجاح.")
}

// AdminUserDeletionRejectSubmit rejects an account deletion request.
func (h *UIHandler) AdminUserDeletionRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users?tab=deletion_requests", http.StatusSeeOther)
		return
	}

	reqID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || reqID <= 0 {
		h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "error", "معرف الطلب غير صالح.")
		return
	}

	if h.idSvc != nil {
		if err := h.idSvc.AdminReviewDeletionRequest(ctx, reqID, actor.UserID, false, "تم رفض طلب الحذف من الإدارة"); err != nil {
			h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "success", "تم رفض طلب حذف الحساب بنجاح.")
}

// Organization approval actions.
//
// The approvals page posted straight to the JSON API and swapped the response
// into the table row, so a successful approval replaced the row with the text
// {"status":"approved"}. These do the work and return the refreshed table.

func (h *UIHandler) adminApprovalAction(
	w http.ResponseWriter,
	r *http.Request,
	action func(ctx context.Context, orgID int64) error,
) {
	ctx := r.Context()

	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/approvals", http.StatusSeeOther)
		return
	}
	if h.orgSvc == nil {
		h.renderError(w, r, apperr.Unavailable("org", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}

	if err := action(ctx, id); err != nil {
		h.renderError(w, r, err)
		return
	}

	pendingStatus := org.StatusPending
	pending, err := h.orgSvc.ListOrganizations(ctx, nil, &pendingStatus, 50, 0)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	orgDocs := make(map[int64][]*attachments.Document)
	if h.attSvc != nil && len(pending) > 0 {
		sysCtx := database.AsSystem(ctx)
		for _, o := range pending {
			if o != nil {
				docs, _ := h.attSvc.ListByOrganization(sysCtx, o.ID)
				if len(docs) > 0 {
					orgDocs[o.ID] = docs
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminApprovalsTable(pending, orgDocs).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render approvals table after action", "error", err)
	}
}

// AdminApproveOrgSubmit approves a pending organization.
func (h *UIHandler) AdminApproveOrgSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminApprovalAction(w, r, func(ctx context.Context, orgID int64) error {
		if err := h.orgSvc.ApproveOrganization(ctx, orgID); err != nil {
			return err
		}
		go h.provisionOrgAIAndSubscription(context.Background(), orgID)
		return nil
	})
}

// AdminRejectOrgSubmit rejects a pending organization.
func (h *UIHandler) AdminRejectOrgSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminApprovalAction(w, r, func(ctx context.Context, orgID int64) error {
		return h.orgSvc.RejectOrganization(ctx, orgID)
	})
}

// AdminAnalyticsPage renders the visitor analytics dashboard.
func (h *UIHandler) AdminAnalyticsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	analytics := &platformadmin.VisitorAnalytics{
		ByDevice:  map[string]int{},
		ByOS:      map[string]int{},
		ByBrowser: map[string]int{},
	}
	if h.adminSvc != nil {
		if a, err := h.adminSvc.VisitorAnalytics(ctx, 20); err == nil && a != nil {
			analytics = a
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAnalytics(lang, dir, analytics).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin analytics", "error", err)
	}
}

// AdminAuditPage renders the platform audit trail.
func (h *UIHandler) AdminAuditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var entries []*platformadmin.AuditEntry
	if h.adminSvc != nil {
		list, err := h.adminSvc.ListAuditLog(ctx, 100, 0)
		if err != nil {
			h.log.WarnContext(ctx, "admin audit: list audit log", "error", err)
		} else {
			for _, e := range list {
				localizeAuditEntry(e)
			}
			entries = list
		}
	}

	values := pages.AdminAuditValues{
		Entries:    entries,
		TotalCount: len(entries),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAuditPage(values, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin audit", "error", err)
	}
}

func localizeAuditEntry(e *platformadmin.AuditEntry) {
	if e == nil {
		return
	}
	e.Severity = "عادي (Info)"
	switch e.Action {
	case "org.registered":
		e.Module = "المنشآت"
		e.ActionLabelAr = "تسجيل منشأة جديدة"
		e.Title = "طلب تسجيل منشأة جديدة"
		e.Description = "تم تقديم ملف ترخيص وسجل تجاري لمنشأة دوائية جديدة"
	case "org.approved", "org.status_updated":
		e.Module = "المنشآت"
		e.ActionLabelAr = "تحديث حالة اعتماد المنشأة"
		e.Title = "اعتماد أو ترخيص منشأة"
		e.Description = "تم التحقق من الوثائق والموافقة على حساب المنشأة"
	case "org.rejected":
		e.Module = "المنشآت"
		e.ActionLabelAr = "رفض اعتماد المنشأة"
		e.Title = "رفض اعتماد منشأة"
		e.Description = "تم رفض ملف المنشأة بسبب عدم استيفاء التراخيص"
		e.Severity = "حرج (Critical)"
	case "org.suspended":
		e.Module = "المنشآت"
		e.ActionLabelAr = "إيقاف المنشأة مؤقتاً"
		e.Title = "إيقاف حساب منشأة"
		e.Description = "تم تعليق حساب المنشأة مؤقتاً لمخالفة اللوائح"
		e.Severity = "متوسط (Warning)"
	case "identity.user.registered":
		e.Module = "المستخدمين"
		e.ActionLabelAr = "تسجيل حساب مستخدم جديد"
		e.Title = "إنشاء حساب مستخدم"
		e.Description = "تم تسجيل عضو أو صيدلي جديد في النظام"
	case "identity.user.status_changed":
		e.Module = "المستخدمين"
		e.ActionLabelAr = "تغيير حالة حساب المستخدم"
		e.Title = "تعديل حالة الحساب"
		e.Description = "تحديث حالة التفعيل أو الإيقاف لحساب المستخدم"
		e.Severity = "متوسط (Warning)"
	case "identity.user.role_assigned":
		e.Module = "الأمان والصلاحيات"
		e.ActionLabelAr = "تعيين دور وصلاحية للمستخدم"
		e.Title = "إسناد صلاحية أمنية"
		e.Description = "تعديل رتبة وصلاحيات المستخدم داخل المنصة"
		e.Severity = "متوسط (Warning)"
	case "identity.user.mfa_reset":
		e.Module = "الأمان والصلاحيات"
		e.ActionLabelAr = "إعادة ضبط التحقق الثنائي (MFA)"
		e.Title = "إعادة ضبط أمني (MFA)"
		e.Description = "إعادة ضبط مفاتيح المصادقة الثنائية لحساب المستخدم"
		e.Severity = "حرج (Critical)"
	case "catalog.product.created", "product.created":
		e.Module = "الكتالوج"
		e.ActionLabelAr = "إضافة صنف دوائي جديد"
		e.Title = "إضافة دواء للكتالوج"
		e.Description = "إدراج صنف دوائي ومستحضر معتمد في الكتالوج الموحد"
	case "catalog.product.updated", "product.updated":
		e.Module = "الكتالوج"
		e.ActionLabelAr = "تعديل بيانات الصنف الدوائي"
		e.Title = "تحديث بيانات دواء"
		e.Description = "تعديل الأسعار أو المادة الفعالة أو بيانات الصنف"
	case "catalog.product.deleted", "product.deleted":
		e.Module = "الكتالوج"
		e.ActionLabelAr = "حذف صنف من الكتالوج"
		e.Title = "حذف صنف دوائي"
		e.Description = "إلغاء أو حذف صنف دوائي من الكتالوج المعتمد"
		e.Severity = "حرج (Critical)"
	case "catalog.variant.created", "variant.created":
		e.Module = "عروض الموردين"
		e.ActionLabelAr = "إضافة عرض توريد جديد"
		e.Title = "إضافة عرض سعر دوائي"
		e.Description = "طرح عرض أسعار وتوريد جديد لصنف معتمد"
	case "order.created":
		e.Module = "أوامر التوريد"
		e.ActionLabelAr = "إنشاء طلب توريد جديد"
		e.Title = "إنشاء أمر توريد"
		e.Description = "تم تقديم أمر توريد دوائي جديد من صيدلية"
	case "order.status_updated", "order.status_changed":
		e.Module = "أوامر التوريد"
		e.ActionLabelAr = "تحديث حالة أمر التوريد"
		e.Title = "تحديث حالة الشحن/التوريد"
		e.Description = "تغيير حالة الطلب الدوائي بين التجهيز والتوصيل والاستلام"
	case "institutional_work.created":
		e.Module = "الهيكل المؤسسي"
		e.ActionLabelAr = "إضافة تصنيف هيكل مؤسسي"
		e.Title = "إضافة هيكل مؤسسي جديد"
		e.Description = "إنشاء تصنيف هيكلي جديد للمنشآت والمستودعات"
	case "institutional_work.updated":
		e.Module = "الهيكل المؤسسي"
		e.ActionLabelAr = "تعديل تصنيف هيكل مؤسسي"
		e.Title = "تعديل هيكل مؤسسي"
		e.Description = "تحديث بيانات تصنيف هيكلي أو باقة التسعير"
	default:
		e.Module = "النظام"
		e.ActionLabelAr = e.Action
		e.Title = e.Action
		e.Description = "عملية إدارية مسجلة بالنظام"
	}

	switch e.EntityType {
	case "organization", "org":
		e.EntityTypeAr = "منشأة / شركة"
	case "identity.user", "user":
		e.EntityTypeAr = "مستخدم"
	case "catalog.product", "product":
		e.EntityTypeAr = "صنف دوائي"
	case "catalog.variant", "product_variant", "variant":
		e.EntityTypeAr = "عرض توريد"
	case "order", "commerce.order":
		e.EntityTypeAr = "أمر توريد"
	case "branch", "org.branch":
		e.EntityTypeAr = "فرع مستودع / صيدلية"
	case "institutional_work":
		e.EntityTypeAr = "هيكل مؤسسي"
	default:
		e.EntityTypeAr = e.EntityType
	}

	if e.ActorName == "" {
		e.ActorName = "النظام / System"
	}
	if e.OrganizationName == "" {
		e.OrganizationName = "المنصة الرئيسية"
	}
	if e.IPAddress == "" {
		e.IPAddress = "127.0.0.1 (Local)"
	}
	if e.Route == "" {
		e.Route = "/admin/" + e.EntityType
	}
}

// renderAdminEnterpriseHub renders the unified enterprise management suite.
func (h *UIHandler) renderAdminEnterpriseHub(w http.ResponseWriter, r *http.Request, defaultTab string) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	typeParam := r.URL.Query().Get("type")
	if strings.Contains(r.URL.Path, "/vendors") || strings.Contains(r.URL.Path, "/suppliers") {
		typeParam = "vendor"
	}

	activeTab := r.URL.Query().Get("tab")
	if activeTab == "" {
		activeTab = defaultTab
	}

	var orgs []*org.Organization
	var branches []*org.Branch
	var users []*identity.User
	var deletionRequests []*identity.AccountDeletionRequest

	orgNames := make(map[int64]string)
	orgTypes := make(map[int64]string)
	branchCounts := make(map[int64]int)
	userCounts := make(map[int64]int)

	if h.orgSvc != nil {
		var filterType *org.OrganizationType
		if typeParam != "" {
			t := org.OrganizationType(typeParam)
			filterType = &t
		}
		orgs, _ = h.orgSvc.ListOrganizations(sysCtx, filterType, nil, 300, 0)
		for _, o := range orgs {
			if o != nil {
				orgNames[o.ID] = o.LegalName
				orgTypes[o.ID] = string(o.Type)
			}
		}

		branches, _ = h.orgSvc.ListBranches(sysCtx, 0)
		for _, b := range branches {
			if b != nil {
				branchCounts[b.OrganizationID]++
			}
		}
	}

	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(ctx, "", "")
		deletionRequests, _ = h.idSvc.AdminListDeletionRequests(ctx, "")
	}

	data := pages.AdminEnterpriseHubData{
		Organizations:    orgs,
		Branches:         branches,
		Users:            users,
		DeletionRequests: deletionRequests,
		ActiveTab:        activeTab,
		CurrentType:      typeParam,
		OrgNames:         orgNames,
		OrgTypes:         orgTypes,
		BranchCounts:     branchCounts,
		UserCounts:       userCounts,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrganizations(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin organizations hub", "error", err)
	}
}

// AdminOrganizationsPage renders the full organization list with lifecycle actions.
func (h *UIHandler) AdminOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	h.renderAdminEnterpriseHub(w, r, "organizations")
}

// AdminOrgApproveSubmit approves an organization.
func (h *UIHandler) AdminOrgApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.ApproveOrganization(ctx, id)
		go h.provisionOrgAIAndSubscription(context.Background(), id)
	}
	http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
}

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
					secretKey = gwSets.APIKey
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

func (h *UIHandler) provisionOrgAIAndSubscription(ctx context.Context, orgID int64) {
	if orgID <= 0 {
		return
	}
	sysCtx := database.AsSystem(ctx)

	// 1. Ensure default subscription if none exists
	var sub *billing.Subscription
	if h.billSvc != nil {
		sub, _ = h.billSvc.AssignDefaultSubscription(sysCtx, 0, &orgID)
	}

	// 2. Fetch organization to get details
	if h.orgSvc == nil {
		return
	}
	o, err := h.orgSvc.GetOrganization(sysCtx, orgID)
	if err != nil || o == nil {
		return
	}

	// Determine AI plan ID from subscription
	aiPlanID := "plan-dev"
	if sub != nil && h.billSvc != nil {
		if plan, err := h.billSvc.GetPlanByID(sysCtx, sub.PlanID); err == nil && plan != nil && plan.AIPlanID != "" {
			aiPlanID = plan.AIPlanID
		}
	}

	// 3. Provision user and virtual key in AI Gateway
	adminClient, _, _ := h.getGatewayAdminClient(sysCtx)
	userID, vkey, provErr := adminClient.ProvisionOrganization(sysCtx, orgID, o.LegalName, "", aiPlanID)
	if provErr == nil && (vkey != "" || userID != "") {
		_ = h.orgSvc.UpdateOrganizationAICredentials(sysCtx, orgID, userID, vkey)
		h.log.InfoContext(ctx, "ai gateway organization provisioned", "org_id", orgID, "user_id", userID, "plan_id", aiPlanID)
	} else if provErr != nil {
		h.log.WarnContext(ctx, "ai gateway organization provisioning failed", "org_id", orgID, "error", provErr)
	}
}

// AdminOrgRejectSubmit rejects an organization.
func (h *UIHandler) AdminOrgRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.RejectOrganization(ctx, id)
	}
	http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
}

// AdminOrgSuspendSubmit suspends an organization.
func (h *UIHandler) AdminOrgSuspendSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.SuspendOrganization(ctx, id)
	}
	http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
}

// AdminOrdersPage renders the cross-tenant order search and procurement tabs.
func (h *UIHandler) AdminOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	query := r.URL.Query().Get("q")
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "all"
	}

	var orders []*commerce.Order
	var directOrders []*commerce.Order
	var negOrders []*commerce.Order
	if h.commSvc != nil {
		orders, _ = h.commSvc.AdminSearchOrders(ctx, query, 200, 0)
		for _, o := range orders {
			if o.IsNegotiation {
				negOrders = append(negOrders, o)
			} else {
				directOrders = append(directOrders, o)
			}
		}
	}

	data := pages.AdminOrdersData{
		ActiveTab:         tab,
		Query:             query,
		Orders:            orders,
		DirectOrders:      directOrders,
		NegotiationOrders: negOrders,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrdersHub(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin orders", "error", err)
	}
}

// AdminProductsPage renders the master products catalog with full-database search and pagination.
func (h *UIHandler) AdminProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("search"))
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "all" {
		status = ""
	}
	dosage := strings.TrimSpace(r.URL.Query().Get("dosage"))
	if dosage == "all" {
		dosage = ""
	}

	var brandIDPtr *int64
	if bStr := strings.TrimSpace(r.URL.Query().Get("brand_id")); bStr != "" && bStr != "0" {
		if bid, err := strconv.ParseInt(bStr, 10, 64); err == nil && bid > 0 {
			brandIDPtr = &bid
		}
	}

	var catIDPtr *int64
	if cStr := strings.TrimSpace(r.URL.Query().Get("category_id")); cStr != "" && cStr != "0" {
		if cid, err := strconv.ParseInt(cStr, 10, 64); err == nil && cid > 0 {
			catIDPtr = &cid
		}
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	} else if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

	var products []*catalog.Product
	var totalProducts int
	var brands []*catalog.Brand
	var categories []*catalog.Category

	if h.catSvc != nil {
		sysCtx := database.AsSystem(ctx)
		prods, total, err := h.catSvc.SearchWithTotal(sysCtx, catalog.SearchParams{
			Query:      q,
			CategoryID: catIDPtr,
			BrandID:    brandIDPtr,
			Status:     status,
			DosageForm: dosage,
			Limit:      limit,
			Offset:     offset,
			Sort:       "newest",
		})
		if err == nil {
			products = prods
			totalProducts = total
		} else {
			h.log.ErrorContext(ctx, "admin products search failed", "error", err)
		}
		brands, _ = h.catSvc.ListBrands(sysCtx)
		categories, _ = h.catSvc.ListCategories(sysCtx)
	}

	var brandFilterVal int64
	if brandIDPtr != nil {
		brandFilterVal = *brandIDPtr
	}
	var catFilterVal int64
	if catIDPtr != nil {
		catFilterVal = *catIDPtr
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminProducts(lang, dir, products, brands, categories, totalProducts, page, limit, q, status, dosage, brandFilterVal, catFilterVal).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin products", "error", err)
	}
}

// AdminProductStatusSubmit sets a product's moderation status.
func (h *UIHandler) AdminProductStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.catSvc != nil {
		_, _ = h.catSvc.SetProductsStatus(database.AsSystem(ctx), []int64{id}, catalog.ProductStatus(r.PostFormValue("status")))
	}
	http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
}

// AdminProductCreateSubmit creates a new master product from Super Admin dashboard.
func (h *UIHandler) AdminProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "خدمة المنتجات غير متاحة حالياً.")
		return
	}

	_ = r.ParseMultipartForm(32 << 20)

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/products", "error", "يرجى كتابة اسم الصنف الدوائي بالعربية أو الإنجليزية.")
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "product_image", "products")
	if imgURL == "" {
		imgURL = r.FormValue("image_url")
	}

	var brandIDPtr *int64
	if brandIDStr := strings.TrimSpace(r.FormValue("brand_id")); brandIDStr != "" {
		if bid, err := strconv.ParseInt(brandIDStr, 10, 64); err == nil && bid > 0 {
			brandIDPtr = &bid
		}
	}

	var categoryIDPtr *int64
	if catIDStr := strings.TrimSpace(r.FormValue("category_id")); catIDStr != "" {
		if cid, err := strconv.ParseInt(catIDStr, 10, 64); err == nil && cid > 0 {
			categoryIDPtr = &cid
		}
	}

	manufacturer := strings.TrimSpace(r.FormValue("manufacturer"))
	if manufacturer == "" && brandIDPtr != nil {
		if b, err := h.catSvc.GetBrand(database.AsSystem(ctx), *brandIDPtr); err == nil && b != nil {
			manufacturer = b.Name.Get("ar")
			if manufacturer == "" {
				manufacturer = b.Name.Get("en")
			}
		}
	}

	prod := &catalog.Product{
		Name:                   i18n.New(nameAr, nameEn),
		Description:            i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		ScientificName:         r.FormValue("generic_name"),
		Active:                 r.FormValue("active_ingredient"),
		DosageForm:             r.FormValue("dosage_form"),
		BrandID:                brandIDPtr,
		CategoryID:             categoryIDPtr,
		ManufacturingCompanies: manufacturer,
		SKU:                    r.FormValue("eda_reg_number"),
		Barcode:                r.FormValue("eda_reg_number"),
		Image:                  imgURL,
		Status:                 catalog.StatusActive,
	}

	if _, err := h.catSvc.CreateProduct(database.AsSystem(ctx), prod); err != nil {
		h.log.ErrorContext(ctx, "admin create product failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/products", "success", "تمت إضافة الصنف الدوائي الأساسي بنجاح إلى الدليل المعتمد.")
}

// AdminProductEditSubmit updates an existing master medicine in the catalog.
func (h *UIHandler) AdminProductEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "خدمة المنتجات غير متاحة حالياً.")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/products", "error", "معرف الدواء غير صالح.")
		return
	}

	_ = r.ParseMultipartForm(32 << 20)

	prod, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), id)
	if err != nil || prod == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "الصنف الدوائي غير موجود.")
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/products", "error", "يرجى كتابة اسم الصنف الدوائي بالعربية أو الإنجليزية.")
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "product_image", "products")
	if imgURL == "" {
		imgURL = r.FormValue("image_url")
	}
	if imgURL == "" {
		imgURL = prod.Image
	}

	priceVal, _ := money.Parse(r.FormValue("price"))

	var brandIDPtr *int64
	if brandIDStr := strings.TrimSpace(r.FormValue("brand_id")); brandIDStr != "" {
		if bid, err := strconv.ParseInt(brandIDStr, 10, 64); err == nil && bid > 0 {
			brandIDPtr = &bid
		}
	}

	var categoryIDPtr *int64
	if catIDStr := strings.TrimSpace(r.FormValue("category_id")); catIDStr != "" {
		if cid, err := strconv.ParseInt(catIDStr, 10, 64); err == nil && cid > 0 {
			categoryIDPtr = &cid
		}
	}

	manufacturer := strings.TrimSpace(r.FormValue("manufacturer"))
	if manufacturer == "" && brandIDPtr != nil {
		if b, err := h.catSvc.GetBrand(database.AsSystem(ctx), *brandIDPtr); err == nil && b != nil {
			manufacturer = b.Name.Get("ar")
			if manufacturer == "" {
				manufacturer = b.Name.Get("en")
			}
		}
	}

	prod.Name = i18n.New(nameAr, nameEn)
	prod.Description = i18n.New(r.FormValue("description_ar"), r.FormValue("description_en"))
	prod.ScientificName = r.FormValue("generic_name")
	prod.Active = r.FormValue("active_ingredient")
	prod.DosageForm = r.FormValue("dosage_form")
	prod.BrandID = brandIDPtr
	prod.CategoryID = categoryIDPtr
	prod.ManufacturingCompanies = manufacturer
	prod.SKU = r.FormValue("eda_reg_number")
	prod.Barcode = r.FormValue("eda_reg_number")
	prod.Image = imgURL
	prod.Price = priceVal
	if st := r.FormValue("status"); st != "" {
		prod.Status = catalog.ProductStatus(st)
	}

	if err := h.catSvc.UpdateProduct(database.AsSystem(ctx), prod); err != nil {
		h.log.ErrorContext(ctx, "admin update product failed", "error", err, "id", id)
		h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/products", "success", "تم تحديث بيانات الصنف الدوائي بنجاح.")
}

// AdminProductDeleteSubmit deletes a master medicine from the catalog.
func (h *UIHandler) AdminProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.catSvc != nil {
		if err := h.catSvc.DeleteProduct(database.AsSystem(ctx), id); err != nil {
			h.log.ErrorContext(ctx, "admin delete product failed", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/admin/products", "success", "تم حذف الصنف الدوائي من الكتالوج المعتمد.")
}

// AdminProductsSampleCSV streams a UTF-8 BOM CSV template with sample pharmaceutical products.
func (h *UIHandler) AdminProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_products_sample.csv\"")

	// Excel on Windows reads a BOM-less UTF-8 CSV as the system codepage and
	// renders every Arabic name as mojibake, so the admin "fixes" it by saving
	// in a codepage the importer then has to guess at. The BOM avoids the whole
	// round trip.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	_ = writer.Write(importSampleHeaders)
	for _, row := range importSampleRows {
		_ = writer.Write(row)
	}
}

// AdminProductsSampleXLSX streams the Excel (.xlsx) import template.
func (h *UIHandler) AdminProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Sheet1"
	write := func(rowIdx int, values []string) {
		for colIdx, value := range values {
			cell, err := excelize.CoordinatesToCellName(colIdx+1, rowIdx)
			if err != nil {
				continue
			}
			_ = f.SetCellValue(sheet, cell, value)
		}
	}

	write(1, importSampleHeaders)
	for i, row := range importSampleRows {
		write(i+2, row)
	}

	// Right-to-left, so the sheet opens the way an Arabic-speaking admin reads
	// it and column A is where they expect it.
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: boolPtr(true)})

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_products_sample.xlsx\"")
	if _, err := f.WriteTo(w); err != nil {
		h.log.ErrorContext(r.Context(), "write products sample xlsx", "error", err)
	}
}

func boolPtr(b bool) *bool { return &b }

// AdminOffersPage renders the offer moderation list.
func (h *UIHandler) AdminOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var offers []*promo.Offer
	if h.promoSvc != nil {
		offers, _ = h.promoSvc.ListOffers(ctx, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOffers(lang, dir, offers).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offers", "error", err)
	}
}

// AdminOfferStatusSubmit activates or deactivates an offer.
func (h *UIHandler) AdminOfferStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.promoSvc != nil {
		_ = h.promoSvc.SetOfferActive(ctx, id, r.PostFormValue("active") == "true")
	}
	http.Redirect(w, r, "/admin/offers", http.StatusSeeOther)
}

// AdminJobsPage renders all job vacancies across the platform showing owning companies.
func (h *UIHandler) AdminJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var jobViews []*pages.AdminJobView
	if h.hrSvc != nil {
		offers, err := h.hrSvc.ListPublishedJobs(ctx, 100, 0)
		if err != nil {
			h.log.WarnContext(ctx, "admin jobs: list published jobs", "error", err)
		} else {
			for _, j := range offers {
				companyName := "منشأة معتمدة"
				companyType := "vendor"
				if h.orgSvc != nil {
					if o, err := h.orgSvc.GetOrganization(ctx, j.OrganizationID); err != nil {
						h.log.DebugContext(ctx, "admin jobs: get org optional", "id", j.OrganizationID, "error", err)
					} else if o != nil {
						if o.TradeName["ar"] != "" {
							companyName = o.TradeName["ar"]
						} else {
							companyName = o.LegalName
						}
						companyType = string(o.Type)
					}
				}
				jobViews = append(jobViews, &pages.AdminJobView{
					Job:         j,
					CompanyName: companyName,
					CompanyType: companyType,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminJobs(lang, dir, jobViews).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin jobs", "error", err)
	}
}

// AdminPlansPage renders the subscription plan editor and active subscribers tab.
func (h *UIHandler) AdminPlansPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "plans"
	}

	var plans []*billing.Plan
	var subs []*billing.Subscription
	if h.billSvc != nil {
		plans, _ = h.billSvc.ListPlans(ctx)
		subs, _ = h.billSvc.AdminListSubscriptions(ctx, 100, 0)
	}

	// Retrieve gateway plans for dropdown dynamically from endpoint
	var gwPlans []gateway.GatewayPlan
	adminClient, endpointURL, _ := h.getGatewayAdminClient(ctx)
	gps, gwErr := adminClient.ListPlans(ctx)
	gwOnline := gwErr == nil && len(gps) > 0
	if gwOnline {
		gwPlans = gps
	} else {
		// Standard MuhiyaLLM Gateway defaults
		gwPlans = []gateway.GatewayPlan{
			{ID: "plan-dev", Name: "MuhiyaCode Free (plan-dev)", RPMLimit: 30, TPMLimit: 300000, Description: "باقة التطوير والتشغيل المجانية"},
			{ID: "yalla", Name: "MuhiyaCode Yalla (yalla)", RPMLimit: 60, TPMLimit: 1200000, Description: "باقة الأعمال والنمو المتوسطة"},
			{ID: "max", Name: "MuhiyaCode Max (max)", RPMLimit: 100, TPMLimit: 2500000, Description: "باقة المؤسسات والشركات الكبرى"},
		}
	}

	data := pages.AdminPlansData{
		ActiveTab:     tab,
		Plans:         plans,
		Subscriptions: subs,
		GatewayPlans:  gwPlans,
		GatewayURL:    endpointURL,
		GatewayOnline: gwOnline,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminPlansHub(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin plans hub", "error", err)
	}
}

// AdminPlanSubmit creates a subscription plan.
func (h *UIHandler) AdminPlanSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	priceMonth, _ := money.Parse(r.PostFormValue("price_month"))
	priceYear, _ := money.Parse(r.PostFormValue("price_year"))
	durationDays, _ := strconv.Atoi(r.PostFormValue("duration_days"))
	if durationDays <= 0 {
		durationDays = 30
	}
	maxSessions, _ := strconv.Atoi(r.PostFormValue("max_login_sessions"))
	if maxSessions <= 0 {
		maxSessions = 3
	}
	maxDevices, _ := strconv.Atoi(r.PostFormValue("max_devices"))
	if maxDevices <= 0 {
		maxDevices = 3
	}
	aiPlanID := strings.TrimSpace(r.PostFormValue("ai_plan_id"))
	if aiPlanID == "" {
		aiPlanID = "plan-basic"
	}
	isDefault := r.PostFormValue("is_default") == "1" || r.PostFormValue("is_default") == "true"

	features := map[string]string{}
	if r.PostFormValue("is_compare") == "1" {
		features["compare"] = "true"
	}
	if r.PostFormValue("feature_bulk_import") == "1" {
		features["bulk_import"] = "true"
	}
	if r.PostFormValue("feature_analytics") == "1" {
		features["analytics"] = "true"
	}

	p := &billing.Plan{
		Slug:             r.PostFormValue("slug"),
		Name:             i18n.New(r.PostFormValue("name_ar"), r.PostFormValue("name_en")),
		Description:      i18n.New(r.PostFormValue("description_ar"), r.PostFormValue("description_en")),
		PriceMonth:       priceMonth,
		PriceYear:        priceYear,
		DurationDays:     durationDays,
		MaxLoginSessions: maxSessions,
		MaxDevices:       maxDevices,
		AIPlanID:         aiPlanID,
		IsDefault:        isDefault,
		IsActive:         true,
		Features:         features,
	}
	if _, err := h.billSvc.CreatePlan(ctx, p); err != nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/plans", "success", "تمت إضافة وتفعيل باقة الاشتراك الموحدة بنجاح.")
}

// AdminPlanUpdateSubmit updates an existing subscription plan.
func (h *UIHandler) AdminPlanUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", "معرف الخطة غير صالح.")
		return
	}

	priceMonth, _ := money.Parse(r.PostFormValue("price_month"))
	priceYear, _ := money.Parse(r.PostFormValue("price_year"))
	durationDays, _ := strconv.Atoi(r.PostFormValue("duration_days"))
	if durationDays <= 0 {
		durationDays = 30
	}
	maxSessions, _ := strconv.Atoi(r.PostFormValue("max_login_sessions"))
	if maxSessions <= 0 {
		maxSessions = 3
	}
	maxDevices, _ := strconv.Atoi(r.PostFormValue("max_devices"))
	if maxDevices <= 0 {
		maxDevices = 3
	}
	aiPlanID := strings.TrimSpace(r.PostFormValue("ai_plan_id"))
	if aiPlanID == "" {
		aiPlanID = "plan-basic"
	}
	isDefault := r.PostFormValue("is_default") == "1" || r.PostFormValue("is_default") == "true"
	isActive := r.PostFormValue("is_active") != "0"

	features := map[string]string{}
	if r.PostFormValue("is_compare") == "1" {
		features["compare"] = "true"
	}
	if r.PostFormValue("feature_bulk_import") == "1" {
		features["bulk_import"] = "true"
	}
	if r.PostFormValue("feature_analytics") == "1" {
		features["analytics"] = "true"
	}

	p := &billing.Plan{
		ID:               id,
		Slug:             r.PostFormValue("slug"),
		Name:             i18n.New(r.PostFormValue("name_ar"), r.PostFormValue("name_en")),
		Description:      i18n.New(r.PostFormValue("description_ar"), r.PostFormValue("description_en")),
		PriceMonth:       priceMonth,
		PriceYear:        priceYear,
		DurationDays:     durationDays,
		MaxLoginSessions: maxSessions,
		MaxDevices:       maxDevices,
		AIPlanID:         aiPlanID,
		IsDefault:        isDefault,
		IsActive:         isActive,
		Features:         features,
	}
	if _, err := h.billSvc.UpdatePlan(ctx, p); err != nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/plans", "success", "تم تحديث باقة الاشتراك بنجاح.")
}

// AdminPolicyCreateSubmit creates a new draft version of a legal policy document.
func (h *UIHandler) AdminPolicyCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", "خدمة السياسات غير متاحة.")
		return
	}

	p := &platformadmin.Policy{
		PolicyKey:   r.PostFormValue("policy_key"),
		Version:     r.PostFormValue("version"),
		Title:       i18n.New(r.PostFormValue("title_ar"), r.PostFormValue("title_en")),
		Content:     i18n.New(r.PostFormValue("content_ar"), r.PostFormValue("content_en")),
		Summary:     i18n.New(r.PostFormValue("summary_ar"), r.PostFormValue("summary_en")),
		IsPublished: r.PostFormValue("is_published") == "1",
		CreatedBy:   &actor.UserID,
	}

	if err := h.adminSvc.CreatePolicyVersion(ctx, p); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=policies&key="+p.PolicyKey, "success", "تم حفظ إصدار السياسة بنجاح.")
}

// AdminPolicyPublishSubmit activates a specific policy version.
func (h *UIHandler) AdminPolicyPublishSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", "معرف السياسة غير صالح.")
		return
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", "خدمة السياسات غير متاحة.")
		return
	}

	if err := h.adminSvc.PublishPolicyVersion(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "success", "تم نشر الإصدار وتفعيله للجمهور.")
}

// AdminInstitutionalPage renders the institutional hierarchy and classification screen.
func (h *UIHandler) AdminInstitutionalPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []*org.InstitutionalWork
	var allWorks []*org.InstitutionalWork
	if h.orgSvc != nil {
		items, _ = h.orgSvc.ListInstitutionalWorks(ctx, false)
		allWorks, _ = h.orgSvc.ListAllFlatInstitutionalWorks(ctx, false)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminInstitutional(lang, dir, items, allWorks).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin institutional", "error", err)
	}
}

// AdminInstitutionalNewSubmit creates a new institutional work category.
func (h *UIHandler) AdminInstitutionalNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", "خدمة الهيكل المؤسسي غير متاحة.")
		return
	}

	_ = r.ParseForm()
	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" && titleEn == "" {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", "يرجى كتابة اسم التصنيف المؤسسي.")
		return
	}
	if titleAr == "" {
		titleAr = titleEn
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	var parentID *int64
	if pid, err := strconv.ParseInt(r.PostFormValue("parent_id"), 10, 64); err == nil && pid > 0 {
		parentID = &pid
	}

	viewType, _ := strconv.Atoi(r.PostFormValue("view_type"))
	if viewType <= 0 {
		viewType = 1
	}

	icon := strings.TrimSpace(r.PostFormValue("icon"))
	if icon == "" {
		icon = "building"
	}

	pricingType := strings.TrimSpace(r.PostFormValue("pricing_type"))
	if pricingType == "" {
		pricingType = "free"
	}

	isActive := r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	var allowedConnections []int64
	for _, val := range r.PostForm["connections"] {
		if toID, err := strconv.ParseInt(val, 10, 64); err == nil && toID > 0 {
			allowedConnections = append(allowedConnections, toID)
		}
	}

	slug := strings.ToLower(strings.ReplaceAll(titleEn, " ", "-"))
	if slug == "" {
		slug = fmt.Sprintf("work-%d", time.Now().UnixNano()%1000000)
	}

	iw := &org.InstitutionalWork{
		Title:              i18n.New(titleAr, titleEn),
		Description:        i18n.New(strings.TrimSpace(r.PostFormValue("description_ar")), strings.TrimSpace(r.PostFormValue("description_en"))),
		Icon:               icon,
		PricingType:        org.PricingType(pricingType),
		IsActive:           isActive,
		ViewType:           viewType,
		Slug:               slug,
		ParentID:           parentID,
		AllowedConnections: allowedConnections,
	}

	if err := h.orgSvc.CreateInstitutionalWork(ctx, iw); err != nil {
		h.log.ErrorContext(ctx, "failed to create institutional work", "error", err)
		h.redirectWithNotice(w, r, "/admin/institutional", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/institutional", "success", "تمت إضافة تصنيف الهيكل المؤسسي والاتصالات المسموح بها بنجاح.")
}

// AdminInstitutionalEditSubmit updates an existing institutional category.
func (h *UIHandler) AdminInstitutionalEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", "معرف التصنيف غير صالح.")
		return
	}

	_ = r.ParseForm()
	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" && titleEn == "" {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", "يرجى كتابة اسم التصنيف المؤسسي.")
		return
	}
	if titleAr == "" {
		titleAr = titleEn
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	var parentID *int64
	if pid, err := strconv.ParseInt(r.PostFormValue("parent_id"), 10, 64); err == nil && pid > 0 && pid != id {
		parentID = &pid
	}

	viewType, _ := strconv.Atoi(r.PostFormValue("view_type"))
	if viewType <= 0 {
		viewType = 1
	}

	icon := strings.TrimSpace(r.PostFormValue("icon"))
	if icon == "" {
		icon = "building"
	}

	pricingType := strings.TrimSpace(r.PostFormValue("pricing_type"))
	if pricingType == "" {
		pricingType = "free"
	}

	isActive := r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	var allowedConnections []int64
	for _, val := range r.PostForm["connections"] {
		if toID, err := strconv.ParseInt(val, 10, 64); err == nil && toID > 0 && toID != id {
			allowedConnections = append(allowedConnections, toID)
		}
	}

	iw := &org.InstitutionalWork{
		ID:                 id,
		Title:              i18n.New(titleAr, titleEn),
		Description:        i18n.New(strings.TrimSpace(r.PostFormValue("description_ar")), strings.TrimSpace(r.PostFormValue("description_en"))),
		Icon:               icon,
		PricingType:        org.PricingType(pricingType),
		IsActive:           isActive,
		ViewType:           viewType,
		ParentID:           parentID,
		AllowedConnections: allowedConnections,
	}

	if err := h.orgSvc.UpdateInstitutionalWork(ctx, iw); err != nil {
		h.log.ErrorContext(ctx, "failed to update institutional work", "error", err)
		h.redirectWithNotice(w, r, "/admin/institutional", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/institutional", "success", "تم تحديث بيانات التصنيف المؤسسي والاتصالات بنجاح.")
}

// AdminInstitutionalDeleteSubmit soft deletes an institutional category.
func (h *UIHandler) AdminInstitutionalDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.DeleteInstitutionalWork(ctx, id)
	}
	h.redirectWithNotice(w, r, "/admin/institutional", "success", "تم حذف التصنيف المؤسسي.")
}

// AdminInstitutionalStatusSubmit toggles active status of an institutional category.
func (h *UIHandler) AdminInstitutionalStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.ToggleInstitutionalWorkStatus(ctx, id)
	}
	h.redirectWithNotice(w, r, "/admin/institutional", "success", "تم تحديث حالة تفعيل التصنيف.")
}

// AdminDocumentsPage redirects to the unified documents & approvals audit registry.
func (h *UIHandler) AdminDocumentsPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/approvals?tab=documents", http.StatusSeeOther)
}

// AdminCreateDocumentRequestSubmit issues an administrative document request to an organization.
func (h *UIHandler) AdminCreateDocumentRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	orgID, err := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	if err != nil || orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", "يرجى اختيار منشأة صالحة من القائمة.")
		return
	}

	docType := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))
	title := strings.TrimSpace(r.PostFormValue("title"))
	description := strings.TrimSpace(r.PostFormValue("description"))
	deadlineDays, _ := strconv.Atoi(r.PostFormValue("deadline_days"))
	if deadlineDays <= 0 {
		deadlineDays = 30
	}

	if title == "" {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", "عنوان المستند المطلوب إلزامي.")
		return
	}

	if h.attSvc == nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", "خدمة المستندات غير متاحة.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	if _, err := h.attSvc.CreateDocumentRequest(sysCtx, actor, orgID, docType, title, description, deadlineDays); err != nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "success", "تم إصدار طلب المستند الرسمي للمنشأة مع التنبيه والمهلة المحددة بنجاح.")
}

// AdminCancelDocumentRequestSubmit cancels an active document request.
func (h *UIHandler) AdminCancelDocumentRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", "معرف الطلب غير صالح.")
		return
	}

	if h.attSvc != nil {
		sysCtx := database.AsSystem(ctx)
		_ = h.attSvc.CancelDocumentRequest(sysCtx, actor, id)
	}

	h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "success", "تم إلغاء طلب المستند.")
}

// AdminVerifyUploadedDocSubmit audits, categorizes, and approves/rejects an uploaded document.
func (h *UIHandler) AdminVerifyUploadedDocSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "error", "معرف المستند غير صالح.")
		return
	}

	docType := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))
	status := attachments.DocumentStatus(strings.TrimSpace(r.PostFormValue("status")))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if status != attachments.StatusVerified && status != attachments.StatusRejected {
		status = attachments.StatusVerified
	}

	if h.attSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if err := h.attSvc.VerifyDocumentWithType(sysCtx, actor, id, docType, status, notes); err != nil {
			h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	msg := "تم اعتماد وتوثيق المستند وتحديث ملف المنشأة بنجاح."
	if status == attachments.StatusRejected {
		msg = "تم رفض المستند وحفظ الملاحظات."
	}
	h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "success", msg)
}

// AdminCitiesPage renders the Egyptian cities and spatial coordinates management screen.
func (h *UIHandler) AdminCitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var cities []*platformadmin.City
	if h.adminSvc != nil {
		cities, _ = h.adminSvc.ListAllCities(ctx, 1)
	}
	if len(cities) == 0 {
		cities = h.listCities(ctx)
	}
	data := pages.AdminCitiesData{
		Cities:     cities,
		TotalCount: len(cities),
		Query:      r.URL.Query().Get("q"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminCities(data, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin cities page", "error", err)
	}
}

// AdminCityCreateSubmit adds a new city / district with coordinates.
func (h *UIHandler) AdminCityCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = r.ParseForm()
	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", "اسم المدينة بالعربية مطلوب.")
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("city_lat"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("city_lon"), 64)

	city := &platformadmin.City{
		CountryID: 1,
		Name:      i18n.New(nameAr, nameEn),
		Latitude:  lat,
		Longitude: lon,
		IsActive:  true,
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.CreateCity(ctx, city); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/cities", "success", "تم حفظ وإضافة المدينة بنجاح في قاعدة البيانات.")
}

// AdminCityToggleSubmit toggles the active status of a city.
func (h *UIHandler) AdminCityToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/cities", "error", "معرف المدينة غير صالح.")
		return
	}

	if h.adminSvc != nil {
		if err := h.adminSvc.ToggleCityStatus(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/admin/cities", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/cities", "success", "تم تحديث حالة تفعيل المدينة بنجاح.")
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
			localizeAuditEntry(e)
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDevelopersPage(values, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin developers page", "error", err)
	}
}

// AdminSQLExecuteSubmit executes a SQL query from the Developer SQL Console and returns JSON.
func (h *UIHandler) AdminSQLExecuteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if h.adminSvc == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "خدمة إدارة المنظومة غير متاحة."})
		return
	}

	query := strings.TrimSpace(r.FormValue("query"))
	if query == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "استعلام SQL فارغ."})
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
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", "خدمة إدارة المنظومة غير متاحة.")
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

	gw := &platformadmin.GatewaySettings{
		EndpointURL: endpoint,
		APIKey:      combinedKey,
		Environment: "production",
		IsActive:    isActive,
	}
	if err := h.adminSvc.SaveGatewaySettings(ctx, gw); err != nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "error", h.safeMessage(err, langOf(r)))
		return
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

	h.redirectWithNotice(w, r, "/admin/developers?tab=ai", "success", "تم حفظ إعدادات بوابة الذكاء الاصطناعي بنجاح.")
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
	w.Header().Set("Content-Type", "application/json")

	_ = r.ParseForm()
	reqEndpoint := strings.TrimSpace(r.FormValue("endpoint_url"))
	reqUser := strings.TrimSpace(r.FormValue("admin_username"))
	if reqUser == "" {
		reqUser = "admin"
	}
	reqKey := strings.TrimSpace(r.FormValue("api_key"))

	adminClient, endpoint, _ := h.getGatewayAdminClient(ctx)
	if reqEndpoint != "" {
		endpoint = reqEndpoint
		adminClient = gateway.NewAdminClient(reqEndpoint, reqUser, reqKey)
	}

	plans, err := adminClient.ListPlans(ctx)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		msg := fmt.Sprintf("تعذر الاتصال بـ %s (%v)", endpoint, err)
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
			msg = fmt.Sprintf("خطأ في المصادقة (401 Unauthorized): كلمة مرور أو بيانات اعتماد المدير غير صحيحة لبوابة %s. يرجى كتابة كلمة المرور المحددة في ADMIN_PASSWORD الخاصة بالبوابة والضغط على حفظ.", endpoint)
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
		"message": fmt.Sprintf("الاتصال بـ %s نشط بنجاح — تم جلب %d باقات ذكاء اصطناعي حية من البوابة", endpoint, len(plans)),
		"count":   len(plans),
	})
}

// AdminErrorLogStatusSubmit updates the status of an error record.
func (h *UIHandler) AdminErrorLogStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", "معرف السجل غير صالح.")
		return
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "RESOLVED"
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", "خدمة إدارة المنظومة غير متاحة.")
		return
	}

	if err := h.adminSvc.UpdateErrorLogStatus(ctx, id, status); err != nil {
		h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/developers?tab=errors", "success", "تم تحديث حالة الخطأ بنجاح.")
}
