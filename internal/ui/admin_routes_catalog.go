package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// The catalogue and inventory surface.
//
// The gates are one per section rather than one per module. "catalog.product.view"
// used to open the drug catalogue, the vendor items list, the promoted
// products screen, both importers, the warehouses, the temporary warehouses,
// the saving products and the AI match-decision memory — nine screens over
// eight different tables. There was no way to give a catalogue editor the
// catalogue without also giving them every vendor's stock.
func (h *UIHandler) registerAdminCatalogRoutes(r chi.Router) {
	h.registerAdminProductRoutes(r)
	h.registerAdminImportRoutes(r)
	h.registerAdminWarehouseRoutes(r)
	h.registerAdminCatalogMutations(r)
}

func (h *UIHandler) registerAdminProductRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.view"))
		g.Get("/admin/products", h.AdminProductsPage)
		g.Get("/admin/products/{id}", h.AdminProductDetailPage)
		g.Get("/admin/products/sample.csv", h.AdminProductsSampleCSV)
		g.Get("/admin/products/sample.xlsx", h.AdminProductsSampleXLSX)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.vendor_product.view"))
		g.Get("/admin/product-child", h.AdminProductChildrenPage)
		g.Get("/admin/product-child/{id}", h.AdminProductDetailPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.adv_product.view"))
		g.Get("/admin/adv-products", h.AdminAdvProductsPage)
		g.Post("/admin/adv-products/{id}/approve", h.AdminAdvProductApproveSubmit)
		g.Post("/admin/adv-products/{id}/reject", h.AdminAdvProductRejectSubmit)
		g.Post("/admin/adv-products/new", h.AdminAdvProductCreateSubmit)
		g.Get("/admin/import/temparte-warehouses", h.AdminAdvProductsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.saving_product.view"))
		g.Get("/admin/saving-products", h.AdminSavingProductsPage)
		g.Get("/admin/saveing-products", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/saving-products", http.StatusMovedPermanently)
		})
		g.Get("/admin/saving-products/user/{userId}", h.AdminSavingProductsPage)
		g.Get("/admin/saving-products/org/{organizationId}", h.AdminSavingProductsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.match_decision.view"))
		g.Get("/admin/match-decisions", h.AdminMatchDecisionsPage)
		g.Post("/admin/match-decisions/toggle-state", h.AdminMatchDecisionToggleStateSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.match_decision.delete"))
		g.Post("/admin/match-decisions/{id}/delete", h.AdminMatchDecisionDeleteSubmit)
		g.Post("/admin/match-decisions/clear", h.AdminMatchDecisionsClearSubmit)
	})
}

