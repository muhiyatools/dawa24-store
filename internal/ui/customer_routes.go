package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// The pharmacy dashboard, /customer/*.
//
// Same story as the vendor side: membership of an approved pharmacy was the
// only question asked, so any employee could place an order against the
// company's wallet, delete a branch, or remove a colleague. A pharmacy owner
// had no way to hire a counter assistant who may search the catalogue but not
// spend money — which is the ordinary case, not an exotic one.
func (h *UIHandler) registerCustomerRoutes(r chi.Router) {
	h.registerCustomerBuyingRoutes(r)
	h.registerCustomerMarketRoutes(r)
	h.registerCustomerSavingRoutes(r)
	h.registerCustomerCompanyRoutes(r)
	h.registerCustomerTeamRoutes(r)
}

func (h *UIHandler) registerCustomerBuyingRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.dashboard.view"))
		g.Get("/customer/dashboard", h.PharmacyDashboardPage)
		// Legacy URLs kept as redirects so bookmarks survive the rename.
		g.Get("/pharmacy/dashboard", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/customer/dashboard", http.StatusFound)
		})
		g.Get("/customer/cpanel", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/customer/dashboard", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(h.scrape.Protect)
		g.Use(authctx.RequireTenantPagePermission("pharmacy.purchase_request.view", "pharmacy.dashboard.view"))
		g.Get("/customer/catalog", h.CustomerCatalogPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.purchase_request.view", "pharmacy.dashboard.view"))
		g.Get("/customer/purchase-request", h.CustomerPurchaseRequestWizardPage)
		g.Get("/customer/catalog/{id}", h.CustomerProductDetailPage)
		g.Get("/customer/purchase-request/products", h.CustomerPurchaseRequestProductsRedirect)
		g.Get("/customer/purchase-request/previous", h.CustomerPurchaseRequestPreviousRedirect)
		g.Get("/customer/purchase-request/supplier", h.CustomerPurchaseRequestSupplierRedirect)
		g.Get("/customer/purchase-request/supplier/{id}", h.CustomerPurchaseRequestSupplierRedirect)
		g.Get("/customer/add-order", h.CustomerAddOrderPage)
		g.Get("/customer/products/main/{id}", h.CustomerProductsMainAlias)
		g.Get("/customer/purchase-priority", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/customer/dashboard", http.StatusMovedPermanently)
		})
		g.Get("/customer/purchase-priority/*", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/customer/dashboard", http.StatusMovedPermanently)
		})
	})

	// The former Automatic Purchase Request feature is superseded by Smart
	// Ordering (specs/001-smart-ordering-system).
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.smart_order.view"))
		g.Get("/customer/automation", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/customer/smart-order", http.StatusMovedPermanently)
		})
		g.Get("/customer/automation/previous", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/customer/smart-order/history", http.StatusMovedPermanently)
		})
	})

	// The cart and the checkout. Placing an order commits the company's money,
	// so it is its own grant: a counter assistant may fill a basket for a
	// pharmacist to approve without being able to submit it themselves.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.cart.use"))
		g.Get("/cart", h.CustomerCartPage)
		g.Post("/cart/add", h.AddToCartSubmit)
		g.Post("/cart/add-offer", h.AddOfferToCartSubmit)
		g.Post("/cart/remove", h.RemoveFromCartSubmit)
		g.Post("/cart/update-quantity", h.UpdateCartQuantitySubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.order.create"))
		g.Get("/checkout", h.CustomerCheckoutPage)
		g.Get("/offers/{id}/checkout", h.CustomerOfferCheckoutPage)
		g.Post("/checkout", h.CheckoutSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.order.view"))
		g.Get("/orders", h.CustomerOrdersPage)
		g.Get("/orders/{id}", h.CustomerOrderDetailPage)
		g.Get("/customer/orders", h.CustomerOrdersPage)
		g.Get("/customer/orders/{id}", h.CustomerOrderDetailPage)
		g.Get("/orders/offers", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/orders", http.StatusMovedPermanently)
		})
		g.Get("/orders/offers/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			http.Redirect(w, r, "/orders/"+id, http.StatusMovedPermanently)
		})
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.order.update"))
		g.Post("/orders/{id}/edit", h.CustomerOrderEditSubmit)
		g.Post("/customer/orders/{id}/edit", h.CustomerOrderEditSubmit)
		g.Post("/customer/negotiate-order", h.CustomerNegotiateOrderSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.wallet.view"))
		g.Get("/customer/wallet", h.TenantWalletPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.wallet.manage"))
		g.Post("/customer/wallet/deposit", h.TenantWalletDepositSubmit)
		g.Post("/customer/wallet/withdraw", h.TenantWalletWithdrawSubmit)
		g.Post("/customer/wallet/payment-methods", h.TenantPaymentMethodAddSubmit)
		g.Post("/customer/wallet/payment-methods/{id}/delete", h.TenantPaymentMethodDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.favorite.view", "pharmacy.favorite.manage", "pharmacy.dashboard.view"))
		g.Get("/favorites", h.FavoritesPage)
		g.Get("/customer/favorites", h.FavoritesPage)
		g.Post("/favorites/{id}/remove", h.FavoriteRemoveSubmit)
		g.Post("/favorites/{id}/add", h.FavoriteAddSubmit)
		g.Post("/favorites/{id}/toggle", h.FavoriteToggleSubmit)
		g.Post("/favorites/toggle", h.FavoriteToggleSubmit)
	})

	// Choosing which of the pharmacy's own branches you are buying for is not
	// a privilege: every member who can order needs it, and it addresses only
	// branches the tenant already owns.
	r.Post("/customer/set-branch", h.SetBuyingBranchSubmit)
	r.Post("/customer/branches/active", h.CustomerSwitchActiveBranchSubmit)
}

