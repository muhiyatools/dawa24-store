package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerPurchaseRequestWizardPage renders the clean gateway page to catalog and suppliers.
func (h *UIHandler) CustomerPurchaseRequestWizardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerPurchaseRequestWizardPage(lang, dir, 1, "").Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render purchase request wizard", "error", err)
	}
}

// Legacy redirects for deleted duplicate routes
func (h *UIHandler) CustomerPurchaseRequestProductsRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/catalog", http.StatusMovedPermanently)
}

func (h *UIHandler) CustomerPurchaseRequestPreviousRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/orders", http.StatusMovedPermanently)
}

func (h *UIHandler) CustomerPurchaseRequestSupplierRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/suppliers", http.StatusMovedPermanently)
}
