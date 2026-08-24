package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminCatalogRoutes(r chi.Router) {
	// Products & Catalog
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.view", h.log))
		g.Get("/admin/products", h.AdminProductsPage)
		g.Get("/admin/products/{id}", h.AdminProductDetailPage)
		g.Get("/admin/product-child", h.AdminProductChildrenPage)
		g.Get("/admin/product-child/{id}", h.AdminProductDetailPage)
		g.Get("/admin/adv-products", h.AdminAdvProductsPage)
		g.Get("/admin/products/import", h.AdminProductsImportPage)
		g.Get("/admin/products/import/{id}", h.AdminProductsImportReviewPage)
		g.Get("/admin/import", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/products/import", http.StatusMovedPermanently)
		})
		g.Get("/admin/products/sample.csv", h.AdminProductsSampleCSV)
		g.Get("/admin/products/sample.xlsx", h.AdminProductsSampleXLSX)

		// Inventory & Warehouses
		g.Get("/admin/stocks", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/warehouses", http.StatusMovedPermanently)
		})
		g.Get("/admin/warehouses", h.AdminWarehousesPage)
		g.Get("/admin/warehouses/{id}", h.AdminWarehouseDetailPage)
		g.Get("/admin/warehouses/{id}/stocks-json", h.AdminWarehouseStocksJSON)

		// Temp Warehouses
		g.Get("/admin/user/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/user/temparte-warehouses/{id}", h.AdminTempWarehousesPage)
		g.Get("/admin/my/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/import/temparte-warehouses", h.AdminAdvProductsPage)
		g.Get("/admin/admins/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/plan/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/plan/temparte-warehouses/{id}", h.AdminTempWarehousesPage)
		g.Get("/admin/user-plan/temparte-warehouses", h.AdminTempWarehousesPage)

		// Saving Products (and 301 misspelling alias)
		g.Get("/admin/saving-products", h.AdminSavingProductsPage)
		g.Get("/admin/saveing-products", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/saving-products", http.StatusMovedPermanently)
		})
		g.Get("/admin/saving-products/user/{userId}", h.AdminSavingProductsPage)
		g.Get("/admin/saving-products/org/{organizationId}", h.AdminSavingProductsPage)
		// Brands & Categories
		g.Get("/admin/brands", h.AdminBrandsPage)
		g.Get("/admin/categories", h.AdminCategoriesPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.update", h.log))
		g.Post("/admin/products/new", h.AdminProductCreateSubmit)
		g.Post("/admin/products/{id}/edit", h.AdminProductEditSubmit)
		g.Post("/admin/products/{id}/status", h.AdminProductStatusSubmit)
		g.Post("/admin/products/import", h.AdminProductsImportSubmit)
		g.Post("/admin/upload-warehouse-file", h.AdminProductsImportSubmit)
		// The staged review: re-run the file under corrected settings, adjust
		// which rows are included, then commit or discard. Only the commit
		// route writes to the catalogue.
		g.Post("/admin/products/import/{id}/prepare", h.AdminProductsImportPrepare)
		g.Post("/admin/products/import/{id}/rows/{rowID}", h.AdminProductsImportRowToggle)
		g.Post("/admin/products/import/{id}/select", h.AdminProductsImportSelect)
		g.Post("/admin/products/import/{id}/commit", h.AdminProductsImportCommit)
		g.Post("/admin/products/import/{id}/cancel", h.AdminProductsImportCancel)
		g.Post("/admin/brands/new", h.AdminBrandCreateSubmit)
		g.Post("/admin/brands/{id}/edit", h.AdminBrandEditSubmit)
		g.Post("/admin/brands/{id}/status", h.AdminBrandStatusSubmit)
		g.Post("/admin/categories/new", h.AdminCategoryCreateSubmit)
		g.Post("/admin/categories/{id}/edit", h.AdminCategoryEditSubmit)
		g.Post("/admin/categories/{id}/toggle", h.AdminCategoryToggleSubmit)
		g.Post("/admin/categories/{id}/status", h.AdminCategoryToggleSubmit)
		g.Post("/admin/product-child/{id}/status", h.AdminProductChildStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.delete", h.log))
		g.Post("/admin/products/{id}/delete", h.AdminProductDeleteSubmit)
		g.Post("/admin/brands/{id}/delete", h.AdminBrandDeleteSubmit)
		g.Post("/admin/categories/{id}/delete", h.AdminCategoryDeleteSubmit)
	})
}
