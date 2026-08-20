package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerCatalogPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	lang, dir := h.localeAndDir(r)

	var categoryID *int64
	if v := r.URL.Query().Get("category_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			categoryID = &id
		}
	}

	var minPrice, maxPrice *money.Amount
	minPriceStr := r.URL.Query().Get("min_price")
	maxPriceStr := r.URL.Query().Get("max_price")
	if minPriceStr != "" {
		if a, err := money.Parse(minPriceStr); err == nil {
			minPrice = &a
		}
	}
	if maxPriceStr != "" {
		if a, err := money.Parse(maxPriceStr); err == nil {
			maxPrice = &a
		}
	}

	dosageForm := r.URL.Query().Get("dosage_form")
	sort := r.URL.Query().Get("sort")
	inStock := r.URL.Query().Get("in_stock") == "true"

	if h.catSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCatalog(pages.CatalogPageData{
			Query: query,
		}, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer catalog", "error", err)
		}
		return
	}

	products, err := h.catSvc.Search(ctx, catalog.SearchParams{
		Query:      query,
		CategoryID: categoryID,
		Sort:       sort,
		MinPrice:   minPrice,
		MaxPrice:   maxPrice,
		Limit:      h.pageLimit(r),
		Offset:     h.pageOffset(r),
	})
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	categories, _ := h.catSvc.ListCategories(ctx)

	viewData := pages.CatalogPageData{
		Products:   products,
		Categories: categories,
		Query:      query,
		CategoryID: categoryID,
		MinPrice:   minPriceStr,
		MaxPrice:   maxPriceStr,
		DosageForm: dosageForm,
		Sort:       sort,
		InStock:    inStock,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCatalog(viewData, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render catalog page", "error", err)
	}
}

func (h *UIHandler) CustomerProductDetailPage(w http.ResponseWriter, r *http.Request) {
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

	product, variants, err := h.catSvc.GetProduct(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	offers := h.offersForProduct(ctx, product, variants)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerProductDetail(product, variants, offers, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render product detail page", "error", err)
	}
}

func (h *UIHandler) CustomerCartPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/catalog", "error", "عذراً، الشراء وسلة الطلبات متاحة حصرياً للصيدليات المرخصة.")
		return
	}

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCart(nil, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render cart page", "error", err)
		}
		return
	}

	cart, err := h.commSvc.GetCart(ctx, actor.UserID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCart(cart, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render cart page", "error", err)
	}
}

func (h *UIHandler) CustomerCheckoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/catalog", "error", "عذراً، إتمام الشراء والتوريد متاح حصرياً للصيدليات المرخصة.")
		return
	}

	userID := actor.UserID

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCheckout(nil, nil, lang, dir).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render checkout page", "error", err)
		}
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	var branches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if bList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err != nil {
			h.log.WarnContext(ctx, "checkout: list customer branches", "error", err)
		} else {
			branches = bList
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCheckout(cart, branches, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render checkout page", "error", err)
	}
}

func (h *UIHandler) CustomerOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/orders", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerOrders(nil, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer orders page", "error", err)
		}
		return
	}

	orders, err := h.commSvc.ListCustomerOrders(ctx, userID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOrders(orders, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer orders page", "error", err)
	}
}

func (h *UIHandler) CustomerOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.commSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	order, err := h.commSvc.GetOrder(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	history, _ := h.commSvc.GetOrderHistory(ctx, id)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOrderDetail(order, history, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer order detail page", "error", err)
	}
}

