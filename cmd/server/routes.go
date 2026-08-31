package main

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"

	"github.com/go-chi/chi/v5"

	assistantPostgres "github.com/muhiya/dawa24-store/internal/modules/assistant/postgres"
	billingPostgres "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	commercePostgres "github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	hrPostgres "github.com/muhiya/dawa24-store/internal/modules/hr/postgres"
	ingestPostgres "github.com/muhiya/dawa24-store/internal/modules/ingest/postgres"
	inventoryPostgres "github.com/muhiya/dawa24-store/internal/modules/inventory/postgres"
	notificationsPostgres "github.com/muhiya/dawa24-store/internal/modules/notifications/postgres"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	platformadminPostgres "github.com/muhiya/dawa24-store/internal/modules/platform_admin/postgres"
	promoPostgres "github.com/muhiya/dawa24-store/internal/modules/promo/postgres"
	workflowPostgres "github.com/muhiya/dawa24-store/internal/modules/workflow/postgres"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	assistantHttp "github.com/muhiya/dawa24-store/internal/modules/assistant/http"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	billingHttp "github.com/muhiya/dawa24-store/internal/modules/billing/http"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogHttp "github.com/muhiya/dawa24-store/internal/modules/catalog/http"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	commerceHttp "github.com/muhiya/dawa24-store/internal/modules/commerce/http"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	hrHttp "github.com/muhiya/dawa24-store/internal/modules/hr/http"
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
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	smartorderHttp "github.com/muhiya/dawa24-store/internal/modules/smartorder/http"
	smartorderPG "github.com/muhiya/dawa24-store/internal/modules/smartorder/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	workflowHttp "github.com/muhiya/dawa24-store/internal/modules/workflow/http"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/storage"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func mountModuleRoutes(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
	adminKeys *adminKeyProvisioner,
	tenantKeys *tenantKeyProvisioner,
) {
	db, idSvc, attachSvc, docsGate, storageClient, permissions := mountModuleRoutesAPI(r, cfg, log, deps, ai, adminKeys, tenantKeys)

	uiHandler := buildUIHandler(cfg, log, deps, db, idSvc, attachSvc, docsGate, tenantKeys, adminKeys, storageClient, ai, permissions)

	uiHandler.RegisterPublicRoutes(r)

	isProd := cfg.Env == "production"

	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(httpx.CSRF(isProd))
		uiRouter.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
		uiRouter.Use(uiHandler.SiteSettingsMiddleware)
		uiRouter.Use(authctx.RequireCustomer(log))
		uiRouter.Use(authctx.RequireApproved(log))
		uiRouter.Use(uiHandler.BuyingBranchSelector)
		uiHandler.RegisterCustomerRoutes(uiRouter)
		uiHandler.RegisterSmartOrderRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(httpx.CSRF(isProd))
		uiRouter.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
		uiRouter.Use(uiHandler.SiteSettingsMiddleware)
		uiRouter.Use(authctx.RequireVendor(log))
		uiRouter.Use(authctx.RequireApproved(log))
		uiHandler.RegisterVendorRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(httpx.CSRF(isProd))
		uiRouter.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
		uiRouter.Use(uiHandler.SiteSettingsMiddleware)
		uiRouter.Use(authctx.RequireStaff(log))
		uiHandler.RegisterAdminRoutes(uiRouter)
	})
	// Tier A: Pre-approval shared routes (authenticated only, no RequireApproved)
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(httpx.CSRF(isProd))
		uiRouter.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
		uiRouter.Use(uiHandler.SiteSettingsMiddleware)
		uiHandler.RegisterPreApprovalRoutes(uiRouter)
	})

	// Tier B: Approved-only shared routes (RequireApproved mounted)
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(httpx.CSRF(isProd))
		uiRouter.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
		uiRouter.Use(uiHandler.SiteSettingsMiddleware)
		uiRouter.Use(authctx.RequireApproved(log))
		uiHandler.RegisterApprovedSharedRoutes(uiRouter)
	})

	// Tier C: Audience-specific customer shared routes
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(httpx.CSRF(isProd))
		uiRouter.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
		uiRouter.Use(uiHandler.SiteSettingsMiddleware)
		uiRouter.Use(authctx.RequireCustomer(log))
		uiHandler.RegisterCustomerSharedRoutes(uiRouter)
	})

	// Tier C: Audience-specific vendor shared routes
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(httpx.CSRF(isProd))
		uiRouter.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
		uiRouter.Use(uiHandler.SiteSettingsMiddleware)
		uiRouter.Use(authctx.RequireVendor(log))
		uiHandler.RegisterVendorSharedRoutes(uiRouter)
	})

}

