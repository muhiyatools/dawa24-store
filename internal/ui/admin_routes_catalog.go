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
		g.Get("/admin/apis-products", h.AdminApisProductsPage)
		g.Get("/admin/products/import", h.AdminAdvProductsPage)
		g.Get("/admin/import", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/products/import", http.StatusMovedPermanently)
		})
		g.Get("/admin/products/sample.csv", h.AdminProductsSampleCSV)
		g.Get("/admin/products/sample.xlsx", h.AdminProductsSampleXLSX)

		// Inventory & Warehouses
		g.Get("/admin/stocks", h.AdminStocksPage)
		g.Get("/admin/warehouses", h.AdminWarehousesPage)
		g.Get("/admin/warehouses/{id}", h.AdminWarehouseDetailPage)

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
		g.Get("/admin/products-saving/import", h.AdminAdvProductsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.update", h.log))
		g.Post("/admin/products/new", h.AdminProductCreateSubmit)
		g.Post("/admin/products/{id}/edit", h.AdminProductEditSubmit)
		g.Post("/admin/products/{id}/status", h.AdminProductStatusSubmit)
		g.Post("/admin/products/import", h.AdminProductsImportSubmit)
		g.Post("/admin/upload-warehouse-file", h.AdminProductsImportSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.delete", h.log))
		g.Post("/admin/products/{id}/delete", h.AdminProductDeleteSubmit)
	})
}
