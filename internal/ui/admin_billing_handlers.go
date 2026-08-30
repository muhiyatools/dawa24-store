package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

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
		plans, _ = h.billSvc.AdminListPlans(ctx)
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

	h.renderPage(ctx, w, "render admin plans hub", pages.AdminPlansHub(data, lang, dir))
}

// AdminPlanSubmit creates a subscription plan.
func (h *UIHandler) AdminPlanSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	slug := strings.TrimSpace(strings.ToLower(r.PostFormValue("slug")))
	if slug == "" {
		if nameEn != "" {
			slug = strings.ReplaceAll(strings.ToLower(nameEn), " ", "-")
		} else {
			slug = fmt.Sprintf("plan-%d", time.Now().Unix())
		}
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
	if r.PostFormValue("feature_market_discounts") == "1" || r.PostFormValue("feature_market_discounts") == "true" {
		features[billing.FeatureMarketDiscounts] = "true"
	} else {
		features[billing.FeatureMarketDiscounts] = "false"
	}
	if r.PostFormValue("feature_compare_tool") == "1" || r.PostFormValue("feature_compare_tool") == "true" || r.PostFormValue("is_compare") == "1" {
		features[billing.FeatureCompareTool] = "true"
		features["compare"] = "true"
	} else {
		features[billing.FeatureCompareTool] = "false"
	}
	if r.PostFormValue("feature_bulk_import") == "1" || r.PostFormValue("feature_bulk_import") == "true" {
		features["bulk_import"] = "true"
	}
	if r.PostFormValue("feature_analytics") == "1" || r.PostFormValue("feature_analytics") == "true" {
		features["analytics"] = "true"
	}
	maxCompareFiles := strings.TrimSpace(r.PostFormValue("max_compare_files"))
	if maxCompareFiles != "" {
		features[billing.FeatureMaxCompareFiles] = maxCompareFiles
	} else {
		features[billing.FeatureMaxCompareFiles] = "10"
	}
	retentionDays := strings.TrimSpace(r.PostFormValue("compare_file_retention_days"))
	if retentionDays != "" {
		features[billing.FeatureCompareRetentionDays] = retentionDays
	} else {
		features[billing.FeatureCompareRetentionDays] = "30"
	}

	p := &billing.Plan{
		Slug:             slug,
		Name:             i18n.New(nameAr, nameEn),
		Description:      i18n.New(strings.TrimSpace(r.PostFormValue("description_ar")), strings.TrimSpace(r.PostFormValue("description_en"))),
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
	if r.PostFormValue("feature_market_discounts") == "1" || r.PostFormValue("feature_market_discounts") == "true" {
		features[billing.FeatureMarketDiscounts] = "true"
	} else {
		features[billing.FeatureMarketDiscounts] = "false"
	}
	if r.PostFormValue("feature_compare_tool") == "1" || r.PostFormValue("feature_compare_tool") == "true" || r.PostFormValue("is_compare") == "1" {
		features[billing.FeatureCompareTool] = "true"
		features["compare"] = "true"
	} else {
		features[billing.FeatureCompareTool] = "false"
	}
	if r.PostFormValue("feature_bulk_import") == "1" {
		features["bulk_import"] = "true"
	}
	if r.PostFormValue("feature_analytics") == "1" {
		features["analytics"] = "true"
	}
	maxCompareFilesUpdate := strings.TrimSpace(r.PostFormValue("max_compare_files"))
	if maxCompareFilesUpdate != "" {
		features[billing.FeatureMaxCompareFiles] = maxCompareFilesUpdate
	} else {
		features[billing.FeatureMaxCompareFiles] = "10"
	}
	retentionDaysUpdate := strings.TrimSpace(r.PostFormValue("compare_file_retention_days"))
	if retentionDaysUpdate != "" {
		features[billing.FeatureCompareRetentionDays] = retentionDaysUpdate
	} else {
		features[billing.FeatureCompareRetentionDays] = "30"
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
	h.redirectWithNotice(w, r, "/admin/plans", "success", "تم حفظ وتحديث بيانات باقة الاشتراك بنجاح.")
}

// AdminPlanToggleSubmit toggles the active/inactive state of a subscription plan.
func (h *UIHandler) AdminPlanToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", "معرف الخطة غير صالح.")
		return
	}
	if err := h.billSvc.TogglePlanActive(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/plans", "success", "تم تغيير حالة تفعيل باقة الاشتراك بنجاح.")
}

// AdminPlanSetDefaultSubmit designates a plan as the system default tier.
func (h *UIHandler) AdminPlanSetDefaultSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", "معرف الخطة غير صالح.")
		return
	}
	if err := h.billSvc.SetDefaultPlan(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/plans", "success", "تم تعيين الباقة المحددة كباقة افتراضية للمنظومة بنجاح.")
}

// AdminPlanDeleteSubmit deletes a subscription plan if it's safe.
func (h *UIHandler) AdminPlanDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", "معرف الخطة غير صالح.")
		return
	}
	if err := h.billSvc.DeletePlan(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/plans", "success", "تم حذف باقة الاشتراك بنجاح.")
}
