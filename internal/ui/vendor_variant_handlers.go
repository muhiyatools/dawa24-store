package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorProductsPage renders one page of the vendor's supply variants.
//
// Everything is paged, searched and filtered at the database. The screen this
// replaces asked for five hundred variants, rendered all of them, and filtered
// them in the browser — so a vendor with nine thousand could not reach the
// other eight and a half thousand, and the counters above the table told them
// they owned five hundred.
func (h *UIHandler) VendorProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	branchOptions, branchMap := h.vendorBranchOptions(ctx, actor.OrganizationID)
	query := vendorVariantQueryFrom(r)

	data := pages.VendorVariantsData{
		Branches:   branchOptions,
		Filter:     query,
		NoticeType: r.URL.Query().Get("notice_type"),
		NoticeMsg:  r.URL.Query().Get("notice"),
	}

	if h.catSvc != nil && actor.OrganizationID > 0 {
		variants, total, err := h.catSvc.ListVendorVariants(ctx, actor.OrganizationID, query)
		if err != nil {
			h.log.ErrorContext(ctx, "list vendor variants", "error", err)
			data.LoadError = h.safeMessage(err, langOf(r))
		} else {
			data.Total = total
			data.Variants = h.decorateVendorVariants(ctx, variants, branchMap)
		}

		stats, statsErr := h.catSvc.VendorVariantStats(ctx, actor.OrganizationID)
		if statsErr != nil {
			h.log.WarnContext(ctx, "vendor variant stats unavailable", "error", statsErr)
		} else {
			data.Stats = stats
		}
	}

	h.renderPage(ctx, w, "render vendor products page", pages.VendorProducts(data, lang, dir, h.isHTMX(r)))
}

// vendorVariantQueryFrom reads the listing controls out of the URL.
//
// Every value is clamped rather than trusted: the page size to the offered
// sizes, the page to at least one, so a hand-edited link cannot ask for a
// hundred thousand rows in one response.
func vendorVariantQueryFrom(r *http.Request) catalog.VendorVariantQuery {
	q := r.URL.Query()
	query := catalog.VendorVariantQuery{
		Query:      strings.TrimSpace(q.Get("q")),
		Status:     q.Get("status"),
		Stock:      catalog.StockFilter(q.Get("stock")),
		Expiring:   q.Get("expiring") == "1",
		Sort:       q.Get("sort"),
		PageNumber: 1,
		PerPage:    catalog.DefaultPageSize,
	}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 1 {
		query.PageNumber = n
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil {
		query.PerPage = n
	}
	switch query.Status {
	case string(catalog.StatusActive), string(catalog.StatusInactive),
		string(catalog.StatusPending), string(catalog.StatusRejected):
	default:
		query.Status = ""
	}
	switch query.Stock {
	case catalog.StockFilterIn, catalog.StockFilterLow, catalog.StockFilterOut:
	default:
		query.Stock = catalog.StockFilterAny
	}
	return query
}

// vendorBranchOptions lists the vendor's branches and a lookup for the table.
func (h *UIHandler) vendorBranchOptions(
	ctx context.Context, orgID int64,
) ([]pages.VendorBranchOption, map[int64]string) {
	names := make(map[int64]string)
	if h.orgSvc == nil || orgID <= 0 {
		return nil, names
	}
	branches, err := h.orgSvc.ListBranches(ctx, orgID)
	if err != nil {
		h.log.WarnContext(ctx, "vendor branches unavailable", "error", err)
		return nil, names
	}
	var options []pages.VendorBranchOption
	for _, b := range branches {
		name := b.Name.Get(i18n.AR)
		if name == "" {
			name = b.Name.Get(i18n.EN)
		}
		options = append(options, pages.VendorBranchOption{ID: b.ID, Name: name, IsMain: b.IsMain})
		names[b.ID] = name
	}
	return options, names
}

// decorateVendorVariants attaches the shared-catalogue product and the branch
// name to each row.
//
// The products are fetched in one query for the whole page. Fetching them per
// row is a hundred round trips for a hundred-row page, which is what the
// previous version did behind a cache that only helped when two variants of the
// same product happened to land on the same screen.
func (h *UIHandler) decorateVendorVariants(
	ctx context.Context, variants []*catalog.ProductVariant, branchMap map[int64]string,
) []*pages.VendorVariantView {
	ids := make([]int64, 0, len(variants))
	seen := make(map[int64]bool, len(variants))
	for _, v := range variants {
		if v.ProductID > 0 && !seen[v.ProductID] {
			seen[v.ProductID] = true
			ids = append(ids, v.ProductID)
		}
	}
	products, err := h.catSvc.ProductsByIDs(ctx, ids)
	if err != nil {
		h.log.WarnContext(ctx, "master products unavailable for vendor listing", "error", err)
		products = map[int64]*catalog.Product{}
	}

	out := make([]*pages.VendorVariantView, 0, len(variants))
	for _, v := range variants {
		branch := i18n.T("ar", "vendor.ingest.main_warehouse")
		if v.BranchID != nil {
			if name, ok := branchMap[*v.BranchID]; ok {
				branch = name
			}
		}
		out = append(out, &pages.VendorVariantView{
			Variant:       v,
			MasterProduct: products[v.ProductID],
			BranchName:    branch,
			StockQuantity: v.StockQty,
		})
	}
	return out
}

// VendorVariantNewPage renders the variant creation form with master product selector.
func (h *UIHandler) VendorVariantNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/variants/new", http.StatusSeeOther)
		return
	}

	var masterProducts []*catalog.Product
	if h.catSvc != nil {
		masterProducts, _ = h.catSvc.Search(ctx, catalog.SearchParams{Limit: 200})
	}

	var branches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	selectedProdID, _ := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)

	data := pages.VendorVariantEditorData{
		MasterProducts: masterProducts,
		Branches:       branches,
		SelectedProdID: selectedProdID,
	}

	h.renderPage(ctx, w, "render new variant page", pages.VendorProductEditor(data, lang, dir))
}

