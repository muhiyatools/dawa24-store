package ui

import (
	"net/http"
)

// CustomerSavingProductsImportSubmit handles Excel/CSV bulk import of saving products for pharmacy.
func (h *UIHandler) CustomerSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSubmit(w, r, "customer")
}

// CustomerSavingProductsPreviewColumnsJSON reads uploaded spreadsheet and returns headers and detected columns.
func (h *UIHandler) CustomerSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPreviewColumnsJSON(w, r)
}

// CustomerSavingProductsImportStartJSON initiates asynchronous processing of an uploaded file for pharmacy.
func (h *UIHandler) CustomerSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportStartJSON(w, r, "customer")
}

// CustomerSavingProductsImportProgressJSON returns the live state and staged items for review for pharmacy.
func (h *UIHandler) CustomerSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportProgressJSON(w, r)
}

// CustomerSavingProductsImportCommitJSON commits the staged session into catalog.saving_products for pharmacy.
func (h *UIHandler) CustomerSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitJSON(w, r)
}

// CustomerSavingProductsImportCancelJSON discards and clears the staged session for pharmacy.
func (h *UIHandler) CustomerSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelJSON(w, r)
}

