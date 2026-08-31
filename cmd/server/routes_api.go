package main

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"log/slog"

	"github.com/go-chi/chi/v5"

	attachmentsPostgres "github.com/muhiya/dawa24-store/internal/modules/attachments/postgres"
	identityPostgres "github.com/muhiya/dawa24-store/internal/modules/identity/postgres"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	attachmentsHttp "github.com/muhiya/dawa24-store/internal/modules/attachments/http"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgHttp "github.com/muhiya/dawa24-store/internal/modules/org/http"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/storage"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// mountModuleRoutes registers domain handlers across all platform bounded contexts.
func mountModuleRoutesAPI(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	deps *dependencies,
	ai gateway.Client,
	adminKeys *adminKeyProvisioner,
	tenantKeys *tenantKeyProvisioner,
) (*database.DB, *identity.Service, *attachments.Service, func(ctx context.Context, orgID int64, orgType string) error, *storage.Client, *rbac.Resolver) {
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

	// Â§4.2 documents gate: an organization with missing mandatory documents
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

	// Authenticated API routes â€” Pre-approval / Onboarding allowlist
	// (Document upload and own organisation status queries needed to achieve approval)
	r.Group(func(preApproval chi.Router) {
		preApproval.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		preApproval.Use(identityHttp.ResolveTenant(idSvc, log))

		// Attachments API (document uploads and confirmation for verification)
		attachmentsHttp.NewHandler(attachSvc, log).RegisterRoutes(preApproval)

		// Organization registration & own org query during onboarding
		orgRepoAPI := orgPostgres.NewRepository(db)
		orgSvcAPI := org.NewService(orgRepoAPI, log)
		orgHttp.NewHandler(orgSvcAPI, log).RegisterPreApprovalRoutes(preApproval)
	})

	// Authenticated API routes â€” Approved organizations only
	r.Group(func(approved chi.Router) {
		approved.Use(identityHttp.RequireAuth(idSvc, permissions, cfg.Session.CookieName, log))
		approved.Use(identityHttp.ResolveTenant(idSvc, log))
		approved.Use(authctx.RequireApproved(log))

		mountAuthenticatedModules(approved, cfg, log, deps, ai, adminKeys, tenantKeys, storageClient, docsGate)
	})

	return db, idSvc, attachSvc, docsGate, storageClient, permissions
}
