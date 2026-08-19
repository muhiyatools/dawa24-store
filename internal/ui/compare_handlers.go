package ui

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// ComparePlansPage renders the pricing page for the discount-comparison plans.
func (h *UIHandler) ComparePlansPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "compare.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	var viewPlans []*billing.Plan
	if h.compareSvc != nil {
		cPlans, err := h.compareSvc.ListPlans(ctx, true)
		if err == nil {
			for _, cp := range cPlans {
				viewPlans = append(viewPlans, &billing.Plan{
					ID:          cp.ID,
					Name:        cp.Name,
					Slug:        cp.Slug,
					Description: cp.Description,
					PriceMonth:  cp.PriceMonthly,
					PriceYear:   cp.PriceYearly,
					IsActive:    cp.IsActive,
				})
			}
		}
	} else if h.billSvc != nil {
		viewPlans, _ = h.billSvc.ListPlans(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ComparePlansPage(lang, dir, viewPlans).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare plans", "error", err)
	}
}

// CompareSubscribeSubmit subscribes the caller to a compare plan.
func (h *UIHandler) CompareSubscribeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare", http.StatusSeeOther)
		return
	}

	slug := r.URL.Query().Get("plan")
	if slug == "" {
		h.redirectWithNotice(w, r, "/compare", "error", "تعذر الاشتراك.")
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.compareSvc != nil {
		if _, err := h.compareSvc.SubscribeDirectly(ctx, slug, orgPtr, actor.UserID, "monthly"); err != nil {
			h.redirectWithNotice(w, r, "/compare", "error", h.safeMessage(err, langOf(r)))
			return
		}
	} else if h.billSvc != nil {
		if _, err := h.billSvc.Subscribe(ctx, actor.UserID, orgPtr, slug, "compare", nil); err != nil {
			h.redirectWithNotice(w, r, "/compare", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم تفعيل اشتراكك بنجاح في محرك المقارنة.")
}

// CompareToolPage renders the comparison tool or an upsell, gated by entitlement.
func (h *UIHandler) CompareToolPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	entitled := false
	if actor, ok := authctx.From(ctx); ok {
		if h.compareSvc != nil {
			ent, err := h.compareSvc.EntitlementFor(ctx, actor.UserID, actor.OrganizationID)
			if err == nil && ent.Active {
				entitled = true
				cookieSessionID := ""
				if c, err := r.Cookie("dawa24_session"); err == nil {
					cookieSessionID = c.Value
				}
				clientInfo := compare.ClientInfo{
					SessionID: cookieSessionID,
					UserAgent: r.UserAgent(),
					IPAddress: r.RemoteAddr,
				}
				_ = h.compareSvc.EnforceSessionCap(ctx, actor.UserID, nil, ent.MaxSessions, clientInfo)
			}
		} else if h.billSvc != nil {
			if has, _, err := h.billSvc.CheckEntitlement(ctx, actor.UserID, "compare"); err == nil {
				entitled = has
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareToolPage(lang, dir, entitled).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare tool", "error", err)
	}
}

// CompareUploadSubmit handles uploading a supplier spreadsheet file.
func (h *UIHandler) CompareUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	if h.compareSvc == nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "خدمة المقارنة غير متاحة حالياً.")
		return
	}

	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "تعذر قراءة الملف المرفوع.")
		return
	}

	file, header, err := r.FormFile("compare_file")
	if err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "يرجى اختيار ملف Excel أو CSV.")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "صيغة الملف غير مدعومة. يرجى رفع ملف بصيغة xlsx أو xls أو csv.")
		return
	}

	supplierName := strings.TrimSpace(r.FormValue("supplier_name"))
	if supplierName == "" {
		supplierName = strings.TrimSuffix(header.Filename, ext)
	}

	storageKey, err := saveUploadedFile(r, "compare_file", "compare")
	if err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "فشل حفظ الملف: "+err.Error())
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	_, archived, err := h.compareSvc.UploadCompareFile(
		ctx, actor.UserID, orgPtr, supplierName, header.Filename,
		header.Header.Get("Content-Type"), header.Size, storageKey,
	)
	if err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
		return
	}

	msg := "تم رفع ملف المورد بنجاح."
	if len(archived) > 0 {
		msg += " تنبيه: لقد تجاوزت الحد الأقصى للملفات النشطة، تم نقل المورد الأقدم (" + strings.Join(archived, "، ") + ") إلى الأرشيف."
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", msg)
}

// CompareFileRenameSubmit handles renaming a supplier file label.
func (h *UIHandler) CompareFileRenameSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}
	newName := strings.TrimSpace(r.FormValue("supplier_name"))
	if newName == "" {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "اسم المورد لا يمكن أن يكون فارغاً.")
		return
	}
	if h.compareSvc != nil {
		if err := h.compareSvc.RenameFile(ctx, id, newName); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم تغيير اسم المورد بنجاح.")
}

// CompareFileArchiveSubmit handles manually archiving a file.
func (h *UIHandler) CompareFileArchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}
	if h.compareSvc != nil {
		if err := h.compareSvc.ArchiveFile(ctx, id, "أرشفة يدوية من قبل المستخدم"); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم نقل الملف إلى الأرشيف.")
}

// CompareFileUnarchiveSubmit handles restoring an archived file.
func (h *UIHandler) CompareFileUnarchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}
	if h.compareSvc != nil {
		if err := h.compareSvc.UnarchiveFile(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم استعادة الملف من الأرشيف بنجاح.")
}

// CompareFileDeleteSubmit handles soft-deleting a file.
func (h *UIHandler) CompareFileDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}
	if h.compareSvc != nil {
		if err := h.compareSvc.DeleteFile(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم حذف الملف بنجاح.")
}

// CompareFileMappingSubmit persists user-confirmed column mapping for a spreadsheet.
func (h *UIHandler) CompareFileMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}

	var config compare.MappingConfig
	if nameStr := r.FormValue("name_col"); nameStr != "" {
		if idx, err := strconv.Atoi(nameStr); err == nil && idx >= 0 {
			config.NameCol = &idx
		}
	}
	if priceStr := r.FormValue("price_col"); priceStr != "" {
		if idx, err := strconv.Atoi(priceStr); err == nil && idx >= 0 {
			config.PriceCol = &idx
		}
	}
	if discStr := r.FormValue("discount_col"); discStr != "" {
		if idx, err := strconv.Atoi(discStr); err == nil && idx >= 0 {
			config.DiscountCol = &idx
		}
	}
	if codeStr := r.FormValue("code_col"); codeStr != "" {
		if idx, err := strconv.Atoi(codeStr); err == nil && idx >= 0 {
			config.CodeCol = &idx
		}
	}

	if h.compareSvc != nil {
		if err := h.compareSvc.SaveFileMapping(ctx, id, config); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم حفظ وتطبيق تعيين الأعمدة بنجاح.")
}

// CompareRowManualMatchSubmit allows users to manually link an uploaded row to a master product.
// Persists the match in compare.file_rows and catalog.customer_product_mappings for future auto-match reuse (Plan V5 §2.4.3).
func (h *UIHandler) CompareRowManualMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف سطر غير صالح.")
		return
	}

	productID, err := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if err != nil || productID <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "يرجى اختيار صنف صحيح للربط.")
		return
	}

	rawName := strings.TrimSpace(r.FormValue("raw_name"))

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.compareSvc != nil {
		if err := h.compareSvc.SaveManualCorrection(ctx, orgPtr, rowID, rawName, productID); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم حفظ وتثبيت المطابقة بنجاح.")
}
