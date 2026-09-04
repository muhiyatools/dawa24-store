package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// VendorVariantUpdateSubmit applies the edit dialog to one supply variant.
//
// It writes only the fields the form carried. That is the whole change, and it
// was a data-loss bug rather than a nicety: the dialog has no SKU, barcode,
// English-name or unit input, and this handler assigned all four
// unconditionally from the absent form values —
//
//	existing.SKU     = r.FormValue("sku")      // ""
//	existing.Barcode = r.FormValue("barcode")  // ""
//
// — which UpdateVariant then wrote. The partial unique index on (organization_id,
// sku) excludes the empty string, so blanking never raised an error; it just
// erased the code. Of the 2,107 live variants, 915 have no SKU and 2,107 have no
// barcode. Cost price and cost discount were cleared the same way, by an else
// branch that ran whenever the field was absent.
//
// A form that does not mention a column must leave that column alone.
func (h *UIHandler) VendorVariantUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "vendor.catalog.invalid_variant_id"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(lang, "common.catalog_service_unavailable"))
		return
	}

	// The listing's page, search and filters, so saving does not throw the
	// vendor back to the top of a nine-thousand-row catalogue.
	back := vendorProductsBackURL(r)

	existing, err := h.catSvc.GetVariant(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.catalog.variant_not_found"))
		return
	}

	if err := applyVariantEdit(r, existing, lang); err != nil {
		h.redirectWithNotice(w, r, back, "error", err.Error())
		return
	}
	if bID := h.resolveTeamBranch(ctx, actor.OrganizationID, r.PostFormValue("branch_id")); bID != nil {
		existing.BranchID = bID
	}

	if _, err := h.catSvc.UpdateVariant(ctx, id, existing); err != nil {
		h.log.ErrorContext(ctx, "update variant", "error", err, "variant_id", id)
		h.redirectWithNotice(w, r, back, "error",
			i18n.T(lang, "vendor.catalog.update_variant_error_prefix")+h.safeMessage(err, lang))
		return
	}

	if stockStr := strings.TrimSpace(r.FormValue("stock_qty")); stockStr != "" {
		if stockQty, convErr := strconv.Atoi(stockStr); convErr == nil && stockQty >= 0 {
			if stockErr := h.recordInitialStock(ctx, actor.OrganizationID, existing, stockQty); stockErr != nil {
				h.log.ErrorContext(ctx, "record variant stock", "error", stockErr, "variant_id", id)
			}
		}
	}

	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.catalog.variant_updated_success"))
}

// vendorProductsBackURL rebuilds the listing the vendor was looking at.
//
// The listing's status filter travels as status_filter, not status: the form
// already has a "status" field and it is the variant's own status. Two things
// named the same in one submission is how one of them gets read as the other.
func vendorProductsBackURL(r *http.Request) string {
	q := url.Values{}
	for formKey, queryKey := range map[string]string{
		"page":          "page",
		"limit":         "limit",
		"q":             "q",
		"status_filter": "status",
		"stock":         "stock",
		"sort":          "sort",
	} {
		if v := strings.TrimSpace(r.PostFormValue(formKey)); v != "" {
			q.Set(queryKey, v)
		}
	}
	if len(q) == 0 {
		return "/vendor/products"
	}
	return "/vendor/products?" + q.Encode()
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
