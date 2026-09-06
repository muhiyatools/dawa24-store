package ui

import (
	"net/http"
)

// VendorSavingProductsImportSubmit handles Excel/CSV bulk import with intelligent auto-matching for vendor.
func (h *UIHandler) VendorSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSubmit(w, r, "vendor")
}

// VendorSavingProductsPreviewColumnsJSON reads uploaded spreadsheet and returns headers and detected columns.
func (h *UIHandler) VendorSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPreviewColumnsJSON(w, r)
}

// VendorSavingProductsImportStartJSON initiates asynchronous processing of an uploaded file for vendor.
func (h *UIHandler) VendorSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportStartJSON(w, r, "vendor")
}

// VendorSavingProductsImportProgressJSON returns the live state and staged items for review for vendor.
func (h *UIHandler) VendorSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportProgressJSON(w, r)
}

// VendorSavingProductsImportCommitJSON commits the staged session into catalog.saving_products for vendor.
func (h *UIHandler) VendorSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitJSON(w, r)
}

// VendorSavingProductsImportCancelJSON discards and clears the staged session for vendor.
func (h *UIHandler) VendorSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelJSON(w, r)
}
