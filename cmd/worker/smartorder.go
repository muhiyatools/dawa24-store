package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgPG "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	smartorderJobs "github.com/muhiya/dawa24-store/internal/modules/smartorder/jobs"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	smartorderPG "github.com/muhiya/dawa24-store/internal/modules/smartorder/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// registerSmartOrderWorker wires the smart ordering pipeline into the worker.
//
// The pipeline's dependencies arrive as narrow interfaces so the module never
// imports workflow, org or the gateway (AGENTS.md rule 5 and rule 2). The
// adaptation happens here, in the composition root, where knowing about all of
// them is the job.
func registerSmartOrderWorker(
	workers *river.Workers,
	db *database.DB,
	ai gateway.Client,
	log *slog.Logger,
) {
	repo := smartorderPG.New(db)
	coverage := workflow.NewCoverageService(db)
	orgSvc := org.NewService(orgPG.NewRepository(db), log)

	// AI is optional. A nil enhancer makes the pipeline skip the stage
	// entirely, which is the same path a disabled Gateway takes — the run
	// completes on deterministic results (AGENTS.md rule 3).
	var enhancer pipeline.Enhancer
	if ai != nil && ai.Enabled() {
		caps := aicapabilities.NewService(ai, log)
		caps.SetKeyResolver(smartOrderKeyResolver(orgSvc))
		enhancer = pipeline.NewGatewayEnhancer(&enhanceAdapter{caps: caps})
	}

	runner := pipeline.NewRunner(
		repo,
		coverageGate(coverage),
		institutionalGate(orgSvc),
		enhancer,
		log,
	)

	river.AddWorker(workers, smartorderJobs.NewRunWorker(
		repo, runner, branchResolver(orgSvc), log,
	))
}

// coverageGate adapts the weekly-coverage service.
func coverageGate(cs *workflow.CoverageService) pipeline.CoverageGate {
	return smartorder.CoverageFunc(func(ctx context.Context, vendorOrgID int64,
		day time.Weekday, lat, lng float64) (bool, int, error) {
		return cs.ServesPoint(ctx, vendorOrgID, day, workflow.Coord{Lat: lat, Lon: lng})
	})
}

// institutionalGate adapts Corporate Operations in Simple mode — the same mode
// ordinary catalogue browsing uses, so a buyer can never smart-order something
// they could not have found by browsing.
func institutionalGate(svc *org.Service) pipeline.InstitutionalGate {
	return smartorder.InstitutionalFunc(func(ctx context.Context, buyerOrgID int64, workIDs []int64) (bool, error) {
		if len(workIDs) == 0 {
			return true, nil // unrestricted products are visible to everyone
		}
		assignments, err := svc.ListOrgEmployeeInstitutionalWorks(ctx, buyerOrgID)
		if err != nil {
			// Failing closed would hide the whole catalogue from a buyer because
			// one lookup timed out. Failing open matches Simple mode's bias, and
			// checkout re-validates before anything is actually bought.
			return true, nil
		}
		for _, want := range workIDs {
			for _, a := range assignments {
				if a.InstitutionalWorkID == want {
					return true, nil
				}
			}
		}
		return false, nil
	})
}

// branchResolver supplies the delivery branch's coordinates.
func branchResolver(svc *org.Service) smartorderJobs.BranchResolver {
	return smartorder.BranchLocationFunc(func(ctx context.Context, orgID, branchID int64) (float64, float64, bool, error) {
		b, err := svc.GetBranch(ctx, branchID)
		if err != nil || b == nil {
			return 0, 0, false, err
		}
		if b.Latitude == nil || b.Longitude == nil {
			return 0, 0, false, nil
		}
		return *b.Latitude, *b.Longitude, true, nil
	})
}

// smartOrderKeyResolver bills adjudication to the buyer's organisation.
//
// Gateway identity is per organisation, not per user: that is how the platform
// provisions it, and it means one buyer's imports share one budget.
func smartOrderKeyResolver(svc *org.Service) aicapabilities.KeyResolver {
	return func(ctx context.Context, orgID int64) (string, error) {
		o, err := svc.GetOrganization(ctx, orgID)
		if err != nil || o == nil {
			return "", err
		}
		return o.AIVirtualKey, nil
	}
}

// enhanceAdapter bridges aicapabilities to the pipeline's gateway contract.
//
// The translation is mechanical and lives here rather than in either module,
// because this is the only place allowed to know about both.
type enhanceAdapter struct {
	caps *aicapabilities.Service
}

func (b *enhanceAdapter) EnhanceBatch(ctx context.Context, batch pipeline.GatewayBatch) ([]pipeline.GatewayOutcome, error) {
	req := aicapabilities.EnhanceRequest{
		Catalog: make([]aicapabilities.CatalogEntry, 0, len(batch.Catalog)),
		Items:   make([]aicapabilities.EnhanceItem, 0, len(batch.Items)),
		// Attribution for the AI usage ledger: the pharmacy reads i18n.TDefault("w4_mod.s_478_478") on its usage log, not a capability name.
		Feature: matchflow.FeatureSmartOrder,
	}
	for _, c := range batch.Catalog {
		req.Catalog = append(req.Catalog, aicapabilities.CatalogEntry(c))
	}
	for _, it := range batch.Items {
		req.Items = append(req.Items, aicapabilities.EnhanceItem(it))
	}

	decisions, err := b.caps.EnhanceMatches(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]pipeline.GatewayOutcome, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, pipeline.GatewayOutcome(d))
	}
	return out, nil
}