func (h *UIHandler) registerCustomerMarketRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(h.scrape.Protect)
		g.Use(authctx.RequireTenantPagePermission("pharmacy.supplier.view"))
		g.Get("/customer/suppliers", h.SuppliersPage)
		g.Get("/customer/suppliers/{id}", h.SupplierProfilePage)
		g.Get("/suppliers/followed", h.FollowedSuppliersPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.offer.view"))
		g.Get("/customer/offers", h.OffersPage)
		g.Get("/customer/offers/{id}", h.OfferDetailPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.supplier.follow"))
		g.Post("/suppliers/{id}/follow", h.SupplierFollowSubmit)
		g.Post("/suppliers/{id}/message", h.SupplierMessageSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.review.write"))
		g.Post("/reviews/submit", h.ReviewSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.decision_memory.view"))
		g.Get("/customer/decision-memory", h.CustomerDecisionMemoryPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.decision_memory.delete"))
		g.Post("/customer/decision-memory/{id}/delete", h.CustomerDecisionMemoryDeleteSubmit)
		g.Post("/customer/decision-memory/clear", h.CustomerDecisionMemoryClearSubmit)
	})
}

func (h *UIHandler) registerCustomerSavingRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.saving_product.view"))
		g.Get("/customer/saving-products", h.CustomerSavingProductsPage)
		g.Get("/customer/saving-products/import", h.CustomerSavingProductsImportPage)
		g.Get("/customer/saving-products/import-page", h.CustomerSavingProductsImportPage)
		g.Get("/customer/saving-products/sample.xlsx", h.CustomerSavingProductsSampleXLSX)
		g.Get("/customer/saving-products/sample.csv", h.CustomerSavingProductsSampleCSV)
		g.Get("/customer/saving-products/import/{id}", h.CustomerSavingProductsImportSessionPage)
		g.Get("/customer/saving-products/import/session/{id}/progress", h.CustomerSavingProductsImportProgressJSON)
		g.Get("/customer/saving-products/export", h.CustomerSavingProductsExport)
		g.Get("/customer/saving-products/providers/{id}", h.CustomerSavingProductProvidersJSON)
		g.Get("/customer/saving-products/search-products", h.CustomerSavingProductSearchJSON)
		g.Get("/customer/saving-products/{id}", h.CustomerSavingProductDetailPage)
		g.Get("/customer/saveing-products", h.CustomerSavingProductsAlias)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.saving_product.manage"))
		g.Post("/customer/saving-products/new", h.CustomerSavingProductCreateSubmit)
		g.Post("/customer/saving-products/{id}/update", h.CustomerSavingProductUpdateSubmit)
		g.Post("/customer/saving-products/{id}/delete", h.CustomerSavingProductDeleteSubmit)
		g.Post("/customer/saving-products/delete-all", h.CustomerSavingProductsDeleteAllSubmit)
		g.Post("/customer/saving-products/import/upload", h.CustomerSavingProductsImportUploadSubmit)
		g.Post("/customer/saving-products/import/{id}/map", h.CustomerSavingProductsImportMapSubmit)
		g.Post("/customer/saving-products/import/{id}/items/{itemIndex}/update", h.CustomerSavingProductsImportItemUpdateSubmit)
		g.Post("/customer/saving-products/import/{id}/items/{itemIndex}/match", h.CustomerSavingProductsImportItemMatchSubmit)
		g.Post("/customer/saving-products/import/{id}/items/{itemIndex}/toggle", h.CustomerSavingProductsImportItemToggleSubmit)
		g.Post("/customer/saving-products/import/{id}/commit", h.CustomerSavingProductsImportCommitSubmit)
		g.Post("/customer/saving-products/import/{id}/cancel", h.CustomerSavingProductsImportCancelSubmit)
		g.Post("/customer/saving-products/import", h.CustomerSavingProductsImportSubmit)
		g.Post("/customer/saving-products/import/start", h.CustomerSavingProductsImportStartJSON)
		g.Post("/customer/saving-products/import/session/{id}/commit", h.CustomerSavingProductsImportCommitJSON)
		g.Post("/customer/saving-products/import/session/{id}/cancel", h.CustomerSavingProductsImportCancelJSON)
		g.Post("/customer/saving-products/preview-columns", h.CustomerSavingProductsPreviewColumnsJSON)
	})
}