func (h *UIHandler) NotificationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/notifications", http.StatusSeeOther)
		return
	}

	if h.notifSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.Notifications(nil, 0, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render notifications page", "error", err)
		}
		return
	}

	logs, err := h.notifSvc.ListUserNotifications(ctx, userID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	unread, _ := h.notifSvc.GetUnreadCount(ctx, userID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Notifications(logs, unread, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render notifications page", "error", err)
	}
}

func (h *UIHandler) AddToCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/catalog", "error", "عذراً، إضافة الأدوية وطلب التوريد متاح حصرياً للصيدليات المرخصة.")
		return
	}

	userID := actor.UserID
	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty <= 0 {
		qty = 1
	}

	// Stock, supplier approval, branch ownership and weekly coverage are all
	// decided by commerce.CheckAvailability. Nothing here defaults a missing
	// supplier or quietly reduces the quantity the pharmacy asked for.
	if !h.assertCartLineAvailable(w, r, actor, variantID, vendorOrgID, qty, "/catalog") {
		return
	}

	item := &commerce.CartItem{
		ProductID:        productID,
		ProductVariantID: variantID,
		OrganizationID:   vendorOrgID,
		Quantity:         qty,
	}

	if priceStr := r.PostFormValue("offer_price"); priceStr != "" {
		if amt, err := money.Parse(priceStr); err == nil {
			item.UnitPrice = amt
		}
	}

	// Keep the offer identity so checkout can carry it into the order
	// (migration 064). Parsed defensively; a bogus id degrades to a
	// non-offer cart line.
	if offerID, err := strconv.ParseInt(r.PostFormValue("offer_id"), 10, 64); err == nil && offerID > 0 {
		item.OfferID = &offerID
	}

	if h.catSvc != nil && productID > 0 && item.UnitPrice.IsZero() {
		if prod, _, err := h.catSvc.GetProduct(ctx, productID); err == nil && prod != nil {
			item.ProductName = prod.Name
			item.UnitPrice = prod.EffectivePrice()
		}
	}

	if _, err := h.commSvc.AddToCart(ctx, userID, item); err != nil {
		h.log.ErrorContext(ctx, "add to cart", "error", err,
			"user", userID, "variant", variantID, "vendor", vendorOrgID)
		h.redirectWithNotice(w, r, "/catalog", "error", h.safeMessage(err, langOf(r)))
		return
	}
	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

// assertCartLineAvailable runs the availability rule and, when it refuses,
// redirects with the reason. It returns false when the caller must stop.
//
// Every buying surface goes through here so the rules cannot drift: the cart,
// the quantity controls and checkout all ask the same question.
func (h *UIHandler) assertCartLineAvailable(
	w http.ResponseWriter, r *http.Request, actor authctx.Actor,
	variantID, vendorOrgID int64, qty int, back string,
) bool {
	ctx := r.Context()

	branchID := int64(0)
	if actor.BranchID != nil {
		branchID = *actor.BranchID
	}

	res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
		VariantID:        variantID,
		VendorOrgID:      vendorOrgID,
		CustomerOrgID:    actor.OrganizationID,
		CustomerBranchID: branchID,
		Quantity:         qty,
		When:             time.Now(),
	})
	if err != nil {
		// A failed check is not permission to buy.
		h.log.ErrorContext(ctx, "availability check failed", "error", err,
			"variant", variantID, "vendor", vendorOrgID, "branch", branchID)
		h.redirectWithNotice(w, r, back, "error",
			"تعذر التحقق من توفر الصنف حالياً، يرجى المحاولة مرة أخرى.")
		return false
	}
	if !res.Allowed {
		h.log.InfoContext(ctx, "cart line refused", "reason", res.Reason,
			"variant", variantID, "vendor", vendorOrgID, "branch", branchID, "qty", qty)
		h.redirectWithNotice(w, r, back, "error", res.MessageAr)
		return false
	}
	return true
}

