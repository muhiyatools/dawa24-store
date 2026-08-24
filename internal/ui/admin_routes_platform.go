package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminPlatformRoutes(r chi.Router) {
	// Settings
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.setting.view", h.log))
		g.Get("/admin/settings", h.AdminSettingsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.setting.update", h.log))
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

	// Content & Policies & Cities & Services & Reference Data
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.content.view", h.log))
		g.Get("/admin/content", h.AdminContentPage)
		// The editor lives in the settings Policies tab now (PLAN_V7 Task 2.3).
		g.Get("/admin/policies", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/settings?tab=policies", http.StatusMovedPermanently)
		})
		g.Get("/admin/finder", h.AdminFinderPage)
		g.Get("/admin/cities", h.AdminCitiesPage)
		g.Get("/admin/categories", h.AdminCategoriesPage)
		g.Get("/admin/brands", h.AdminBrandsPage)
		g.Get("/admin/social-media", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/settings?tab=site", http.StatusMovedPermanently)
		})
		g.Get("/admin/highlight-sections", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/content?tab=sections", http.StatusMovedPermanently)
		})
		g.Get("/admin/api-integrations", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.content.update", h.log))
		g.Post("/admin/content", h.AdminContentSubmit)
		g.Post("/admin/content/{id}/toggle", h.AdminContentToggleSubmit)
		g.Post("/admin/content/{id}/delete", h.AdminContentDeleteSubmit)
		g.Post("/admin/policies", h.AdminPolicyCreateSubmit)
		g.Post("/admin/policies/{id}/publish", h.AdminPolicyPublishSubmit)
		g.Post("/admin/finder/question", h.AdminFinderQuestionSubmit)
		g.Post("/admin/finder/result", h.AdminFinderResultSubmit)
		g.Post("/admin/finder/option", h.AdminFinderOptionSubmit)
		g.Post("/admin/cities/new", h.AdminCityCreateSubmit)
		g.Post("/admin/cities/{id}/toggle", h.AdminCityToggleSubmit)
		g.Post("/admin/messages/{id}/toggle", h.AdminMessageToggleSubmit)
		g.Post("/admin/messages/{id}/delete", h.AdminMessageDeleteSubmit)
	})

	// Audit & Logs & Chat History & AskFor & Notifications
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.activity_log.view", h.log))
		g.Get("/admin/audit", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=audit", http.StatusMovedPermanently)
		})
		g.Get("/admin/analytics", h.AdminAnalyticsPage)
		g.Get("/admin/messages", h.AdminMessagesPage)
		g.Get("/admin/documents", h.AdminDocumentsPage)
		g.Get("/admin/jobs", h.AdminJobsPage)
		g.Get("/admin/chat/tree", h.AdminChatTreePage)
		g.Get("/admin/chat/history", h.AdminChatHistoryPage)
		g.Get("/admin/chat/ai/{id}", h.AdminAIChatDetailPage)
		g.Get("/admin/chat/history/{id}", h.AdminChatHistoryDetailPage)
		g.Get("/admin/ask-for", h.AdminAskForPage)
		g.Get("/admin/ask-for/{id}", h.AdminAskForDetailPage)
		g.Post("/admin/ask-for/{id}/respond", h.AdminAskForRespondSubmit)
		g.Get("/admin/full-error-logs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=errors", http.StatusMovedPermanently)
		})
		g.Get("/admin/full-error-logs/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=errors", http.StatusMovedPermanently)
		})
		g.Post("/admin/full-error-logs/{id}/status", h.AdminErrorLogTransitionSubmit)
		g.Get("/admin/full-activity-logs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=audit", http.StatusMovedPermanently)
		})
		g.Get("/admin/full-activity-logs/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers?tab=audit", http.StatusMovedPermanently)
		})
		g.Get("/admin/notifications", h.AdminFullNotificationsPage)
		g.Get("/admin/full/admin-notification", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/notifications", http.StatusMovedPermanently)
		})
		g.Get("/admin/full/admin-notification/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/notifications", http.StatusMovedPermanently)
		})
		g.Get("/admin/system-page", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers", http.StatusMovedPermanently)
		})
		g.Get("/admin/system-page/{system}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/developers", http.StatusMovedPermanently)
		})
		g.Get("/admin/first-look", h.AdminFirstLookPage)
		// Deletes-lists and trash-list were the same screen twice over the same
		// (previously fabricated) model list. One survives (PLAN_V7 Task 2.5).
		g.Get("/admin/deletes-lists", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/trash-list", http.StatusMovedPermanently)
		})
		g.Get("/admin/deletes-lists/{model}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/trash-list/"+chi.URLParam(r, "model"), http.StatusMovedPermanently)
		})
		g.Get("/admin/deletes-lists/{model}/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/trash-list/"+chi.URLParam(r, "model"), http.StatusMovedPermanently)
		})
		g.Get("/admin/trash-list", h.AdminTrashListPage)
		g.Get("/admin/trash-list/{model}", h.AdminTrashListModelPage)
		g.Post("/admin/trash-list/{model}/{id}/restore", h.AdminTrashRestoreSubmit)
		g.Post("/admin/trash-list/{model}/{id}/purge", h.AdminTrashPurgeSubmit)
		g.Get("/admin/session-plan", h.AdminSessionPlansPage)
		g.Get("/admin/session-plan/requests", h.AdminSessionPlanRequestsPage)
		g.Get("/admin/report-issues", h.AdminReportIssuesPage)
	})

	// Developers Section (SQL Console, AI Gateway, Errors)
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.developer.sql", h.log))
		g.Get("/admin/developers", h.AdminDevelopersPage)
		g.Post("/admin/developers/sql", h.AdminSQLExecuteSubmit)
		g.Post("/admin/developers/ai", h.AdminDeveloperAISettingsSubmit)
		g.Post("/admin/developers/ai/fetch-models", h.AdminAIFetchModelsAPI)
		g.Post("/admin/developers/ai/test", h.AdminGatewayTestConnection)
		g.Post("/admin/developers/errors/{id}/status", h.AdminErrorLogStatusSubmit)
	})
}
