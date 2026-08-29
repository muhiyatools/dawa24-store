package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// The platform-administration surface.
//
// Every group below is gated on the permission that also reveals the matching
// sidebar item in internal/platform/rbac/nav_admin.go. That correspondence is
// the point: a link the caller can see leads to a page they can open, and a
// page they cannot open has no link.
//
// It was not so before. One group gated on "platform.activity_log.view" held
// analytics, contact messages, job listings, chat history, notifications, the
// trash and the first-look report — seven unrelated screens behind one key, so
// granting a moderator the audit log handed them the trash can as well.
func (h *UIHandler) registerAdminPlatformRoutes(r chi.Router) {
	h.registerAdminSettingsRoutes(r)
	h.registerAdminContentRoutes(r)
	h.registerAdminToolRoutes(r)
	h.registerAdminDiagnosticRoutes(r)
}

func (h *UIHandler) registerAdminSettingsRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.setting.view"))
		g.Get("/admin/settings", h.AdminSettingsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.setting.update"))
		g.Post("/admin/settings", h.AdminSettingsSubmit)
		g.Post("/admin/settings/site", h.AdminSiteSettingsSubmit)
		g.Post("/admin/settings/branding", h.AdminBrandingSubmit)
		g.Post("/admin/settings/ai", h.AdminAISettingsSubmit)
		g.Post("/admin/settings/gateway", h.AdminGatewaySettingsSubmit)
		g.Post("/admin/settings/gateway/test", h.AdminGatewayTestConnection)
		g.Post("/admin/settings/features/toggle", h.AdminFeatureToggleSubmit)
		g.Post("/admin/settings/payment-methods", h.AdminPlatformPaymentMethodSubmit)
		g.Post("/admin/settings/payment-methods/toggle", h.AdminPlatformPaymentMethodToggleSubmit)
		g.Post("/admin/settings/payment-methods/{id}/delete", h.AdminPlatformPaymentMethodDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.translation.view"))
		g.Get("/admin/translations", h.AdminTranslationsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.translation.update"))
		g.Post("/admin/translations", h.AdminTranslationUpdateSubmit)
		g.Post("/admin/translations/reset", h.AdminTranslationResetSubmit)
		g.Post("/admin/translations/sync", h.AdminTranslationsSyncSubmit)
	})
}

func (h *UIHandler) registerAdminContentRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.content.view"))
		g.Get("/admin/content", h.AdminContentPage)
		// The editor lives in the settings Policies tab now (PLAN_V7 Task 2.3).
		g.Get("/admin/policies", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/settings?tab=policies", http.StatusMovedPermanently)
		})
		g.Get("/admin/social-media", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/settings?tab=site", http.StatusMovedPermanently)
		})
		g.Get("/admin/highlight-sections", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/content?tab=sections", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.content.update"))
		g.Post("/admin/content", h.AdminContentSubmit)
		g.Post("/admin/content/{id}/toggle", h.AdminContentToggleSubmit)
		g.Post("/admin/content/{id}/delete", h.AdminContentDeleteSubmit)
		g.Post("/admin/policies", h.AdminPolicyCreateSubmit)
		g.Post("/admin/policies/{id}/publish", h.AdminPolicyPublishSubmit)
	})

	// Geography is its own concern: a content editor has no reason to redraw
	// the delivery map, and a logistics moderator has no reason to rewrite the
	// About page.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.geo.view"))
		g.Get("/admin/cities", h.AdminCitiesPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.geo.update"))
		g.Post("/admin/cities/new", h.AdminCityCreateSubmit)
		g.Post("/admin/cities/{id}/edit", h.AdminCityEditSubmit)
		g.Post("/admin/cities/{id}/toggle", h.AdminCityToggleSubmit)
		g.Post("/admin/governorates/new", h.AdminGovernorateCreateSubmit)
		g.Post("/admin/governorates/{id}/edit", h.AdminGovernorateEditSubmit)
		g.Post("/admin/governorates/{id}/toggle", h.AdminGovernorateToggleSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.category.view"))
		g.Get("/admin/categories", h.AdminCategoriesPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.brand.view"))
		g.Get("/admin/brands", h.AdminBrandsPage)
	})
}

