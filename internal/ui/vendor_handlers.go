package ui

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorOrganizationPage displays the supplier's commercial profile, order price limits, contact details, description, logo and cover image.
func (h *UIHandler) VendorOrganizationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/organization", http.StatusSeeOther)
		return
	}

	orgID := actor.OrganizationID
	if orgID <= 0 {
		http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice_msg")

	profile, err := h.orgSvc.GetSupplierProfile(ctx, orgID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to get supplier organization profile", "org_id", orgID, "error", err)
		profile = &org.SupplierOrgProfile{
			ID:            orgID,
			NameAr:        actor.Email,
			Type:          "supplier",
			MinOrderPrice: money.FromMajor(10),
			MaxOrderPrice: money.FromMajor(50),
		}
	}

	_ = pages.VendorOrganizationPage(lang, dir, profile, noticeType, noticeMsg).Render(ctx, w)
}

// VendorOrganizationSubmit handles updating supplier organization commercial info, limits, and file uploads.
func (h *UIHandler) VendorOrganizationSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/organization", http.StatusSeeOther)
		return
	}

	orgID := actor.OrganizationID
	if orgID <= 0 {
		http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
		return
	}

	_ = r.ParseMultipartForm(10 << 20) // 10MB

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	orgType := strings.TrimSpace(r.FormValue("type"))
	if orgType == "" {
		orgType = "supplier"
	}

	minPrice, err := money.Parse(strings.TrimSpace(r.FormValue("min_order_price")))
	if err != nil {
		minPrice = money.FromMajor(10)
	}
	maxPrice, err := money.Parse(strings.TrimSpace(r.FormValue("max_order_price")))
	if err != nil {
		maxPrice = money.FromMajor(50)
	}
	if maxPrice.Minor() < minPrice.Minor() {
		http.Redirect(w, r, "/vendor/organization?notice_type=error&notice_msg="+url.QueryEscape("الحد الأقصى لسعر الطلب يجب أن يكون أكبر من أو يساوي الحد الأدنى"), http.StatusSeeOther)
		return
	}

	orgNumber := strings.TrimSpace(r.FormValue("organization_number"))
	email := strings.TrimSpace(r.FormValue("email"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	taxNumber := strings.TrimSpace(r.FormValue("tax_number"))
	address := strings.TrimSpace(r.FormValue("address"))
	descAr := strings.TrimSpace(r.FormValue("description_ar"))
	descEn := strings.TrimSpace(r.FormValue("description_en"))

	var logoURL, coverURL string
	if file, header, err := r.FormFile("logo_file"); err == nil && file != nil {
		defer file.Close()
		data, _ := io.ReadAll(file)
		if len(data) > 0 {
			if u, err := saveUploadedBytes(data, header.Filename, "org"); err == nil {
				logoURL = u
			}
		}
	}

	if file, header, err := r.FormFile("coverage_file"); err == nil && file != nil {
		defer file.Close()
		data, _ := io.ReadAll(file)
		if len(data) > 0 {
			if u, err := saveUploadedBytes(data, header.Filename, "org"); err == nil {
				coverURL = u
			}
		}
	}

	profile := &org.SupplierOrgProfile{
		ID:                 orgID,
		NameAr:             nameAr,
		NameEn:             nameEn,
		Type:               orgType,
		MinOrderPrice:      minPrice,
		MaxOrderPrice:      maxPrice,
		OrganizationNumber: orgNumber,
		Email:              email,
		Phone:              phone,
		TaxNumber:          taxNumber,
		Address:            address,
		DescriptionAr:      descAr,
		DescriptionEn:      descEn,
		Image:              logoURL,
		CoverageImage:      coverURL,
	}

	if err := h.orgSvc.UpdateSupplierProfile(ctx, profile); err != nil {
		h.log.ErrorContext(ctx, "failed to update supplier profile", "org_id", orgID, "error", err)
		http.Redirect(w, r, "/vendor/organization?notice_type=error&notice_msg="+url.QueryEscape("حدث خطأ أثناء حفظ التعديلات: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/vendor/organization?notice_type=success&notice_msg="+url.QueryEscape("تم حفظ وتحديث بيانات المنشأة والهوية التجارية بنجاح"), http.StatusSeeOther)
}
