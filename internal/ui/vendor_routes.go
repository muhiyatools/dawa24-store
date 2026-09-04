package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// The supplier dashboard, /vendor/*.
//
// Until now this whole surface was a flat list of route registrations behind
// RequireVendor and RequireApproved — "is this caller a member of an approved
// vendor company?" and nothing else. Any member of the company could open the
// wallet, withdraw from it, delete a branch, remove another employee, publish
// a storefront or read the activity log, because membership was the only
// question anyone asked.
//
// Every group below now names the permission from the vendor catalogue that
// reveals its sidebar item. The company's owner grants those permissions to
// the company's own roles, which nobody outside the company can see or select.
func (h *UIHandler) registerVendorRoutes(r chi.Router) {
	h.registerVendorCompanyRoutes(r)
	h.registerVendorTeamRoutes(r)
	h.registerVendorCatalogRoutes(r)
	h.registerVendorIngestRoutes(r)
	h.registerVendorPromoRoutes(r)
	h.registerVendorCommerceRoutes(r)
	h.registerVendorContentRoutes(r)
}

func (h *UIHandler) registerVendorCompanyRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.dashboard.view"))
		g.Get("/vendor/dashboard", h.VendorDashboardPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.organization.view"))
		g.Get("/vendor/organization", h.OrganizationProfilePage)
		g.Get("/vendor/settings/organization", h.OrganizationProfilePage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.organization.update"))
		g.Post("/vendor/organization/{section}", h.OrganizationProfileSectionSubmit)
		g.Post("/vendor/organization/requests/{id}/withdraw", h.OrganizationProfileWithdrawSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.user_org.view"))
		g.Get("/vendor/user-organization", h.VendorUserOrganizationsPage)
		g.Get("/vendor/api/users/search", h.VendorUserSearchJSON)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.user_org.manage"))
		g.Post("/vendor/user-organization/new", h.VendorUserOrganizationCreateSubmit)
		g.Post("/vendor/user-organization/{id}/approve", h.VendorUserOrganizationApproveSubmit)
		g.Post("/vendor/user-organization/{id}/reject", h.VendorUserOrganizationRejectSubmit)
		g.Post("/vendor/user-organization/{id}/edit", h.VendorUserOrganizationUpdateSubmit)
		g.Post("/vendor/user-organization/{id}/delete", h.VendorUserOrganizationDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.branch.view"))
		g.Get("/vendor/branches", h.VendorBranchesPage)
		g.Get("/vendor/branches/{id}/edit", h.VendorBranchEditPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.branch.create"))
		g.Post("/vendor/branches/new", h.VendorBranchNewSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.branch.update"))
		g.Post("/vendor/branches/{id}/edit", h.VendorBranchEditSubmit)
		g.Post("/vendor/branches/{id}/manager", h.SettingsBranchManagerAssignSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.branch.delete"))
		g.Post("/vendor/branches/{id}/delete", h.VendorBranchDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.coverage.view"))
		g.Get("/vendor/coverage", h.VendorCoveragePage)
		g.Get("/vendor/coverage/governorates/{id}/cities", h.APIGovernorateCitiesJSON)
		g.Get("/api/geo/governorates/{id}/cities", h.APIGovernorateCitiesJSON)
		g.Get("/vendor/coverage/branch/{branchID}", h.VendorBranchCoveragePage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.coverage.manage"))
		g.Post("/vendor/coverage", h.VendorCoverageCreateSubmit)
		g.Post("/vendor/coverage/{id}", h.VendorCoverageUpdateSubmit)
		g.Post("/vendor/coverage/{id}/update", h.VendorCoverageUpdateSubmit)
		g.Post("/vendor/coverage/{id}/edit", h.VendorCoverageUpdateSubmit)
		g.Post("/vendor/coverage/{id}/delete", h.VendorCoverageDeleteSubmit)
		g.Post("/vendor/coverage/{id}/toggle", h.VendorCoverageToggleSubmit)
		g.Post("/vendor/delivery-bands", h.VendorDeliveryBandCreateSubmit)
		g.Post("/vendor/delivery-bands/create", h.VendorDeliveryBandCreateSubmit)
		g.Post("/vendor/delivery-bands/{id}/delete", h.VendorDeliveryBandDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.pharmacy_coverage.view"))
		g.Get("/vendor/pharmacy-coverage", h.VendorPharmacyCoveragePage)
		g.Get("/vendor/pharmacy-coverage/{id}", h.VendorPharmacyCoverageDetailPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.subscription.view"))
		g.Get("/vendor/subscription", h.TenantSubscriptionPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.subscription.manage"))
		g.Post("/vendor/subscription/checkout", h.TenantSubscriptionCheckoutSubmit)
	})

	r.Get("/vendor/session", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/vendor/sessions", http.StatusMovedPermanently)
	})
	r.Get("/vendor/notifications", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/notifications", http.StatusMovedPermanently)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.session.view", "vendor.dashboard.view"))
		g.Get("/vendor/sessions", h.TenantSessionsPage)
		g.Get("/vendor/mfa", h.VendorMFAPage)
		g.Post("/vendor/mfa/setup", h.VendorMFASetupSubmit)
		g.Post("/vendor/mfa/confirm", h.VendorMFAConfirmSubmit)
		g.Post("/vendor/mfa/disable", h.VendorMFADisableSubmit)
		g.Post("/vendor/mfa/regenerate-codes", h.VendorMFARegenerateRecoveryCodesSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.session.revoke"))
		g.Post("/vendor/sessions/revoke", h.TenantSessionRevokeSubmit)
		g.Post("/vendor/sessions/revoke-all", h.TenantSessionRevokeAllSubmit)
	})
	// Changing your own password needs no permission: it is an account action,
	// not a company one, and a member locked out of every page must still be
	// able to secure their own credentials.
	r.Post("/vendor/password", h.TenantPasswordChangeSubmit)
}

func (h *UIHandler) registerVendorTeamRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.team.view"))
		g.Get("/vendor/team", h.VendorTeamPage)
		g.Get("/vendor/team/import", h.VendorTeamImportPage)
		g.Get("/vendor/team/import/sample.xlsx", h.VendorTeamImportSampleDownload)
		g.Get("/vendor/team/import/{id}", h.VendorTeamImportSessionPage)
		g.Get("/vendor/team/fast-add", h.VendorTeamFastAddPage)
		g.Get("/vendor/team/{id}", h.VendorTeamUserDetailPage)
		g.Get("/vendor/team/{id}/info", h.VendorTeamUserInfoPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.team.create"))
		g.Post("/vendor/team/new", h.VendorTeamNewSubmit)
		g.Post("/vendor/team/import/upload", h.VendorTeamImportUploadSubmit)
		g.Post("/vendor/team/import/{id}/map", h.VendorTeamImportMapSubmit)
		g.Post("/vendor/team/import/{id}/commit", h.VendorTeamImportCommitSubmit)
		g.Post("/vendor/team/import/{id}/cancel", h.VendorTeamImportCancelSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.team.update"))
		g.Post("/vendor/team/{id}/toggle", h.VendorTeamToggleSubmit)
		g.Post("/vendor/team/{id}/edit", h.VendorTeamEditSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.team.delete"))
		g.Post("/vendor/team/{id}/delete", h.VendorTeamDeleteSubmit)
	})

	// Roles. Whoever may assign a role can hand themselves everything that
	// role holds, so assignment is granted separately from editing, and both
	// are separate from merely reading the list.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.role.view"))
		g.Get("/vendor/roles", h.VendorRolesPage)
		g.Get("/vendor/roles/{id}", h.VendorRoleDetailPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.role.create"))
		g.Post("/vendor/roles", h.VendorRoleCreateSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.role.update"))
		g.Post("/vendor/roles/{id}", h.VendorRoleUpdateSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.role.delete"))
		g.Post("/vendor/roles/{id}/delete", h.VendorRoleDeleteSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.role.assign"))
		g.Post("/vendor/team/{id}/role", h.VendorMemberRoleAssignSubmit)
	})
}

func (h *UIHandler) registerVendorPromoRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.offer.view"))
		g.Get("/vendor/offers", h.VendorOffersPage)
		g.Get("/vendor/offers/new", h.VendorOfferNewPage)
		g.Get("/vendor/offers/{id}/edit", h.VendorOfferEditPage)
		g.Get("/vendor/offers/{id}/locations", h.VendorOfferLocationsPage)
		g.Get("/vendor/offers/locations", h.VendorOffersLocationsPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.offer.manage"))
		g.Post("/vendor/offers/new", h.VendorOfferNewSubmit)
		g.Post("/vendor/offers/{id}/edit", h.VendorOfferEditSubmit)
		g.Post("/vendor/offers/{id}/locations/new", h.VendorOfferLocationNewSubmit)
		g.Post("/vendor/offers/{id}/delete", h.VendorOfferDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.offer_package.view"))
		g.Get("/vendor/offers-packages", h.VendorOffersPackagesPage)
		g.Get("/vendor/offers-packages/{id}", h.VendorOffersPackagesPage)
		g.Get("/vendor/offers-packages/sponsorships", h.VendorOffersPackagesSponsorshipsPage)
		g.Get("/vendor/offers-packages/sponsorships/{id}", h.VendorOffersPackagesSponsorshipsPage)
		g.Get("/vendor/offers-packages/promotions", h.VendorOffersPackagesPromotionsPage)
		// كشف حساب للباقة: where this purchase's credits went. The card showed
		// "31 / 50" and nothing about the missing nineteen.
		g.Get("/vendor/offers-packages/purchases/{id}/statement", h.CreditStatementPage)
		g.Get("/vendor/sponsorship-requests", h.VendorSponsorshipRequestsPage)
		g.Post("/vendor/sponsorship-requests/new", h.VendorSponsorshipRequestSubmit)
		g.Post("/vendor/sponsorship-requests/{id}/cancel", h.VendorSponsorshipRequestCancelSubmit)
		g.Post("/vendor/sponsorship-packages/{id}/purchase", h.VendorSponsorshipPackagePurchaseSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.ad.view"))
		g.Get("/vendor/ads", h.VendorAdsPage)
		g.Get("/vendor/ads/add", h.VendorAdsPage)
		g.Get("/vendor/ads/{id}", h.VendorAdsPage)
		g.Get("/vendor/ads/{id}/edit", h.VendorAdsPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.ad.manage"))
		g.Post("/vendor/ads/new", h.VendorAdCreateSubmit)
		g.Post("/vendor/ads/{id}/edit", h.VendorAdUpdateSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.storefront.view"))
		g.Get("/vendor/storefront", h.VendorStorefrontPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.storefront.manage"))
		g.Post("/vendor/storefront/section", h.VendorStorefrontSectionSubmit)
		g.Post("/vendor/storefront/section/{id}/update", h.VendorStorefrontSectionUpdateSubmit)
		g.Post("/vendor/storefront/section/{id}/delete", h.VendorStorefrontSectionDeleteSubmit)
		g.Post("/vendor/storefront/section/{id}/toggle", h.VendorStorefrontSectionToggleSubmit)
	})
}

func (h *UIHandler) registerVendorCommerceRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.order.view"))
		g.Get("/vendor/orders", h.VendorOrdersPage)
		g.Get("/vendor/orders/offers", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/vendor/orders", http.StatusMovedPermanently)
		})
		g.Get("/vendor/orders/offers/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/vendor/orders", http.StatusMovedPermanently)
		})
		// Legacy purchase requests on vendor side redirect to orders
		g.Get("/vendor/purchase-requests", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/vendor/orders", http.StatusMovedPermanently)
		})
		g.Get("/vendor/purchase-requests/*", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/vendor/orders", http.StatusMovedPermanently)
		})
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.order.update"))
		g.Post("/vendor/orders/{id}/status", h.VendorOrderStatusSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.order.negotiate"))
		g.Post("/vendor/orders/{id}/negotiation/accept", h.VendorNegotiationAcceptSubmit)
		g.Post("/vendor/orders/{id}/negotiation/reject", h.VendorNegotiationRejectSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.payment.view"))
		g.Get("/vendor/payments", h.VendorPaymentsPage)
		g.Post("/vendor/payments/record", h.VendorRecordPaymentSubmit)
		g.Get("/vendor/invoices", h.InvoicesPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.earnings.view"))
		g.Get("/vendor/earnings/order", h.VendorEarningsOrderPage)
		g.Get("/vendor/earnings/offers", h.VendorEarningsOffersPage)
	})

	// The wallet moves the company's money. Reading the balance and moving it
	// are separate grants, and neither is implied by ordinary membership.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.wallet.view"))
		g.Get("/vendor/wallet", h.TenantWalletPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.wallet.manage"))
		g.Post("/vendor/wallet/deposit", h.TenantWalletDepositSubmit)
		g.Post("/vendor/wallet/withdraw", h.TenantWalletWithdrawSubmit)
		g.Post("/vendor/wallet/payment-methods", h.TenantPaymentMethodAddSubmit)
		// Editing and re-defaulting a saved method lived only under
		// /settings/payment-methods, which is gated on approval and not on the
		// wallet permission at all — so a member without vendor.wallet.manage
		// could add and change payment details there, and the wallet screen
		// they were actually on could not offer an edit form. Both now sit
		// beside the add, behind the same grant.
		g.Post("/vendor/wallet/payment-methods/{id}/edit", h.SettingsPaymentMethodEditSubmit)
		g.Post("/vendor/wallet/payment-methods/{id}/default", h.SettingsPaymentMethodSetDefaultSubmit)
		g.Post("/vendor/wallet/payment-methods/{id}/delete", h.TenantPaymentMethodDeleteSubmit)
	})
}

func (h *UIHandler) registerVendorContentRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.document.view"))
		g.Get("/vendor/documents", h.OrganizationDocumentsPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.document.manage"))
		g.Post("/documents/upload", h.OrganizationDocumentsUploadSubmit)
		g.Post("/documents/delete", h.OrganizationDocumentDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.policy.view"))
		g.Get("/vendor/policies", h.VendorPoliciesPage)
		g.Get("/vendor/social-media", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/vendor/policies", http.StatusMovedPermanently)
		})
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.policy.update"))
		g.Post("/vendor/policies", h.VendorPoliciesSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.activity.view", "vendor.team.view", "vendor.dashboard.view"))
		g.Get("/vendor/activities", h.VendorActivitiesPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.ai_log.view"))
		g.Get("/vendor/ai-logs", h.AIConsumptionLogsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.review.view"))
		g.Get("/vendor/reviews", h.VendorReviewsPage)
		g.Post("/vendor/reviews/{id}/reply", h.VendorReviewReplySubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.job.view"))
		g.Get("/vendor/jobs", h.VendorJobsPage)
		g.Get("/vendor/jobs/{id}", h.JobDetailPage)
		g.Get("/vendor/jobs/{id}/applications", h.VendorJobApplicationsJSON)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireTenantPagePermission("vendor.job.manage"))
		g.Post("/vendor/jobs", h.VendorJobCreateSubmit)
		g.Post("/vendor/jobs/{id}/edit", h.VendorJobUpdateSubmit)
		g.Post("/vendor/jobs/{id}/update", h.VendorJobUpdateSubmit)
		g.Post("/vendor/jobs/{id}/toggle", h.VendorJobToggleSubmit)
		g.Post("/vendor/jobs/{id}/delete", h.VendorJobDeleteSubmit)
		g.Post("/vendor/jobs/{id}/applications/{appId}/accept", h.VendorJobApplicationAcceptSubmit)
		g.Post("/vendor/jobs/{id}/applications/{appId}/reject", h.VendorJobApplicationRejectSubmit)
	})
}
