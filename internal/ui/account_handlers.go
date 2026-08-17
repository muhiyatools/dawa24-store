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

	data := pages.WalletData{Currency: "EGP"}
	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err == nil && wallet != nil {
			data.Balance = wallet.Balance
			if txs, err := h.billSvc.ListWalletTransactions(ctx, wallet.ID, 20, 0); err == nil {
				data.Transactions = txs
			}
		}
		if methods, err := h.billSvc.ListPaymentMethods(ctx, actor.UserID); err == nil {
			data.PaymentMethods = methods
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.WalletPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render wallet page", "error", err)
	}
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

	http.Redirect(w, r, "/favorites", http.StatusSeeOther)
}