func (h *UIHandler) RemoveFromCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	_, _ = h.commSvc.RemoveFromCart(ctx, userID, variantID)

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		lang, _ := h.localeAndDir(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCartContent(cart, lang).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer cart content", "error", err)
		}
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) UpdateCartQuantitySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty < 0 {
		qty = 0
	}

	// Raising a quantity is a purchase decision and gets the same check as
	// adding the line. The client's "+" button is a hint; this is the rule.
	// Quantity 0 means "remove", which needs no availability check.
	if qty > 0 {
		actor, ok := authctx.From(ctx)
		if !ok {
			http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
			return
		}
		vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
		if vendorOrgID <= 0 {
			// The cart row knows its supplier even when the form omits it.
			if line, err := h.commSvc.GetCartLine(ctx, userID, variantID); err == nil && line != nil {
				vendorOrgID = line.OrganizationID
			}
		}
		if !h.assertCartLineAvailable(w, r, actor, variantID, vendorOrgID, qty, "/cart") {
			return
		}
	}

	if _, err := h.commSvc.SetCartQuantity(ctx, userID, variantID, qty); err != nil {
		h.log.ErrorContext(ctx, "set cart quantity", "error", err,
			"user", userID, "variant", variantID, "qty", qty)
		h.redirectWithNotice(w, r, "/cart", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		lang, _ := h.localeAndDir(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerCartContent(cart, lang).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render customer cart content", "error", err)
		}
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) CheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/orders", http.StatusSeeOther)
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil || cart == nil || len(cart.Items) == 0 {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	var items []commerce.CheckoutLineItem
	var offerID int64
	for _, it := range cart.Items {
		pID := it.ProductID
		vID := it.ProductVariantID
		vOrgID := it.OrganizationID
		if vOrgID <= 0 {
			vOrgID = 1
		}
		uPrice := it.UnitPrice
		if uPrice.IsZero() {
			uPrice, _ = money.Parse("38.50")
		}
		pName := it.ProductName
		if len(pName) == 0 {
			pName = i18n.Text{"ar": "صنف دوائي معتمد", "en": "Certified Medicine"}
		}
		items = append(items, commerce.CheckoutLineItem{
			VendorOrgID:      vOrgID,
			ProductID:        &pID,
			ProductVariantID: &vID,
			ProductName:      pName,
			Quantity:         it.Quantity,
			UnitPrice:        uPrice,
		})
		// One offer per order (main_orders parity). If the cart mixes offers,
		// the order degrades to a legacy non-offer order — the cart-per-offer
		// UI is Phase 5.
		if it.OfferID != nil {
			if offerID == 0 {
				offerID = *it.OfferID
			} else if offerID != *it.OfferID {
				offerID = 0
			}
		}
	}

	paymentMethod := r.PostFormValue("payment_method")
	if paymentMethod == "" {
		paymentMethod = "cod"
	}

	var branchID *int64
	if bID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64); err == nil && bID > 0 {
		branchID = &bID
	}

	input := commerce.CheckoutInput{
		CustomerID:    userID,
		BranchID:      branchID,
		PaymentMethod: paymentMethod,
		Notes:         r.PostFormValue("notes"),
		Items:         items,
	}
	if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		input.CustomerOrgID = actor.OrganizationID
	}
	if offerID > 0 {
		input.OfferID = offerID
		// The offer is the authority for the minimum order amount and the
		// fulfilling vendor branch; the buying branch comes from the shell
		// selector, validated against the actor's own branches.
		if h.promoSvc != nil {
			if offer, err := h.promoSvc.GetOffer(ctx, offerID); err == nil && offer != nil {
				input.MinOrderAmount = offer.MinOrderAmount
				if offer.BranchID != nil && *offer.BranchID > 0 {
					input.VendorBranchID = offer.BranchID
				}
			}
		}
		if input.BranchID == nil {
			if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil {
				input.BranchID = buying.Active
			}
		}
	}

	order, err := h.commSvc.Checkout(ctx, input)
	if err != nil {
		h.log.ErrorContext(ctx, "checkout failed", "error", err)
		h.renderError(w, r, err)
		return
	}

	_ = h.commSvc.ClearCart(ctx, userID)
	http.Redirect(w, r, "/orders/"+strconv.FormatInt(order.ID, 10), http.StatusSeeOther)
}

func (h *UIHandler) MarkNotificationReadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, _ := authctx.UserID(ctx)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.notifSvc != nil && id > 0 {
		_ = h.notifSvc.MarkRead(ctx, id, userID)
	}
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// CustomerBranchesPage renders the pharmacy's own branches screen in CustomerShell.
func (h *UIHandler) CustomerBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	var branches []*org.Branch
	if h.orgSvc != nil {
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	data := pages.CustomerBranchesData{
		Branches: branches,
		Cities:   h.listCities(ctx),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerBranches(data, lang, dir, actor.Permissions).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer branches page", "error", err)
	}
}