func (h *UIHandler) registerAdminImportRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.import.view"))
		g.Get("/admin/products/import", h.AdminProductsImportPage)
		g.Get("/admin/products/import/{id}", h.AdminProductsImportReviewPage)
		g.Get("/admin/products/import/{id}/mapping", h.AdminProductsImportMappingPage)
		g.Get("/admin/products/import/{id}/progress", h.AdminProductsImportProgress)
		g.Get("/admin/import", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/products/import", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.import.run"))
		g.Post("/admin/products/import", h.AdminProductsImportSubmit)
		g.Post("/admin/upload-warehouse-file", h.AdminProductsImportSubmit)
		// The staged review: re-run the file under corrected settings, adjust
		// which rows are included, then commit or discard. Only the commit
		// route writes to the catalogue.
		g.Post("/admin/products/import/{id}/preview", h.AdminProductsImportPreview)
		g.Post("/admin/products/import/{id}/prepare", h.AdminProductsImportPrepare)
		g.Post("/admin/products/import/{id}/rows/{rowID}", h.AdminProductsImportRowToggle)
		g.Post("/admin/products/import/{id}/select", h.AdminProductsImportSelect)
		g.Post("/admin/products/import/{id}/commit", h.AdminProductsImportCommit)
		g.Post("/admin/products/import/{id}/cancel", h.AdminProductsImportCancel)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.image_import.view"))
		g.Get("/admin/products/images/import", h.AdminProductImagesImportPage)
		g.Get("/admin/products/images/import/{id}", h.AdminProductImagesSessionPage)
		g.Get("/admin/products/images/import/{id}/progress", h.AdminProductImagesProgressJSON)
		g.Get("/admin/products/images/import/sample.xlsx", h.AdminProductImagesSampleXLSX)
		g.Get("/admin/products/images/import/sample.csv", h.AdminProductImagesSampleCSV)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.image_import.run"))
		g.Post("/admin/products/images/import/upload", h.AdminProductImagesUploadSubmit)
		g.Post("/admin/products/images/import/{id}/mapping", h.AdminProductImagesMappingSubmit)
		g.Post("/admin/products/images/import/{id}/cancel", h.AdminProductImagesCancelSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.org_import.view"))
		g.Get("/admin/organizations/import", h.AdminOrgImportPage)
		g.Get("/admin/organizations/import/saving/{id}", h.AdminOrgImportSavingSessionPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.org_import.run"))
		g.Post("/admin/organizations/import/saving/upload", h.AdminOrgImportSavingsUploadSubmit)
		g.Post("/admin/organizations/import/saving/{id}/map", h.AdminOrgImportSavingSessionMapSubmit)
		g.Post("/admin/organizations/import/saving/{id}/commit", h.AdminOrgImportSavingSessionCommitSubmit)
		g.Post("/admin/organizations/import/saving/{id}/cancel", h.AdminOrgImportSavingSessionCancelSubmit)
		g.Post("/admin/organizations/import/temp-warehouse/upload", h.AdminOrgImportTempWarehouseUploadSubmit)
	})
}

