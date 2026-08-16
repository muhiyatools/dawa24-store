package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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

	if h.invSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorInventory(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	stocks, err := h.invSvc.ListLowStock(ctx, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorInventory(stocks, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor inventory page", "error", err)
	}
}

func (h *UIHandler) VendorTransfersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.invSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorTransfers(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	transfers, err := h.invSvc.ListTransfers(ctx, "", h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
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