func (h *UIHandler) registerAdminToolRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.analytics.view"))
		g.Get("/admin/analytics", h.AdminAnalyticsPage)
		g.Get("/admin/first-look", h.AdminFirstLookPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.message.view"))
		g.Get("/admin/messages", h.AdminMessagesPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.message.update"))
		g.Post("/admin/messages/{id}/toggle", h.AdminMessageToggleSubmit)
		g.Post("/admin/messages/{id}/delete", h.AdminMessageDeleteSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("hr.job.view"))
		g.Get("/admin/jobs", h.AdminJobsPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("hr.document.view"))
		g.Get("/admin/documents", h.AdminDocumentsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.chat.view"))
		g.Get("/admin/chat/history", h.AdminChatHistoryPage)
		g.Get("/admin/chat/ai/{id}", h.AdminAIChatDetailPage)
		g.Get("/admin/chat/history/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			http.Redirect(w, r, "/admin/chat/ai/"+id, http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("workflow.request.view", "workflow.issue.view"))
		g.Get("/admin/requests", h.AdminAskForPage)
		g.Get("/admin/ask-for", h.AdminAskForPage)
		g.Get("/admin/ask-for/{id}", h.AdminAskForDetailPage)
		g.Get("/admin/report-issues", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/requests", http.StatusMovedPermanently)
		})
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("workflow.request.update"))
		g.Post("/admin/ask-for/{id}/respond", h.AdminAskForRespondSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("notifications.center.view"))
		g.Get("/admin/notifications", h.AdminFullNotificationsPage)
		g.Get("/admin/full/admin-notification", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/notifications", http.StatusMovedPermanently)
		})
		g.Get("/admin/full/admin-notification/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/notifications", http.StatusMovedPermanently)
		})
	})

	// Trash is destructive and separately granted: restoring a record is not
	// the same right as reading an audit log, and purging is not the same
	// right as restoring.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.trash.view"))
		g.Get("/admin/trash-list", h.AdminTrashListPage)
		g.Get("/admin/trash-list/{model}", h.AdminTrashListModelPage)
		g.Get("/admin/deletes-lists", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/trash-list", http.StatusMovedPermanently)
		})
		g.Get("/admin/deletes-lists/{model}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/trash-list/"+chi.URLParam(r, "model"), http.StatusMovedPermanently)
		})
		g.Get("/admin/deletes-lists/{model}/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/trash-list/"+chi.URLParam(r, "model"), http.StatusMovedPermanently)
		})
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.trash.update"))
		g.Post("/admin/trash-list/{model}/{id}/restore", h.AdminTrashRestoreSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.trash.purge"))
		g.Post("/admin/trash-list/{model}/{id}/purge", h.AdminTrashPurgeSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.subscription_plan.view"))
		g.Get("/admin/session-plan", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/plans", http.StatusMovedPermanently)
		})
		g.Get("/admin/session-plan/requests", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/plans?tab=subscriptions", http.StatusMovedPermanently)
		})
	})
}

func (h *UIHandler) registerAdminDiagnosticRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.activity_log.view"))
		g.Get("/admin/audit", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=audit", http.StatusMovedPermanently)
		})
		g.Get("/admin/full-activity-logs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=audit", http.StatusMovedPermanently)
		})
		g.Get("/admin/full-activity-logs/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=audit", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.error_log.view"))
		g.Get("/admin/full-error-logs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=errors", http.StatusMovedPermanently)
		})
		g.Get("/admin/full-error-logs/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=errors", http.StatusMovedPermanently)
		})
		g.Get("/admin/system-page", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers", http.StatusMovedPermanently)
		})
		g.Get("/admin/system-page/{system}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers", http.StatusMovedPermanently)
		})
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.error_log.update"))
		g.Post("/admin/full-error-logs/{id}/status", h.AdminErrorLogTransitionSubmit)
	})

	// The SQL console reads and writes arbitrary tables. It is the one page
	// where the gate is the whole of the security model.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.developer.sql"))
		g.Get("/admin/developers", h.AdminDevelopersPage)
		g.Get("/admin/api-integrations", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers", http.StatusMovedPermanently)
		})
		g.Post("/admin/developers/sql", h.AdminSQLExecuteSubmit)
		g.Post("/admin/developers/ai", h.AdminDeveloperAISettingsSubmit)
		g.Post("/admin/developers/ai/fetch-models", h.AdminAIFetchModelsAPI)
		g.Post("/admin/developers/ai/test", h.AdminGatewayTestConnection)
		g.Post("/admin/developers/errors/{id}/status", h.AdminErrorLogStatusSubmit)
	})
}
