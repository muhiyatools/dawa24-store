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

	// AI is optional. A nil adjudicator makes the pipeline skip the tier
	// entirely, which is the same path a disabled Gateway takes — the run
	// completes on deterministic results (AGENTS.md rule 3).
	var adjudicator pipeline.Adjudicator
	if ai != nil && ai.Enabled() {
		caps := aicapabilities.NewService(ai, log)
		caps.SetKeyResolver(smartOrderKeyResolver(orgSvc))
		adjudicator = pipeline.NewGatewayAdjudicator(&batchAdapter{caps: caps})
	}

	runner := pipeline.NewRunner(
		repo,
		coverageGate(coverage),
		institutionalGate(orgSvc),
		adjudicator,
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

// batchAdapter bridges aicapabilities to the pipeline's gateway contract.
type batchAdapter struct {
	caps *aicapabilities.Service
}

func (b *batchAdapter) AdjudicateBatch(ctx context.Context, items []pipeline.GatewayItem) ([]pipeline.GatewayDecision, error) {
	in := make([]aicapabilities.AdjudicateItem, 0, len(items))
	for _, it := range items {
		item := aicapabilities.AdjudicateItem{LineID: it.LineID, Text: it.Text}
		for _, c := range it.Candidates {
			item.Candidates = append(item.Candidates, aicapabilities.AdjudicateCandidate{
				ProductID:     c.ProductID,
				Name:          c.Name,
				NameEN:        c.NameEN,
				Scientific:    c.Scientific,
				DosageForm:    c.DosageForm,
				Concentration: c.Concentration,
				Manufacturer:  c.Manufacturer,
			})
		}
		in = append(in, item)
	}

	decisions, err := b.caps.AdjudicateBatch(ctx, in)
	if err != nil {
		return nil, err
	}
	out := make([]pipeline.GatewayDecision, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, pipeline.GatewayDecision{
			LineID:     d.LineID,
			ProductID:  d.ProductID,
			Confidence: d.Confidence,
			Reason:     d.Reason,
		})
	}
	return out, nil
}
