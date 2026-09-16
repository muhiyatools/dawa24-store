package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestRegisterUploadRoutes_BlocksPrivateDocumentCategories(t *testing.T) {
	r := chi.NewRouter()
	ui.RegisterUploadRoutes(r)

	privatePaths := []string{
		"/uploads/licenses/license_12345678abcdef01.pdf",
		"/uploads/documents/doc_12345678abcdef01.pdf",
		"/uploads/receipts/receipt_12345678abcdef01.pdf",
		"/uploads/cvs/cv_12345678abcdef01.pdf",
		"/uploads/resumes/resume_12345678abcdef01.pdf",
		"/uploads/compare/sheet_12345678abcdef01.xlsx",
		"/uploads/imports/batch_12345678abcdef01.csv",
		"/uploads/temp_warehouses/tw_12345678abcdef01.xlsx",
	}

	for _, path := range privatePaths {
		t.Run("private_"+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusForbidden, rec.Code, "expected 403 Forbidden for private upload category: %s", path)
			assert.Contains(t, rec.Body.String(), "Forbidden: private document")
		})
	}

	// Public media categories must NOT be blocked by the private category check (they fall through to 404 if file is not on disk)
	publicPaths := []string{
		"/uploads/products/non_existent_product.jpg",
		"/uploads/avatars/non_existent_avatar.png",
		"/uploads/brands/non_existent_brand.webp",
		"/uploads/ads/non_existent_ad.png",
		"/uploads/offers/non_existent_offer.jpg",
	}

	for _, path := range publicPaths {
		t.Run("public_"+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code, "expected 404 Not Found (not 403) for public upload path: %s", path)
		})
	}

	// Path traversal attempts must be rejected with 403
	t.Run("path_traversal", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/uploads/..%2F..%2Fetc%2Fpasswd", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}
