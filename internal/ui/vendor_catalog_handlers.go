package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// VendorCatalogSearchJSON returns JSON autocomplete list of general catalog products.
func (h *UIHandler) VendorCatalogSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	type catalogResult struct {
		ID             int64  `json:"id"`
		NameAR         string `json:"name_ar"`
		NameEN         string `json:"name_en"`
		ScientificName string `json:"scientific_name"`
		DosageForm     string `json:"dosage_form"`
		Concentration  string `json:"concentration"`
		Unit           string `json:"unit"`
		SKU            string `json:"sku"`
		Barcode        string `json:"barcode"`
		Price          string `json:"price"`
		Manufacturer   string `json:"manufacturer"`
	}

	var results []catalogResult
	if h.catSvc != nil {
		products, err := h.catSvc.Search(ctx, catalog.SearchParams{
			Query: query,
			Limit: 20,
		})
		if err == nil {
			for _, p := range products {
				results = append(results, catalogResult{
					ID:             p.ID,
					NameAR:         p.Name.Get(i18n.AR),
					NameEN:         p.Name.Get(i18n.EN),
					ScientificName: p.ScientificName,
					DosageForm:     p.DosageForm,
					Concentration:  p.Concentration,
					Unit:           p.Unit,
					SKU:            p.SKU,
					Barcode:        p.Barcode,
					Price:          p.Price.String(),
					Manufacturer:   p.ManufacturingCompanies,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}

// VendorProductDetailJSON returns detailed master product data by ID as JSON.
func (h *UIHandler) VendorProductDetailJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, `{"error":"invalid product id"}`, http.StatusBadRequest)
		return
	}

	if h.catSvc == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	p, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), id)
	if err != nil || p == nil {
		http.Error(w, `{"error":"product not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                      p.ID,
		"name_ar":                 p.Name.Get(i18n.AR),
		"name_en":                 p.Name.Get(i18n.EN),
		"scientific_name":         p.ScientificName,
		"dosage_form":             p.DosageForm,
		"concentration":           p.Concentration,
		"unit":                    p.Unit,
		"sku":                     p.SKU,
		"barcode":                 p.Barcode,
		"price":                   p.Price.String(),
		"manufacturing_companies": p.ManufacturingCompanies,
		"category_id":             p.CategoryID,
	})
}

// VendorProductAddFromCatalogSubmit adds a master product as a vendor variant with customized fields.
func (h *UIHandler) VendorProductAddFromCatalogSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", "بيانات النموذج غير صالحة.")
		return
	}

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if productID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/products", "error", "يرجى اختيار صنف من الكتالوج العام أولاً.")
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		if h.catSvc != nil {
			if mp, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), productID); err == nil && mp != nil {
				nameAr = mp.Name.Get(i18n.AR)
				nameEn = mp.Name.Get(i18n.EN)
			}
		}
	}
	if nameAr == "" && nameEn == "" {
		nameAr = "صنف توريد معتمد"
	}

	batch := strings.TrimSpace(r.FormValue("batch_number"))
	sku := strings.TrimSpace(r.FormValue("sku"))
	barcode := strings.TrimSpace(r.FormValue("barcode"))
	unit := strings.TrimSpace(r.FormValue("unit"))
	if unit == "" {
		unit = "عبوة"
	}

	priceStr := strings.TrimSpace(r.FormValue("price"))
	costStr := strings.TrimSpace(r.FormValue("cost_price"))
	discountStr := strings.TrimSpace(r.FormValue("discount"))
	stockQty, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("stock_qty")))
	minQty, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("min_order_qty")))
	if minQty <= 0 {
		minQty = 1
	}

	branchIDVal, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("branch_id")), 10, 64)
	var branchID *int64
	if branchIDVal > 0 {
		branchID = &branchIDVal
	} else if h.orgSvc != nil {
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil && len(branches) > 0 {
			for _, b := range branches {
				if b.IsMain {
					branchID = &b.ID
					break
				}
			}
			if branchID == nil {
				branchID = &branches[0].ID
			}
		}
	}

	var expiryDate *time.Time
	if expStr := strings.TrimSpace(r.FormValue("expiry_date")); expStr != "" {
		if t, err := time.Parse("2006-01-02", expStr); err == nil {
			expiryDate = &t
		}
	}

	price, _ := money.Parse(priceStr)
	cost, _ := money.Parse(costStr)
	discount, _ := money.Parse(discountStr)
	isNegotiable := r.FormValue("is_negotiable") == "true" || r.FormValue("is_negotiable") == "1"

	variant := &catalog.ProductVariant{
		OrganizationID: actor.OrganizationID,
		ProductID:      productID,
		Name:           i18n.New(nameAr, nameEn),
		BatchNumber:    batch,
		ExpiryDate:     expiryDate,
		Price:          price,
		CostPrice:      cost,
		Discount:       discount,
		StockQty:       stockQty,
		MinOrderQty:    minQty,
		BranchID:       branchID,
		SKU:            sku,
		Barcode:        barcode,
		Unit:           unit,
		IsNegotiable:   isNegotiable,
		Status:         catalog.StatusActive,
	}

	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", "خدمة الكتالوج غير متاحة حالياً.")
		return
	}

	created, err := h.catSvc.CreateVariant(ctx, variant)
	if err != nil {
		h.log.ErrorContext(ctx, "add variant from catalog error", "error", err)
		h.redirectWithNotice(w, r, "/vendor/products", "error", "حدث خطأ أثناء إضافة الصنف: "+h.safeMessage(err, langOf(r)))
		return
	}

	if stockQty > 0 && created != nil {
		_ = h.recordInitialStock(ctx, actor.OrganizationID, created, stockQty)
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", "تمت إضافة الصنف من الكتالوج العام ونشره في قائمة أصناف التوريد بنجاح.")
}

// VendorVariantUpdateSubmit updates an existing variant's prices, batch, expiry, and attributes.
func (h *UIHandler) VendorVariantUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/products", "error", "معرف الصنف غير صالح.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", "بيانات النموذج غير صالحة.")
		return
	}

	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", "خدمة الكتالوج غير متاحة حالياً.")
		return
	}

	existing, err := h.catSvc.GetVariant(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/products", "error", "لم يتم العثور على الصنف المطلوب تعديله.")
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr != "" || nameEn != "" {
		existing.Name = i18n.New(nameAr, nameEn)
	}

	existing.BatchNumber = strings.TrimSpace(r.FormValue("batch_number"))
	existing.SKU = strings.TrimSpace(r.FormValue("sku"))
	existing.Barcode = strings.TrimSpace(r.FormValue("barcode"))
	if u := strings.TrimSpace(r.FormValue("unit")); u != "" {
		existing.Unit = u
	}

	if pStr := strings.TrimSpace(r.FormValue("price")); pStr != "" {
		if p, err := money.Parse(pStr); err == nil {
			existing.Price = p
		}
	}
	if dStr := strings.TrimSpace(r.FormValue("discount")); dStr != "" {
		if d, err := money.Parse(dStr); err == nil {
			existing.Discount = d
		}
	}
	if cStr := strings.TrimSpace(r.FormValue("cost_price")); cStr != "" {
		if c, err := money.Parse(cStr); err == nil {
			existing.CostPrice = c
		}
	}

	if minQ, err := strconv.Atoi(strings.TrimSpace(r.FormValue("min_order_qty"))); err == nil && minQ > 0 {
		existing.MinOrderQty = minQ
	}

	if bStr := strings.TrimSpace(r.FormValue("branch_id")); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			existing.BranchID = &bID
		}
	}
	if existing.BranchID == nil && h.orgSvc != nil {
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil && len(branches) > 0 {
			for _, b := range branches {
				if b.IsMain {
					existing.BranchID = &b.ID
					break
				}
			}
			if existing.BranchID == nil {
				existing.BranchID = &branches[0].ID
			}
		}
	}

	if expStr := strings.TrimSpace(r.FormValue("expiry_date")); expStr != "" {
		if t, err := time.Parse("2006-01-02", expStr); err == nil {
			existing.ExpiryDate = &t
		}
	}

	if st := strings.TrimSpace(r.FormValue("status")); st != "" {
		existing.Status = catalog.ProductStatus(st)
	}

	if negStr := strings.TrimSpace(r.FormValue("is_negotiable")); negStr != "" {
		existing.IsNegotiable = (negStr == "true" || negStr == "1")
	}

	if _, err := h.catSvc.UpdateVariant(ctx, id, existing); err != nil {
		h.log.ErrorContext(ctx, "update variant error", "error", err, "variant_id", id)
		h.redirectWithNotice(w, r, "/vendor/products", "error", "حدث خطأ أثناء تعديل الصنف: "+h.safeMessage(err, langOf(r)))
		return
	}

	if stockStr := strings.TrimSpace(r.FormValue("stock_qty")); stockStr != "" {
		if stockQty, err := strconv.Atoi(stockStr); err == nil && stockQty >= 0 {
			_ = h.recordInitialStock(ctx, actor.OrganizationID, existing, stockQty)
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", "تم تحديث بيانات وسعر صنف التوريد بنجاح.")
}

// VendorCatalogSelectPage permanently redirects legacy route to /vendor/products.
func (h *UIHandler) VendorCatalogSelectPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/products", http.StatusMovedPermanently)
}

// VendorCatalogSelectSubmit redirects legacy form submission to /vendor/products.
func (h *UIHandler) VendorCatalogSelectSubmit(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/products", http.StatusMovedPermanently)
}

// VendorProductsDeleteAllSubmit removes all products/variants of the current vendor.
func (h *UIHandler) VendorProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", "خدمة الكتالوج غير متاحة حالياً.")
		return
	}
	count, err := h.catSvc.DeleteAllVariantsByOrg(ctx, actor.OrganizationID)
	if err != nil {
		h.log.ErrorContext(ctx, "delete all vendor variants error", "error", err)
		h.redirectWithNotice(w, r, "/vendor/products", "error", "حدث خطأ أثناء حذف الأصناف: "+h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/products", "success", fmt.Sprintf("تم حذف %d من أصناف التوريد الخاصة بك بنجاح.", count))
}