func (h *UIHandler) registerAdminWarehouseRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.warehouse.view"))
		g.Get("/admin/stocks", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/warehouses", http.StatusMovedPermanently)
		})
		g.Get("/admin/warehouses", h.AdminWarehousesPage)
		g.Get("/admin/warehouses/{id}", h.AdminWarehouseDetailPage)
		g.Get("/admin/warehouses/{id}/stocks-json", h.AdminWarehouseStocksJSON)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.temp_warehouse.view"))
		g.Get("/admin/temporary-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/temporary-warehouses/{id}/items-json", h.AdminTempWarehouseItemsJSON)
		g.Get("/admin/temporary-warehouses/{id}/mapping-json", h.AdminTempWarehouseMappingJSON)
		g.Get("/admin/temporary-warehouses/{id}/export", h.AdminTempWarehouseExportXLSX)
		g.Get("/admin/user/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/user/temparte-warehouses/{id}", h.AdminTempWarehousesPage)
		g.Get("/admin/admins/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/plan/temparte-warehouses", h.AdminTempWarehousesPage)
		g.Get("/admin/plan/temparte-warehouses/{id}", h.AdminTempWarehousesPage)
		g.Get("/admin/user-plan/temparte-warehouses", h.AdminTempWarehousesPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.warehouse.update"))
		g.Post("/admin/temporary-warehouses/upload", h.AdminTempWarehouseUploadSubmit)
		g.Post("/admin/user/temparte-warehouses/upload", h.AdminTempWarehouseUploadSubmit)
		g.Post("/admin/temporary-warehouses/{id}/mapping", h.AdminTempWarehouseMappingSubmit)
		g.Post("/admin/temporary-warehouses/{id}/toggle-archive", h.AdminTempWarehouseToggleArchiveSubmit)
		g.Post("/admin/temporary-warehouses/bulk", h.AdminTempWarehouseBulkSubmit)
		g.Post("/admin/user/temparte-warehouses/bulk", h.AdminTempWarehouseBulkSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.warehouse.delete"))
		g.Post("/admin/temporary-warehouses/items/{id}/delete", h.AdminTempWarehouseItemDeleteSubmit)
		g.Post("/admin/temporary-warehouses/{id}/delete", h.AdminTempWarehouseDeleteSubmit)
	})

	// "مستودعاتي المرفوعة" — same screen, scoped to the signed-in user's own
	// uploads. Its own permissions so a moderator can be given just this.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.my_temp_warehouse.view"))
		g.Get("/admin/my/temparte-warehouses", h.AdminMyTempWarehousesPage)
		g.Get("/admin/my/temparte-warehouses/{id}/items-json", h.AdminMyTempWarehouseItemsJSON)
		g.Get("/admin/my/temparte-warehouses/{id}/mapping-json", h.AdminMyTempWarehouseMappingJSON)
		g.Get("/admin/my/temparte-warehouses/{id}/export", h.AdminMyTempWarehouseExportXLSX)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.my_temp_warehouse.manage"))
		g.Post("/admin/my/temparte-warehouses/upload", h.AdminTempWarehouseUploadSubmit)
		g.Post("/admin/my/temparte-warehouses/{id}/mapping", h.AdminMyTempWarehouseMappingSubmit)
		g.Post("/admin/my/temparte-warehouses/{id}/toggle-archive", h.AdminMyTempWarehouseToggleArchiveSubmit)
		g.Post("/admin/my/temparte-warehouses/{id}/delete", h.AdminMyTempWarehouseDeleteSubmit)
		g.Post("/admin/my/temparte-warehouses/items/{id}/delete", h.AdminMyTempWarehouseItemDeleteSubmit)
		g.Post("/admin/my/temparte-warehouses/bulk", h.AdminMyTempWarehouseBulkSubmit)
	})

	// "مستودعات المشرفين تحت إدارتي" — a main moderator's view of the
	// moderators assigned under them.
	//
	// The permission gate answers "may this person manage a team's
	// warehouses?". It does NOT answer "whose team?" — that is decided per
	// request against identity.users.moderator_parent_id, inside every handler
	// below, because a route gate cannot know which files belong to which
	// hierarchy. See admin_team_temp_warehouse_handlers.go.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.team_temp_warehouse.view"))
		g.Get("/admin/team/temparte-warehouses", h.AdminTeamTempWarehousesPage)
		g.Get("/admin/team/temparte-warehouses/{id}/items-json", h.AdminTeamTempWarehouseItemsJSON)
		g.Get("/admin/team/temparte-warehouses/{id}/mapping-json", h.AdminTeamTempWarehouseMappingJSON)
		g.Get("/admin/team/temparte-warehouses/{id}/export", h.AdminTeamTempWarehouseExportXLSX)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("inventory.team_temp_warehouse.manage"))
		g.Post("/admin/team/temparte-warehouses/{id}/mapping", h.AdminTeamTempWarehouseMappingSubmit)
		g.Post("/admin/team/temparte-warehouses/{id}/toggle-archive", h.AdminTeamTempWarehouseToggleArchiveSubmit)
		g.Post("/admin/team/temparte-warehouses/items/{id}/delete", h.AdminTeamTempWarehouseItemDeleteSubmit)
	})
}

func (h *UIHandler) registerAdminCatalogMutations(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.create"))
		g.Post("/admin/products/new", h.AdminProductCreateSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.update"))
		g.Post("/admin/products/{id}/edit", h.AdminProductEditSubmit)
		g.Post("/admin/products/{id}/status", h.AdminProductStatusSubmit)
		g.Post("/admin/product-child/{id}/status", h.AdminProductChildStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.product.delete"))
		g.Post("/admin/products/delete-all", h.AdminProductsDeleteAllSubmit)
		g.Post("/admin/products/{id}/delete", h.AdminProductDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.brand.update"))
		g.Post("/admin/brands/new", h.AdminBrandCreateSubmit)
		g.Post("/admin/brands/{id}/edit", h.AdminBrandEditSubmit)
		g.Post("/admin/brands/{id}/status", h.AdminBrandStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.brand.delete"))
		g.Post("/admin/brands/{id}/delete", h.AdminBrandDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.category.update"))
		g.Post("/admin/categories/new", h.AdminCategoryCreateSubmit)
		g.Post("/admin/categories/{id}/edit", h.AdminCategoryEditSubmit)
		g.Post("/admin/categories/{id}/toggle", h.AdminCategoryToggleSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.category.delete"))
		g.Post("/admin/categories/{id}/delete", h.AdminCategoryDeleteSubmit)
	})
}
