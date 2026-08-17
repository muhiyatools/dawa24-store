package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) VendorProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	products, err := h.catSvc.Search(ctx, catalog.SearchParams{
		OrganizationID: &actor.OrganizationID,
		Limit:          h.pageLimit(r),
		Offset:         h.pageOffset(r),
	})
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorProducts(products, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor products page", "error", err)
	}
}

func (h *UIHandler) VendorProductNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var categories []*catalog.Category
	var brands []*catalog.Brand
	if h.catSvc != nil {
		categories, _ = h.catSvc.ListCategories(ctx)
		brands, _ = h.catSvc.ListBrands(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorProductEditor(nil, categories, brands, true, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render new product page", "error", err)
	}
}

func (h *UIHandler) VendorProductEditorPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.catSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	product, _, err := h.catSvc.GetProduct(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	var categories []*catalog.Category
	var brands []*catalog.Brand
	if h.catSvc != nil {
		categories, _ = h.catSvc.ListCategories(ctx)
		brands, _ = h.catSvc.ListBrands(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorProductEditor(product, categories, brands, false, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render edit product page", "error", err)
	}
}

func (h *UIHandler) VendorInventoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if _, ok := database.TenantFrom(ctx); !ok {
		ctx = database.WithTenant(ctx, 1)
	}

	if h.invSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorInventory(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	stocks, err := h.invSvc.ListLowStock(ctx, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.log.WarnContext(ctx, "list low stock error", "error", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorInventory(stocks, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor inventory page", "error", err)
	}
}

func (h *UIHandler) VendorTransfersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if _, ok := database.TenantFrom(ctx); !ok {
		ctx = database.WithTenant(ctx, 1)
	}

	if h.invSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorTransfers(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	transfers, err := h.invSvc.ListTransfers(ctx, "", h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.log.WarnContext(ctx, "list transfers error", "error", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorTransfers(transfers, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor transfers page", "error", err)
	}
}

func (h *UIHandler) VendorIngestPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	if h.ingSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorIngest(nil, lang, dir).Render(ctx, w)
		return
	}

	sessions, err := h.ingSvc.ListSessions(ctx, actor.OrganizationID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngest(sessions, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor ingest page", "error", err)
	}
}

func (h *UIHandler) VendorOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorOrders(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	shipments, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOrders(shipments, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor orders page", "error", err)
	}
}

func (h *UIHandler) VendorOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.promoSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorOffers(nil, nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	offers, err := h.promoSvc.ListActiveOffers(ctx, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	packages, _ := h.promoSvc.ListPackages(ctx)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOffers(offers, packages, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offers page", "error", err)
	}
}

func (h *UIHandler) VendorProductSaveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	if h.catSvc == nil {
		http.Redirect(w, r, "/vendor/products", http.StatusSeeOther)
		return
	}

	nameAr := r.PostFormValue("name_ar")
	nameEn := r.PostFormValue("name_en")
	dosage := r.PostFormValue("dosage_form")
	manufacturer := r.PostFormValue("manufacturing_companies")
	scientific := r.PostFormValue("scientific_name")
	barcode := r.PostFormValue("barcode")

	prod := &catalog.Product{
		OrganizationID:         actor.OrganizationID,
		Name:                   i18n.New(nameAr, nameEn),
		DosageForm:             dosage,
		ManufacturingCompanies: manufacturer,
		ScientificName:         scientific,
		Barcode:                barcode,
		Status:                 catalog.StatusActive,
	}

	// The error used to be discarded into `_, _`. A product that failed
	// validation or hit a constraint redirected to the list exactly like a
	// successful one, so the vendor watched their work vanish with no
	// explanation and no way to tell the two outcomes apart.
	if _, err := h.catSvc.CreateProduct(ctx, prod); err != nil {
		h.log.ErrorContext(ctx, "create product from vendor form", "error", err,
			"organization_id", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/products/new", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/products", "success", "تم حفظ المنتج بنجاح.")
}

// VendorProductDeleteSubmit removes a product and re-renders the table.
//
// The catalog API has exposed DELETE /api/v1/catalog/products/{id} all along;
// the vendor screen offered no way to reach it, so a product added by mistake
// could only be edited, never removed.
func (h *UIHandler) VendorProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}
	if h.catSvc == nil {
		h.renderError(w, r, apperr.Unavailable("catalog", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}

	// Confirm the product belongs to this vendor before deleting it. The id
	// arrives from the page, and DeleteProduct takes an id alone.
	prod, _, err := h.catSvc.GetProduct(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	if prod.OrganizationID != actor.OrganizationID {
		h.renderError(w, r, apperr.NotFound("product"))
		return
	}

	if err := h.catSvc.DeleteProduct(ctx, id); err != nil {
		h.renderError(w, r, err)
		return
	}

	// Re-render the table so the row disappears without a full page load.
	products, err := h.catSvc.Search(ctx, catalog.SearchParams{
		OrganizationID: &actor.OrganizationID,
		Limit:          h.pageLimit(r),
		Offset:         h.pageOffset(r),
	})
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorProductsTable(products).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render products table after delete", "error", err)
	}
}

func (h *UIHandler) VendorOrderStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	shipmentID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	toStatus := r.PostFormValue("status")

	if h.commSvc != nil && shipmentID > 0 && toStatus != "" {
		_, _ = h.commSvc.TransitionShipmentStatus(ctx, shipmentID, commerce.OrderStatus(toStatus), &actor.UserID, "")
		if carrier := r.PostFormValue("carrier"); carrier != "" {
			_ = h.commSvc.SetShipmentTracking(ctx, shipmentID, carrier, r.PostFormValue("tracking"))
		}
	}

	http.Redirect(w, r, "/vendor/orders", http.StatusSeeOther)
}

// VendorStockAdjustSubmit adjusts a stock level with a reason.
func (h *UIHandler) VendorStockAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}

	stockID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	delta, _ := strconv.Atoi(r.PostFormValue("delta"))
	if h.invSvc != nil && stockID > 0 && delta != 0 {
		_, _ = h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
			StockID: stockID,
			Delta:   delta,
			Type:    inventory.MovementAdjustment,
			Details: r.PostFormValue("reason"),
			UserID:  &actor.UserID,
		})
	}
	http.Redirect(w, r, "/vendor/inventory", http.StatusSeeOther)
}
