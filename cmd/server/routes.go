package main

import (
	"log/slog"

	"github.com/go-chi/chi/v5"

	billingPostgres "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	commercePostgres "github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	hrPostgres "github.com/muhiya/dawa24-store/internal/modules/hr/postgres"
	identityPostgres "github.com/muhiya/dawa24-store/internal/modules/identity/postgres"
	ingestPostgres "github.com/muhiya/dawa24-store/internal/modules/ingest/postgres"
	inventoryPostgres "github.com/muhiya/dawa24-store/internal/modules/inventory/postgres"
	notificationsPostgres "github.com/muhiya/dawa24-store/internal/modules/notifications/postgres"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	platformadminPostgres "github.com/muhiya/dawa24-store/internal/modules/platform_admin/postgres"
	promoPostgres "github.com/muhiya/dawa24-store/internal/modules/promo/postgres"
	workflowPostgres "github.com/muhiya/dawa24-store/internal/modules/workflow/postgres"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	billingHttp "github.com/muhiya/dawa24-store/internal/modules/billing/http"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogHttp "github.com/muhiya/dawa24-store/internal/modules/catalog/http"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	commerceHttp "github.com/muhiya/dawa24-store/internal/modules/commerce/http"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	hrHttp "github.com/muhiya/dawa24-store/internal/modules/hr/http"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	ingestHttp "github.com/muhiya/dawa24-store/internal/modules/ingest/http"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	inventoryHttp "github.com/muhiya/dawa24-store/internal/modules/inventory/http"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	notificationsHttp "github.com/muhiya/dawa24-store/internal/modules/notifications/http"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgHttp "github.com/muhiya/dawa24-store/internal/modules/org/http"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	platformadminHttp "github.com/muhiya/dawa24-store/internal/modules/platform_admin/http"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	promoHttp "github.com/muhiya/dawa24-store/internal/modules/promo/http"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	workflowHttp "github.com/muhiya/dawa24-store/internal/modules/workflow/http"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// mountModuleRoutes registers domain handlers across all platform bounded contexts.
func mountModuleRoutes(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
) {
	db := deps.Handle()

	// 1. Identity & Auth
	idRepo := identityPostgres.NewRepository(db)
	// The cache handle is stable from construction and gains its client when
	// Redis connects, so the session store resolves a live client per call
	// rather than capturing nil at mount time.
	sessionStore := identity.NewSessionStore(deps.CacheHandle(), cfg.Session)
	idSvc := identity.NewService(idRepo, sessionStore, log)
	identityHttp.NewHandler(idSvc, cfg.Session, log).RegisterRoutes(r)

	// Everything from here on requires an authenticated caller.
	//
	// Until this group existed, every module route was reachable anonymously
	// and each handler took the acting user from a `?user_id=` query parameter
	// — so any caller could read any wallet, cart or notification feed, and
	// withdraw from any wallet, simply by changing a number.
	//
	// RequireAuth establishes who is calling; ResolveTenant establishes which
	// organization they are acting within, verifying membership before honouring
	// the X-Dawa-Org-ID header. Row-level security then scopes queries to a
	// tenant the caller has actually been proven to belong to.
	r.Group(func(protected chi.Router) {
		protected.Use(identityHttp.RequireAuth(idSvc, cfg.Session.CookieName, log))
		protected.Use(identityHttp.ResolveTenant(idSvc, log))

		mountAuthenticatedModules(protected, log, deps, ai)
	})

	// 13. Templ SSR Frontend & Static Assets
	catRepoUI := catalogPostgres.NewRepository(db)
	orgRepoUI := orgPostgres.NewRepository(db)
	ingRepoUI := ingestPostgres.NewRepository(db)
	commRepoUI := commercePostgres.NewRepository(db)
	invRepoUI := inventoryPostgres.NewRepository(db)
	notifRepoUI := notificationsPostgres.NewRepository(db)
	promoRepoUI := promoPostgres.NewRepository(db)
	adminRepoUI := platformadminPostgres.NewRepository(db)

	uiHandler := ui.NewUIHandler(
		catalog.NewService(catRepoUI, log),
		org.NewService(orgRepoUI, log),
		ingest.NewService(ingRepoUI, log),
		commerce.NewService(commRepoUI, log),
		inventory.NewService(invRepoUI, log),
		idSvc,
		notifications.NewService(notifRepoUI, log),
		promo.NewService(promoRepoUI, log),
		platformadmin.NewService(adminRepoUI, log),
		log,
	)
	uiHandler.RegisterPageRoutes(r)
}

// mountAuthenticatedModules registers every module whose endpoints require a
// logged-in caller. It is a separate function so the authentication group above
// reads as one decision rather than being repeated per module.
func mountAuthenticatedModules(
	r chi.Router,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
) {
	db := deps.Handle()

	// 2. Catalog
	catRepo := catalogPostgres.NewRepository(db)
	catSvc := catalog.NewService(catRepo, log)
	catalogHttp.NewHandler(catSvc, log).RegisterRoutes(r)

	// 3. Inventory
	invRepo := inventoryPostgres.NewRepository(db)
	invSvc := inventory.NewService(invRepo, log)
	inventoryHttp.NewHandler(invSvc, log).RegisterRoutes(r)

	// 4. Commerce
	commRepo := commercePostgres.NewRepository(db)
	commSvc := commerce.NewService(commRepo, log)
	commerceHttp.NewHandler(commSvc, log).RegisterRoutes(r)

	// 5. Billing & Entitlements
	billRepo := billingPostgres.NewRepository(db)
	billSvc := billing.NewService(billRepo, log)
	billingHttp.NewHandler(billSvc, log).RegisterRoutes(r)

	// 6. Ingest & AI Matching
	aiSvc := aicapabilities.NewService(ai, log)
	ingRepo := ingestPostgres.NewRepository(db)
	ingSvc := ingest.NewService(ingRepo, log)
	ingSvc.SetAIMatcher(aiSvc)
	ingestHttp.NewHandler(ingSvc, log).RegisterRoutes(r)

	// 7. Promo, Offers & Ads
	promoRepo := promoPostgres.NewRepository(db)
	promoSvc := promo.NewService(promoRepo, log)
	promoHttp.NewHandler(promoSvc, log).RegisterRoutes(r)

	// 8. Workflow
	wfRepo := workflowPostgres.NewRepository(db)
	wfSvc := workflow.NewService(wfRepo, log)
	workflowHttp.NewHandler(wfSvc, log).RegisterRoutes(r)

	// 9. HR
	hrRepo := hrPostgres.NewRepository(db)
	hrSvc := hr.NewService(hrRepo, log)
	hrHttp.NewHandler(hrSvc, log).RegisterRoutes(r)

	// 10. Platform Admin
	paRepo := platformadminPostgres.NewRepository(db)
	paSvc := platformadmin.NewService(paRepo, log)
	platformadminHttp.NewHandler(paSvc, log).RegisterRoutes(r)

	// 11. Notifications
	notifRepo := notificationsPostgres.NewRepository(db)
	notifSvc := notifications.NewService(notifRepo, log)
	notificationsHttp.NewHandler(notifSvc, log).RegisterRoutes(r)

	// 12. Organizations & Tenants
	orgRepo := orgPostgres.NewRepository(db)
	orgSvc := org.NewService(orgRepo, log)
	orgHttp.NewHandler(orgSvc, log).RegisterRoutes(r)

}
