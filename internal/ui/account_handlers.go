package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// WalletPage redirects to the unified settings wallet tab.
func (h *UIHandler) WalletPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings?tab=wallet", http.StatusMovedPermanently)
}

// WalletDepositSubmit handles submitting a funds deposit request and crediting the wallet.
func (h *UIHandler) WalletDepositSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings?tab=wallet", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "يرجى إدخال مبلغ إيداع صالح.")
		return
	}

	method := r.PostFormValue("payment_method")
	ref := r.PostFormValue("reference_number")
	notes := r.PostFormValue("notes")

	desc := fmt.Sprintf("إيداع رصيد عبر %s (مرجع: %s)", method, ref)
	if notes != "" {
		desc += " - " + notes
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "خدمة المحفظة والفواتير غير متوفرة.")
		return
	}

	if _, err := h.billSvc.Deposit(ctx, actor.UserID, "EGP", amt, "user_deposit", nil, desc); err != nil {
		h.log.ErrorContext(ctx, "failed wallet deposit", "error", err)
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "فشل إتمام عملية الإيداع: "+err.Error())
		return
	}

	h.redirectWithNotice(w, r, "/settings?tab=wallet", "success", "تم إيداع الرصيد وتحديث المحفظة بنجاح.")
}

// WalletWithdrawSubmit handles submitting a funds withdrawal request and debiting the wallet.
func (h *UIHandler) WalletWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings?tab=wallet", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "يرجى إدخال مبلغ سحب صالح.")
		return
	}

	dest := r.PostFormValue("destination_id")
	reason := r.PostFormValue("reason")
	desc := fmt.Sprintf("طلب سحب رصيد إلى: %s", dest)
	if reason != "" {
		desc += fmt.Sprintf(" (السبب: %s)", reason)
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "خدمة المحفظة والفواتير غير متوفرة.")
		return
	}

	if _, err := h.billSvc.Withdraw(ctx, actor.UserID, "EGP", amt, "user_withdrawal", nil, desc); err != nil {
		h.log.ErrorContext(ctx, "failed wallet withdrawal", "error", err)
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "فشل إتمام عملية السحب: "+err.Error())
		return
	}

	h.redirectWithNotice(w, r, "/settings?tab=wallet", "success", "تم خصم وتسجيل طلب السحب بنجاح.")
}

// InvoicesPage renders the organization's invoice list with status badges.
func (h *UIHandler) InvoicesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/invoices", http.StatusSeeOther)
		return
	}

	data := pages.InvoicesData{}
	if h.billSvc != nil {
		if invoices, err := h.billSvc.ListInvoices(ctx, actor.OrganizationID, 20, 0); err != nil {
			h.log.WarnContext(ctx, "account: list invoices", "error", err)
		} else {
			data.Invoices = invoices
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.InvoicesPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render invoices page", "error", err)
	}
}

// FavoritesPage lists the signed-in user's favourited products.
func (h *UIHandler) FavoritesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	var products []*catalog.Product
	if h.idSvc != nil && h.catSvc != nil {
		ids, err := h.idSvc.ListFavorites(ctx, actor.UserID)
		if err != nil {
			h.log.WarnContext(ctx, "favorites: list favorites", "error", err)
		} else {
			for _, id := range ids {
				if p, _, err := h.catSvc.GetProduct(ctx, id); err != nil {
					h.log.DebugContext(ctx, "favorites: get product optional", "id", id, "error", err)
				} else if p != nil {
					products = append(products, p)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.FavoritesPage(lang, dir, products).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render favorites page", "error", err)
	}
}

// FavoriteRemoveSubmit removes a product from the user's favourites.
func (h *UIHandler) FavoriteRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	if h.idSvc != nil {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			h.log.DebugContext(ctx, "favorite remove: invalid id", "error", err)
		} else {
			if err := h.idSvc.RemoveFavorite(ctx, actor.UserID, id); err != nil {
				h.log.WarnContext(ctx, "favorite remove: failed", "id", id, "error", err)
			}
		}
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// FavoriteAddSubmit adds a product to the user's favourites.
func (h *UIHandler) FavoriteAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	productID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if productID <= 0 {
		productID, _ = strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	}

	if h.idSvc != nil && productID > 0 {
		if err := h.idSvc.AddFavorite(ctx, actor.UserID, productID); err != nil {
			h.log.WarnContext(ctx, "favorite add: failed", "product_id", productID, "error", err)
		}
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// FavoriteToggleSubmit toggles a product in the user's favourites.
func (h *UIHandler) FavoriteToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	productID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if productID <= 0 {
		productID, _ = strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	}

	if h.idSvc != nil && productID > 0 {
		favs, err := h.idSvc.ListFavorites(ctx, actor.UserID)
		isFav := false
		if err != nil {
			h.log.WarnContext(ctx, "favorite toggle: list favorites", "error", err)
		} else {
			for _, id := range favs {
				if id == productID {
					isFav = true
					break
				}
			}
		}
		if isFav {
			_ = h.idSvc.RemoveFavorite(ctx, actor.UserID, productID)
		} else {
			_ = h.idSvc.AddFavorite(ctx, actor.UserID, productID)
		}
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
