package main

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"

	assistantPostgres "github.com/muhiya/dawa24-store/internal/modules/assistant/postgres"
	attachmentsPostgres "github.com/muhiya/dawa24-store/internal/modules/attachments/postgres"
	billingPostgres "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	chatPostgres "github.com/muhiya/dawa24-store/internal/modules/chat/postgres"
	commercePostgres "github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
	comparePostgres "github.com/muhiya/dawa24-store/internal/modules/compare/postgres"
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
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	assistantHttp "github.com/muhiya/dawa24-store/internal/modules/assistant/http"
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	attachmentsHttp "github.com/muhiya/dawa24-store/internal/modules/attachments/http"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	billingHttp "github.com/muhiya/dawa24-store/internal/modules/billing/http"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	cataloggw "github.com/muhiya/dawa24-store/internal/modules/catalog/gateway"
	catalogHttp "github.com/muhiya/dawa24-store/internal/modules/catalog/http"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	commerceHttp "github.com/muhiya/dawa24-store/internal/modules/commerce/http"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
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

	aiusagePostgres "github.com/muhiya/dawa24-store/internal/platform/aiusage/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/components"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// mountModuleRoutes registers domain handlers across all platform bounded contexts.
func mountModuleRoutes(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
	adminKeys *adminKeyProvisioner,
	tenantKeys *tenantKeyProvisioner,
) {
	db := deps.Handle()

	// Initialize dynamic feature flags engine
	if _, err := features.Init(context.Background(), db, log); err != nil {
		log.Warn("failed to initialize features engine", "error", err)
	}

	// 1. Storage & Attachments
	var storageClient *storage.Client
	if sClient, err := storage.New(context.Background(), cfg.Storage); err == nil {
		storageClient = sClient
		log.Info("object storage client initialized", "bucket", cfg.Storage.Bucket)
	} else {
		log.Warn("object storage client not initialized", "error", err)
	}

	attachRepo := attachmentsPostgres.NewRepository(db)
	attachSvc := attachments.NewService(attachRepo, storageClient, log)

	// §4.2 documents gate: an organization with missing mandatory documents
	// cannot check out (customer) or publish offers (vendor). Composed here
	// because modules must not import each other. Fail closed: if the
	// documents service errors, trading is refused until it recovers.
	docsGate := func(ctx context.Context, orgID int64, orgType string) error {
		if orgID <= 0 {
			return nil
		}
		missing, err := attachSvc.MissingRequiredDocuments(ctx, orgID, orgType)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return apperr.Validation("documents.incomplete", "Organization must attach its mandatory documents before trading.", nil)
		}
		return nil
	}

	// 2. Identity & Auth
	idRepo := identityPostgres.NewRepository(db)
	sessionStore := identity.NewSessionStore(deps.CacheHandle(), cfg.Session)
	idSvc := identity.NewService(idRepo, sessionStore, log)

	// The permission resolver is shared by every gate in the process. It reads
	// a caller's effective permissions from the database and caches them
	// against identity.rbac_version, so a role change is visible to every
	// process within seconds rather than at the end of a 720-hour session.
	permissions := rbac.NewResolver(db)
	idSvc.SetPermissionResolver(permissions)

	identityHandler := identityHttp.NewHandler(idSvc, cfg.Session, log)
	identityHandler.SetResolver(permissions)
	identityHandler.RegisterRoutes(r)

	// Authenticated API routes
	r.Group(func(protected chi.Router) {
		protected.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		protected.Use(identityHttp.ResolveTenant(idSvc, log))

		// Attachments API
		attachmentsHttp.NewHandler(attachSvc, log).RegisterRoutes(protected)

		mountAuthenticatedModules(protected, cfg, log, deps, ai, adminKeys, tenantKeys, storageClient, docsGate)
	})

	// 13. Templ SSR Frontend & Static Assets
	// MapPicker embeds need the Google Maps Embed API key before the first page
	// renders; without it the picker falls back to coordinate entry + deep links.
	components.SetGoogleMapsAPIKey(cfg.Maps.GoogleMapsAPIKey)

	catRepoUI := catalogPostgres.NewRepository(db)
	orgRepoUI := orgPostgres.NewRepository(db)
	ingRepoUI := ingestPostgres.NewRepository(db)
	commRepoUI := commercePostgres.NewRepository(db)
	invRepoUI := inventoryPostgres.NewRepository(db)
	notifRepoUI := notificationsPostgres.NewRepository(db)
	promoRepoUI := promoPostgres.NewRepository(db)
	adminRepoUI := platformadminPostgres.NewRepository(db)
	billRepoUI := billingPostgres.NewRepository(db)
	chatRepoUI := chatPostgres.NewRepository(db)
	wfRepoUI := workflowPostgres.NewRepository(db)
	hrRepoUI := hrPostgres.NewRepository(db)

	orgSvcUI := org.NewService(orgRepoUI, log)

	instGate := catalog.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvcUI.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	})

	catSvcUI := catalog.NewService(catRepoUI, log)
	catSvcUI.SetInstitutionalGate(instGate)
	// The staged catalogue import: the admin reviews a parsed file before any of
	// it is written. Without a store the wizard reports itself unavailable
	// rather than falling back to writing blind.
	catSvcUI.SetImportStore(catRepoUI)
	if cacheHandle := deps.CacheHandle(); cacheHandle != nil {
		catSvcUI.SetCache(cacheHandle)
	}

	commSvcUI := commerce.NewService(commRepoUI, log)
	commSvcUI.SetRequiredDocsChecker(docsGate)
	// §1.2 availability gate: stock, supplier approval, branch ownership and
	// weekly coverage are checked in one place for every buying surface.
	// Composed here because commerce must not import catalog/org/workflow.
	uiAvailability := newAvailabilityProbe(
		catalog.NewService(catRepoUI, log),
		org.NewService(orgRepoUI, log),
		workflow.NewCoverageService(db),
		inventory.NewService(invRepoUI, log),
	)
	commSvcUI.SetAvailabilityProbe(uiAvailability)

	promoSvcUI := promo.NewService(promoRepoUI, log)
	promoSvcUI.SetRequiredDocsChecker(docsGate)
	// Wallet debiter for UI-driven sponsorship purchases.
	billSvcUIForPromo := billing.NewService(billRepoUI, log)
	promoSvcUI.SetWalletDebiter(func(ctx context.Context, orgID int64, amount money.Amount, description string) (*int64, error) {
		uid, _ := authctx.UserID(ctx)
		if uid <= 0 {
			return nil, apperr.Validation("auth.required", "يجب تسجيل الدخول لشراء الباقة.", nil)
		}
		wallet, err := billSvcUIForPromo.GetWallet(ctx, uid, "EGP")
		if err != nil {
			return nil, err
		}
		if wallet.Balance.Minor() < amount.Minor() {
			return nil, apperr.Conflict("wallet.insufficient_funds", "رصيد المحفظة غير كافٍ. يرجى شحن المحفظة أولاً.")
		}
		tx, err := billSvcUIForPromo.Withdraw(ctx, uid, "EGP", amount, "sponsorship_package", nil, description)
		if err != nil {
			return nil, err
		}
		txID := tx.ID
		return &txID, nil
	})
	promoSvcUI.SetInstitutionalGate(promo.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvcUI.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	}))

	wfSvcUI := workflow.NewService(wfRepoUI, log)
	wfSvcUI.SetInstitutionalGate(workflow.InstitutionalGateFunc(func(ctx context.Context, userID int64, mode int) ([]int64, error) {
		return orgSvcUI.AllowedWorkIDs(ctx, userID, org.InstitutionalFilterMode(mode))
	}))

	invSvcUI := inventory.NewService(invRepoUI, log)

	ingSvcUI := ingest.NewService(ingRepoUI, log)
	if storageClient != nil {
		ingSvcUI.SetStorage(storageClient)
	}
	// The rebuilt vendor catalogue import. All three ports are required for the
	// screen to be offered: without the store there is nowhere to hold a review,
	// and without the catalogue there is nothing to match against. Wiring them
	// here keeps the module free of the composition root.
	ingSvcUI.SetImportStore(ingRepoUI)
	// The decision cache the AI stage reads before it asks anything. Shared
	// with the smart order, on purpose: both ask the same question through the
	// same prompt, so an answer either bought is free to the other.
	ingSvcUI.SetMatchMemory(ingRepoUI)
	// The same cache, in the shared vocabulary, for the two import paths that
	// had none: the saving-list import and the administrator's master-catalogue
	// import. Four tools, one table, one key — an answer bought by any of them
	// is free to the other three.
	sharedMatchMemory := newMatchMemory(ingRepoUI)
	catSvcUI.SetMatchMemory(sharedMatchMemory)
	ingSvcUI.SetCatalogPort(catSvcUI)
	ingSvcUI.SetInventoryPort(invSvcUI)

	// A subscription bought or assigned here has to move the tenant's AI quota
	// with it, which is what the sync port does. Without it an upgrade changed
	// the invoice and nothing else.
	billSvcUI := billing.NewService(billRepoUI, log)
	billSvcUI.SetAIPlanSync(tenantKeys)

	uiHandler := ui.NewUIHandler(
		catSvcUI,
		orgSvcUI,
		ingSvcUI,
		commSvcUI,
		invSvcUI,
		idSvc,
		notifications.NewService(notifRepoUI, log),
		promoSvcUI,
		platformadmin.NewService(adminRepoUI, log),
		billSvcUI,
		chat.NewService(chatRepoUI, log),
		wfSvcUI,
		hr.NewService(hrRepoUI, log),
		attachSvc,
		log,
	)

	// Every employee of a منشأة spends against that منشأة's own Gateway key.
	//
	// The four hand-rolled copies of this — two here, one in the dashboard
	// handler, one in the admin handler — each provisioned inline on the request
	// path and each minted a fresh key every time, which revoked the one before
	// it. tenantKeyProvisioner does the same job once, with a per-organisation
	// lock, a validated cache, and a plan that follows the subscription.
	keyResolverUI := func(ctx context.Context, orgID int64) (string, error) {
		return tenantKeys.Key(ctx, orgID), nil
	}

	// The saving-list import reaches the shared cache through the handler,
	// because that is where its staging runs.
	uiHandler.SetMatchMemory(sharedMatchMemory)

	compareRepoUI := comparePostgres.NewRepository(db)
	compareSvcUI := compare.NewService(compareRepoUI, log)
	if ai != nil {
		uiHandler.SetGatewayClient(ai)
		// The settings screen resets this when an operator changes the Gateway
		// credentials the admin panel's key was issued from.
		uiHandler.SetGatewayKeyCache(adminKeys)
		uiHandler.SetTenantGatewayKeys(tenantKeys)
		// The usage screens read the local ledger rather than calling the
		// Gateway on every render.
		uiHandler.SetAIUsage(aiusagePostgres.NewRepository(db))
		aiCapabilitiesSvc := aicapabilities.NewService(ai, log)
		aiCapabilitiesSvc.SetKeyResolver(keyResolverUI)
		compareSvcUI.SetAIMatcher(aiCapabilitiesSvc)
		// The compare tool now runs the same catalogue matching stage as every
		// other importer, so it gets the same enhancer and the same decision
		// cache. Before this it had a matched_product_id column that nothing
		// ever wrote, and its comparisons joined supplier lines to each other
		// on a normalised string instead of to the catalogue.
		compareSvcUI.SetMatchEnhancer(&ingestEnhanceAdapter{caps: aiCapabilitiesSvc})

		// The catalogue import's three mapping calls: which column is which
		// field, and which existing category and pharmaceutical form each of the
		// file's distinct words means. Three requests per import regardless of
		// how many rows it has; the rows themselves never reach a model.
		mapper := cataloggw.NewMapper(ai, log)
		mapper.SetKeyResolver(cataloggw.KeyResolver(keyResolverUI))
		catSvcUI.SetAIMapper(mapper)
		// The fourth call, and the only one that is about rows: the products
		// similarity could not tie to an existing catalogue entry, adjudicated
		// in batches of twenty-five against a shortlist the importer retrieved.
		// Without it the import matched barcodes and identical spellings only,
		// and staged everything else as a new product.
		// The administrator's import asks the same question as the other three,
		// so it asks it through the same capability rather than through a third
		// prompt of its own. The mapper still answers the two questions that are
		// genuinely its own — which column is which, and what a category word
		// means — and those are asked once per file, not once per row.
		catSvcUI.SetMatchAdjudicator(&catalogAdjudicateAdapter{caps: aiCapabilitiesSvc})
		// The vendor import runs the smart order's enhancement stage: the same
		// system prompt, the same shared catalogue window, the same guards, and
		// the same decision cache in catalog.match_decisions — so an answer
		// bought by a pharmacy's order is free to the vendor whose price list
		// asks the same question, and there is one prompt to tune rather than
		// two that drift.
		enhancer := &ingestEnhanceAdapter{caps: aiCapabilitiesSvc}
		ingSvcUI.SetEnhancer(ingest.NewGatewayEnhancer(enhancer))
		// The saving-list import runs the same stage through the same adapter.
		// It was the last of the four importers without one, and giving it a
		// second implementation would have been the mistake this whole refactor
		// exists to undo.
		uiHandler.SetMatchEnhancer(enhancer)
	}
	if storageClient != nil {
		compareSvcUI.SetStorage(storageClient)
	}
	compareSvcUI.SetCatalogSource(newCompareCatalog(catSvcUI))
	compareSvcUI.SetMatchMemory(sharedMatchMemory)
	uiHandler.SetCompareService(compareSvcUI)

	if storageClient != nil {
		uiHandler.SetStorage(storageClient)
	}
	uiHandler.SetAssistantRepository(assistantPostgres.NewRepository(db))
	uiHandler.SetPermissionResolver(permissions)
	// A company that has no roles yet gets them the first time its owner opens
	// the roles or team screen. The boot seeder covers companies that already
	// existed; this covers one registered while the process was running, and a
	// registration whose seeding step failed.
	uiHandler.SetRoleSeeder(func(ctx context.Context, orgID int64, orgType string) error {
		return rbac.EnsureCompanyRoles(ctx, db, orgID, orgType)
	})

	// Smart ordering (specs/001-smart-ordering-system). The server has no queue
	// client, so runs are left queued and the worker collects them — which is
	// also what makes a run survive a server restart mid-import.
	wireSmartOrder(db, uiHandler, orgSvcUI, workflow.NewCoverageService(db), commSvcUI, ai, log)

	// Audience-gated UI groups (Rebuild V2 §1.3). Every route is registered
	// under exactly one group; a route living outside these groups means it is
	// reachable by anyone regardless of account type — test/route_audience_test.go
	// walks the app the same way admin_guard_test.go does and forbids that.
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
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(httpx.CSRF(isProd))
		uiRouter.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		uiRouter.Use(identityHttp.ResolveTenant(idSvc, log))
		uiRouter.Use(uiHandler.SiteSettingsMiddleware)
		uiHandler.RegisterSharedRoutes(uiRouter)
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
			return nil, apperr.Validation("auth.required", "يجب تسجيل الدخول لشراء الباقة.", nil)
		}
		wallet, err := billSvcForPromo.GetWallet(ctx, uid, "EGP")
		if err != nil {
			return nil, err
		}
		if wallet.Balance.Minor() < amount.Minor() {
			return nil, apperr.Conflict("wallet.insufficient_funds", "رصيد المحفظة غير كافٍ. يرجى شحن المحفظة أولاً.")
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
	orgHttp.NewHandler(orgSvc, log).RegisterRoutes(r)

	// Every employee of a منشأة spends against that منشأة's own Gateway key.
	//
	// The four hand-rolled copies of this — two here, one in the dashboard
	// handler, one in the admin handler — each provisioned inline on the request
	// path and each minted a fresh key every time, which revoked the one before
	// it. tenantKeyProvisioner does the same job once, with a per-organisation
	// lock, a validated cache, and a plan that follows the subscription.
	keyResolverAPI := func(ctx context.Context, orgID int64) (string, error) {
		return tenantKeys.Key(ctx, orgID), nil
	}

	// 13. Assistant (كبسولة)
	assistantRepo := assistantPostgres.NewRepository(db)
	assistantSvc := assistant.NewService(assistantRepo, ai, log)
	assistantHandler := assistantHttp.NewHandler(assistantSvc, ai, assistantRepo, log)
	assistantHandler.SetKeyResolver(keyResolverAPI)
	assistantHandler.RegisterRoutes(r)

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