// CustomerBranchNewSubmit creates a new pharmacy branch for the customer organization.
func (h *UIHandler) CustomerBranchNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()

	nameAr := r.PostFormValue("name_ar")
	nameEn := r.PostFormValue("name_en")
	if nameAr == "" {
		nameAr = "فرع صيدلية"
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := r.PostFormValue("code")
	address := r.PostFormValue("address")
	phone := r.PostFormValue("phone")
	gmaps := r.PostFormValue("google_maps_url")
	isMain := r.PostFormValue("is_main") == "true"

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	var latPtr, lngPtr *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}

	b := &org.Branch{
		OrganizationID: actor.OrganizationID,
		Name:           i18n.New(nameAr, nameEn),
		Code:           code,
		WarehouseType:  "pharmacy",
		Address:        address,
		Phone:          phone,
		GoogleMapsURL:  gmaps,
		CityID:         cityID,
		Latitude:       latPtr,
		Longitude:      lngPtr,
		IsMain:         isMain,
		Status:         "active",
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.CreateBranch(ctx, b); err != nil {
			h.log.ErrorContext(ctx, "customer create branch error", "error", err)
			h.redirectWithNotice(w, r, "/customer/branches", "error", "فشل في حفظ فرع الصيدلية: "+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", "تم إضافة فرع الصيدلية بنجاح.")
}

// CustomerBranchDeleteSubmit deletes a branch owned by the customer organization.
func (h *UIHandler) CustomerBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "معرف الفرع غير صالح.")
		return
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.DeleteBranch(ctx, actor.OrganizationID, id); err != nil {
			h.redirectWithNotice(w, r, "/customer/branches", "error", "فشل في حذف الفرع: "+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", "تم حذف الفرع بنجاح.")
}

// ReviewSubmit handles customer feedback submissions with multi-criteria rating.
func (h *UIHandler) ReviewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	targetOrgID, _ := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	if targetOrgID <= 0 {
		h.redirectWithNotice(w, r, "/suppliers", "error", "المؤسسة المستهدفة غير صالحة.")
		return
	}

	var orderID *int64
	if oIDStr := r.PostFormValue("order_id"); oIDStr != "" {
		if oID, err := strconv.ParseInt(oIDStr, 10, 64); err == nil && oID > 0 {
			orderID = &oID
		}
	}

	repScore, _ := strconv.Atoi(r.PostFormValue("rating_rep"))
	speedScore, _ := strconv.Atoi(r.PostFormValue("rating_speed"))
	qualityScore, _ := strconv.Atoi(r.PostFormValue("rating_quality"))

	if repScore < 1 {
		repScore = 5
	}
	if speedScore < 1 {
		speedScore = 5
	}
	if qualityScore < 1 {
		qualityScore = 5
	}

	rev := &org.Review{
		OrganizationID: targetOrgID,
		UserID:         actor.UserID,
		OrderID:        orderID,
		Title:          r.PostFormValue("title"),
		ReviewText:     r.PostFormValue("review_text"),
		Context:        r.PostFormValue("context"),
		IsApproved:     true,
		Ratings: []org.ReviewRating{
			{Criterion: "rep", Score: repScore},
			{Criterion: "speed", Score: speedScore},
			{Criterion: "quality", Score: qualityScore},
		},
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.SubmitReview(ctx, rev); err != nil {
			h.log.ErrorContext(ctx, "failed to submit review", "error", err, "target_org_id", targetOrgID)
			h.redirectWithNotice(w, r, fmt.Sprintf("/suppliers/%d", targetOrgID), "error", "فشل إضافة التقييم: "+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/suppliers/%d", targetOrgID), "success", "تم إرسال تقييمك بنجاح. شكراً لمشاركتك!")
}

// CustomerSwitchActiveBranchSubmit handles switching the active branch for customer context.
func (h *UIHandler) CustomerSwitchActiveBranchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsCustomer() {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	branchIDStr := r.FormValue("branch_id")
	if branchIDStr != "" {
		if branchID, err := strconv.ParseInt(branchIDStr, 10, 64); err == nil && branchID > 0 {
			h.log.InfoContext(ctx, "switched active pharmacy branch", "branch_id", branchID, "org_id", actor.OrganizationID)
		}
	}

	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = "/customer/dashboard"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}