// VendorVariantNewSubmit processes vendor variant creation.
func (h *UIHandler) VendorVariantNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	prodID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	nameAr := r.PostFormValue("name_ar")
	nameEn := r.PostFormValue("name_en")
	batch := r.PostFormValue("batch_number")
	priceStr := r.PostFormValue("price")
	costStr := r.PostFormValue("cost_price")
	costDiscStr := r.PostFormValue("cost_discount_percentage")
	if costDiscStr == "" {
		costDiscStr = r.PostFormValue("cost_discount")
	}
	discountStr := r.PostFormValue("discount")
	stockQty, _ := strconv.Atoi(r.PostFormValue("stock_qty"))
	minQty, _ := strconv.Atoi(r.PostFormValue("min_order_qty"))
	branchIDVal, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	sku := r.PostFormValue("sku")

	if minQty <= 0 {
		minQty = 1
	}

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
	if expStr := r.PostFormValue("expiry_date"); expStr != "" {
		if t, err := time.Parse("2006-01-02", expStr); err == nil {
			expiryDate = &t
		}
	}

	price, _ := money.Parse(priceStr)
	var cost *money.Amount
	if costStr != "" {
		if c, err := money.Parse(costStr); err == nil && c.IsPositive() {
			cost = &c
		}
	}
	costDiscount, _ := strconv.ParseFloat(costDiscStr, 64)
	if costDiscount < 0 {
		costDiscount = 0
	} else if costDiscount > 100 {
		costDiscount = 100
	}

	discount, _ := money.Parse(discountStr)
	isNegotiable := r.PostFormValue("is_negotiable") == "true" || r.PostFormValue("is_negotiable") == "1"

	variant := &catalog.ProductVariant{
		OrganizationID:         actor.OrganizationID,
		ProductID:              prodID,
		Name:                   i18n.New(nameAr, nameEn),
		BatchNumber:            batch,
		ExpiryDate:             expiryDate,
		Price:                  price,
		CostPrice:              cost,
		CostDiscountPercentage: costDiscount,
		Discount:               discount,
		StockQty:               stockQty,
		MinOrderQty:            minQty,
		BranchID:               branchID,
		SKU:                    sku,
		IsNegotiable:           isNegotiable,
		Status:                 catalog.StatusActive,
	}

	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/variants/new", "error", i18n.T(langOf(r), "common.catalog_service_unavailable"))
		return
	}
	created, err := h.catSvc.CreateVariant(ctx, variant)
	if err != nil {
		h.log.ErrorContext(ctx, "create variant error", "error", err)
		h.redirectWithNotice(w, r, "/vendor/variants/new", "error", h.safeMessage(err, langOf(r)))
		return
	}

	// The stock number the vendor typed does not live on the variant —
	// catalog.product_variants has no stock column. It belongs in
	// inventory.stocks against a warehouse. Writing it there is the difference
	// between "50 in stock" being real and being silently discarded, which is
	// what happened before.
	if stockQty > 0 && created != nil {
		if err := h.recordInitialStock(ctx, actor.OrganizationID, created, stockQty); err != nil {
			h.log.WarnContext(ctx, "variant created but its opening stock could not be recorded",
				"error", err, "variant", created.ID, "org", actor.OrganizationID, "qty", stockQty)
			h.redirectWithNotice(w, r, "/vendor/products", "error",
				i18n.T(langOf(r), "vendor.variant.initial_stock_warning"))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", i18n.T(langOf(r), "vendor.variant.published_success"))
}

// VendorVariantDeleteSubmit removes a supplier's variant offer and clears associated warehouse stocks.
func (h *UIHandler) VendorVariantDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid variant ID", nil))
		return
	}

	if h.catSvc != nil {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
		if err := h.catSvc.DeleteVariant(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/vendor/products", "error", i18n.T(langOf(r), "vendor.variant.delete_error_prefix")+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", i18n.T(langOf(r), "vendor.variant.deleted_success"))
}