// mountAuthenticatedModules registers every module whose endpoints require a logged-in caller.
func mountAuthenticatedModules(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
	adminKeys *adminKeyProvisioner,
	tenantKeys *tenantKeyProvisioner,
	storageClient *storage.Client,
	docsGate func(ctx context.Context, orgID int64, orgType string) error,
) {
	db := deps.Handle()

	// 2. Catalog
	catRepo := catalogPostgres.NewRepository(db)
	catSvc := catalog.NewService(catRepo, log)
	if cacheHandle := deps.CacheHandle(); cacheHandle != nil {
		catSvc.SetCache(cacheHandle)
	}
	catalogHttp.NewHandler(catSvc, log).RegisterRoutes(r)

	// 3. Inventory
	invRepo := inventoryPostgres.NewRepository(db)
	invSvc := inventory.NewService(invRepo, log)
	inventoryHttp.NewHandler(invSvc, log).RegisterRoutes(r)

	// 4. Commerce
	commRepo := commercePostgres.NewRepository(db)
	commSvc := commerce.NewService(commRepo, log)
	commSvc.SetRequiredDocsChecker(commerce.RequiredDocsChecker(docsGate))
	// The JSON API must enforce the same availability rules as the HTML surface,
	// otherwise it is a way around them.
	commSvc.SetAvailabilityProbe(newAvailabilityProbe(
		catalog.NewService(catalogPostgres.NewRepository(db), log),
		org.NewService(orgPostgres.NewRepository(db), log),
		workflow.NewCoverageService(db),
		inventory.NewService(inventoryPostgres.NewRepository(db), log),
	))
	commerceHttp.NewHandler(commSvc, log).RegisterRoutes(r)

	// 5. Billing & Entitlements
	billRepo := billingPostgres.NewRepository(db)
	billSvc := billing.NewService(billRepo, log)
	billSvc.SetAIPlanSync(tenantKeys)
	billingHttp.NewHandler(billSvc, log).RegisterRoutes(r)

	// 6. Ingest & AI Matching
	ingRepo := ingestPostgres.NewRepository(db)
	ingSvc := ingest.NewService(ingRepo, log)
	if storageClient != nil {
		ingSvc.SetStorage(storageClient)
	}
	ingestHttp.NewHandler(ingSvc, log).RegisterRoutes(r)

	// 7. Promo, Offers & Ads
	promoRepo := promoPostgres.NewRepository(db)
	promoSvc := promo.NewService(promoRepo, log)
	promoSvc.SetRequiredDocsChecker(promo.RequiredDocsChecker(docsGate))
	// Wallet debiter: charges the vendor's wallet for a sponsorship package
	// purchase. Composed here because promo must not import billing (ADR 0002).
	billSvcForPromo := billing.NewService(billRepo, log)
	promoSvc.SetWalletDebiter(func(ctx context.Context, orgID int64, amount money.Amount, description string) (*int64, error) {
		// Find the wallet for the org's first member (owner).
		// The billing service's GetOrCreateWallet is user-scoped; we resolve
		// the org's owner user from the tenant context.
		uid, _ := authctx.UserID(ctx)
		if uid <= 0 {
			return nil, apperr.Validation("auth.required", i18n.TDefault("w4_cmd.w4str_263_263"), nil)
		}
		wallet, err := billSvcForPromo.GetWallet(ctx, uid, "EGP")
		if err != nil {
			return nil, err
		}
		if wallet.Balance.Minor() < amount.Minor() {
			return nil, apperr.Conflict("wallet.insufficient_funds", i18n.TDefault("w4_cmd.w4str_264_264"))
		}
		tx, err := billSvcForPromo.Withdraw(ctx, uid, "EGP", amount, "sponsorship_package", nil, description)
		if err != nil {
			return nil, err
		}
		txID := tx.ID
		return &txID, nil
	})
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
	orgHttp.NewHandler(orgSvc, log).RegisterApprovedRoutes(r)

	// Every employee of a Ù…Ù†Ø´Ø£Ø© spends against that Ù…Ù†Ø´Ø£Ø©'s own Gateway key.
	//
	// The four hand-rolled copies of this â€” two here, one in the dashboard
	// handler, one in the admin handler â€” each provisioned inline on the request
	// path and each minted a fresh key every time, which revoked the one before
	// it. tenantKeyProvisioner does the same job once, with a per-organisation
	// lock, a validated cache, and a plan that follows the subscription.
	keyResolverAPI := func(ctx context.Context, orgID int64) (string, error) {
		return tenantKeys.Key(ctx, orgID), nil
	}

	// 13. Assistant (ÙƒØ¨Ø³ÙˆÙ„Ø©)
	assistantRepo := assistantPostgres.NewRepository(db)
	assistantSvc := assistant.NewService(assistantRepo, ai, log)
	assistantHandler := assistantHttp.NewHandler(assistantSvc, ai, assistantRepo, log)
	assistantHandler.SetKeyResolver(keyResolverAPI)
	assistantHandler.RegisterRoutes(r)

	// 14. Smart Order API
	smartorderRepo := smartorderPG.New(db)
	smartorderSvc := smartorder.NewService(smartorderRepo, log)
	soHandler := smartorderHttp.NewHandler(smartorderSvc, log)
	soFinalizer := smartorder.NewFinalizer(
		smartorderRepo,
		placeSmartOrder(commSvc, orgSvc, workflow.NewCoverageService(db), log),
		&reverifier{wfCoverage: workflow.NewCoverageService(db), orgSvc: orgSvc},
	)
	smartorderHttp.RegisterRoutes(r, soHandler, smartorderHttp.NewReviewer(soHandler, smartorderSvc, soFinalizer))

	instGateAPI := catalog.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvc.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	})
	catSvc.SetInstitutionalGate(instGateAPI)
	promoSvc.SetInstitutionalGate(promo.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvc.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	}))
	wfSvc.SetInstitutionalGate(workflow.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvc.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	}))
}
