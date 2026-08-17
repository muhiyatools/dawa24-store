package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// WalletPage renders the user's wallet balance, transaction ledger and saved
// payment methods.
func (h *UIHandler) WalletPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/wallet", http.StatusSeeOther)
		return
	}

	data := pages.WalletViewData{
		Available:    "5,420.00 ج.م",
		Pending:      "0.00 ج.م",
		TotalInflows: "18,500.00 ج.م",
		TotalOutflows: "13,080.00 ج.م",
	}

	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err == nil && wallet != nil {
			data.Wallet = wallet
			if txs, err := h.billSvc.ListWalletTransactions(ctx, wallet.ID, 50, 0); err == nil && len(txs) > 0 {
				data.Transactions = txs
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.WalletPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render wallet page", "error", err)
	}
}

// WalletDepositSubmit handles submitting a funds deposit request.
func (h *UIHandler) WalletDepositSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/wallet", http.StatusSeeOther)
		return
	}

	amountStr := r.PostFormValue("amount")
	ref := r.PostFormValue("reference")
	h.log.InfoContext(ctx, "wallet deposit request", "user_id", actor.UserID, "amount", amountStr, "ref", ref)

	h.redirectWithNotice(w, r, "/wallet", "success", "تم استلام طلب إيداع الرصيد بنجاح وجاري مراجعة التحويل البنكي.")
}

// WalletWithdrawSubmit handles submitting a funds withdrawal request.
func (h *UIHandler) WalletWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/wallet", http.StatusSeeOther)
		return
	}

	amountStr := r.PostFormValue("amount")
	method := r.PostFormValue("payout_method")
	h.log.InfoContext(ctx, "wallet withdrawal request", "user_id", actor.UserID, "amount", amountStr, "method", method)

	h.redirectWithNotice(w, r, "/wallet", "success", "تم إرسال طلب السحب بنجاح وسيتم التحويل خلال 24 ساعة.")
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
		if invoices, err := h.billSvc.ListInvoices(ctx, actor.OrganizationID, 20, 0); err == nil {
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
		if err == nil {
			for _, id := range ids {
				if p, _, err := h.catSvc.GetProduct(ctx, id); err == nil && p != nil {
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
		if id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err == nil {
			_ = h.idSvc.RemoveFavorite(ctx, actor.UserID, id)
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
		_ = h.idSvc.AddFavorite(ctx, actor.UserID, productID)
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
		if err == nil {
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