func (h *UIHandler) registerCustomerCompanyRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.branch.view"))
		g.Get("/customer/branches", h.CustomerBranchesPage)
		g.Get("/customer/branches/create", h.CustomerBranchCreatePage)
		g.Get("/customer/branches/{id}/edit", h.CustomerBranchEditPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.branch.create"))
		g.Post("/customer/branches/new", h.CustomerBranchNewSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.branch.update"))
		g.Post("/customer/branches/{id}/edit", h.CustomerBranchEditSubmit)
		g.Post("/customer/branches/{id}", h.CustomerBranchEditSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.branch.delete"))
		g.Post("/customer/branches/{id}/delete", h.CustomerBranchDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.document.view"))
		g.Get("/customer/documents", h.OrganizationDocumentsPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.document.manage"))
		g.Post("/documents/upload", h.OrganizationDocumentsUploadSubmit)
		g.Post("/documents/delete", h.OrganizationDocumentDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.institutional.view"))
		g.Get("/customer/institutional-work", h.CustomerInstitutionalWorkPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.user_org.view"))
		g.Get("/customer/user-organization", h.CustomerUserOrganizationsPage)
		g.Get("/customer/api/vendors/search", h.CustomerVendorSearchJSON)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.user_org.manage"))
		g.Post("/customer/user-organization/new", h.CustomerUserOrganizationCreateSubmit)
		g.Post("/customer/user-organization/{id}/edit", h.CustomerUserOrganizationUpdateSubmit)
		g.Post("/customer/user-organization/{id}/delete", h.CustomerUserOrganizationDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.subscription.view"))
		g.Get("/customer/subscription", h.TenantSubscriptionPage)
		g.Get("/pharmacy/subscription", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/customer/subscription", http.StatusMovedPermanently)
		})
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.subscription.manage"))
		g.Post("/customer/subscription/checkout", h.TenantSubscriptionCheckoutSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.job.view"))
		g.Get("/customer/jobs", h.CustomerJobsPage)
		g.Get("/customer/jobs/{id}", h.JobDetailPage)
		g.Get("/customer/jobs/{id}/applications", h.CustomerJobApplicationsJSON)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.job.manage"))
		g.Post("/customer/jobs", h.CustomerJobCreateSubmit)
		g.Post("/customer/jobs/{id}/edit", h.CustomerJobUpdateSubmit)
		g.Post("/customer/jobs/{id}/update", h.CustomerJobUpdateSubmit)
		g.Post("/customer/jobs/{id}/toggle", h.CustomerJobToggleSubmit)
		g.Post("/customer/jobs/{id}/delete", h.CustomerJobDeleteSubmit)
		g.Post("/customer/jobs/{id}/applications/{appId}/accept", h.CustomerJobApplicationAcceptSubmit)
		g.Post("/customer/jobs/{id}/applications/{appId}/reject", h.CustomerJobApplicationRejectSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.ai_log.view"))
		g.Get("/customer/ai-logs", h.AIConsumptionLogsPage)
	})

	r.Get("/customer/session", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/customer/sessions", http.StatusMovedPermanently)
	})
	r.Get("/customer/notifications", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/notifications", http.StatusMovedPermanently)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.session.view", "pharmacy.dashboard.view"))
		g.Get("/customer/sessions", h.TenantSessionsPage)
		g.Get("/customer/mfa", h.CustomerMFAPage)
		g.Post("/customer/mfa/setup", h.CustomerMFASetupSubmit)
		g.Post("/customer/mfa/confirm", h.CustomerMFAConfirmSubmit)
		g.Post("/customer/mfa/disable", h.CustomerMFADisableSubmit)
		g.Post("/customer/mfa/regenerate-codes", h.CustomerMFARegenerateRecoveryCodesSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.session.revoke"))
		g.Post("/customer/sessions/revoke", h.TenantSessionRevokeSubmit)
		g.Post("/customer/sessions/revoke-all", h.TenantSessionRevokeAllSubmit)
	})
	r.Post("/customer/password", h.TenantPasswordChangeSubmit)
}

func (h *UIHandler) registerCustomerTeamRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.team.view"))
		g.Get("/customer/team", h.CustomerTeamPage)
		g.Get("/customer/activities", h.CustomerActivitiesPage)
		g.Get("/customer/team/import", h.CustomerTeamImportPage)
		g.Get("/customer/team/import/sample.xlsx", h.CustomerTeamImportSampleDownload)
		g.Get("/customer/team/import/{id}", h.CustomerTeamImportSessionPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.team.create"))
		g.Post("/customer/employees/new", h.CustomerEmployeeCreateSubmit)
		g.Post("/customer/team/import/upload", h.CustomerTeamImportUploadSubmit)
		g.Post("/customer/team/import/{id}/map", h.CustomerTeamImportMapSubmit)
		g.Post("/customer/team/import/{id}/commit", h.CustomerTeamImportCommitSubmit)
		g.Post("/customer/team/import/{id}/cancel", h.CustomerTeamImportCancelSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.team.update"))
		g.Post("/customer/employees/{id}/edit", h.CustomerEmployeeEditSubmit)
		g.Post("/customer/employees/{id}", h.CustomerEmployeeEditSubmit)
		g.Post("/customer/employees/{id}/status", h.CustomerEmployeeStatusSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.team.delete"))
		g.Post("/customer/employees/{id}/delete", h.CustomerEmployeeDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.role.view"))
		g.Get("/customer/roles", h.CustomerRolesPage)
		g.Get("/customer/roles/{id}", h.CustomerRoleDetailPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.role.create"))
		g.Post("/customer/roles", h.CustomerRoleCreateSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.role.update"))
		g.Post("/customer/roles/{id}", h.CustomerRoleUpdateSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.role.delete"))
		g.Post("/customer/roles/{id}/delete", h.CustomerRoleDeleteSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("pharmacy.role.assign"))
		g.Post("/customer/employees/{id}/role", h.CustomerMemberRoleAssignSubmit)
	})
}