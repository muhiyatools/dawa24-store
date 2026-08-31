package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

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
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.invalid_variant_id"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
		return
	}

	existing, err := h.catSvc.GetVariant(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.variant_not_found"))
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
		if c, err := money.Parse(cStr); err == nil && c.IsPositive() {
			existing.CostPrice = &c
		} else {
			existing.CostPrice = nil
		}
	} else {
		existing.CostPrice = nil
	}
	if cdStr := strings.TrimSpace(r.FormValue("cost_discount_percentage")); cdStr != "" {
		if cd, err := strconv.ParseFloat(cdStr, 64); err == nil {
			if cd < 0 {
				cd = 0
			} else if cd > 100 {
				cd = 100
			}
			existing.CostDiscountPercentage = cd
		}
	} else if cdStr := strings.TrimSpace(r.FormValue("cost_discount")); cdStr != "" {
		if cd, err := strconv.ParseFloat(cdStr, 64); err == nil {
			existing.CostDiscountPercentage = cd
		}
	} else {
		existing.CostDiscountPercentage = 0
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
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.update_variant_error_prefix")+h.safeMessage(err, langOf(r)))
		return
	}

	if stockStr := strings.TrimSpace(r.FormValue("stock_qty")); stockStr != "" {
		if stockQty, err := strconv.Atoi(stockStr); err == nil && stockQty >= 0 {
			_ = h.recordInitialStock(ctx, actor.OrganizationID, existing, stockQty)
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", i18n.T(langOf(r), "vendor.catalog.variant_updated_success"))
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
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
		return
	}
	count, err := h.catSvc.DeleteAllVariantsByOrg(ctx, actor.OrganizationID)
	if err != nil {
		h.log.ErrorContext(ctx, "delete all vendor variants error", "error", err)
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.catalog.delete_variants_error_prefix")+h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/products", "success", fmt.Sprintf(i18n.T(langOf(r), "vendor.catalog.deleted_all_success"), count))
}
