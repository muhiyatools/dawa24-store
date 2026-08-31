package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	smartorderPG "github.com/muhiya/dawa24-store/internal/modules/smartorder/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// Running a smart order from the web process.
//
// The feature was originally written to hand the run to the River worker and
// let the buyer watch progress. That is still the right shape when the worker is
// deployed — but when it is not, the run sits in `queued` forever and the buyer
// stares at i18n.TDefault("w4_mod.s_477_477") that never finishes. There is no error to show and
// nothing in the logs, because nothing went wrong: nobody was listening.
//
// So the server processes the run itself, in a goroutine, and the worker stays
// registered for deployments that run it. Exactly one of them does the work:
// the runner claims the run by moving it `queued → processing` and the claim is
// conditional in SQL, so whichever gets there first wins and the other sees zero
// rows affected and stops.
//
// Two details that are easy to get wrong and expensive to debug:
//
//   - The request context must NOT be used. It is cancelled the moment the
//     redirect is written, which would kill the run a few milliseconds in.
//   - A panic in the pipeline must not take the web process down with it.

// smartOrderRunTimeout bounds one run. Generous, because a ten-thousand-row file
// against a thirty-thousand-product catalogue is minutes of honest work — but
// bounded, so a wedged run releases its slot instead of living forever.
const smartOrderRunTimeout = 20 * time.Minute

// inlineSmartOrderRunner processes a run inside the web process.
func inlineSmartOrderRunner(
	db *database.DB,
	orgSvc *org.Service,
	ai gateway.Client,
	log *slog.Logger,
) ui.SmartOrderEnqueueFunc {
	repo := smartorderPG.New(db)
	coverage := workflow.NewCoverageService(db)

	// AI is optional; a nil enhancer makes the pipeline skip the stage, which
	// is the same path a disabled Gateway takes.
	var enhancer pipeline.Enhancer
	if ai != nil && ai.Enabled() {
		caps := aicapabilities.NewService(ai, log)
		caps.SetKeyResolver(func(ctx context.Context, orgID int64) (string, error) {
			o, err := orgSvc.GetOrganization(ctx, orgID)
			if err != nil || o == nil {
				return "", err
			}
			return o.AIVirtualKey, nil
		})
		enhancer = pipeline.NewGatewayEnhancer(&serverEnhanceAdapter{caps: caps})
	}

	runner := pipeline.NewRunner(repo,
		serverCoverageGate(coverage),
		serverInstitutionalGate(orgSvc),
		enhancer, log)

	return func(_ context.Context, runID, orgID int64) error {
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("smart order run panicked", "run_id", runID, "panic", rec)
					_ = repo.UpdateRunStatus(context.Background(), runID,
						smartorder.StatusFailed, 3, "internal error while processing")
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), smartOrderRunTimeout)
			defer cancel()

			if err := executeSmartOrderRun(ctx, repo, runner, orgSvc, runID, orgID, log); err != nil {
				log.ErrorContext(ctx, "smart order run failed", "run_id", runID, "error", err)
				_ = repo.UpdateRunStatus(ctx, runID, smartorder.StatusFailed, 3, err.Error())
			}
		}()
		return nil
	}
}

// executeSmartOrderRun claims the run and drives the pipeline.
func executeSmartOrderRun(
	ctx context.Context,
	repo smartorder.Repository,
	runner *pipeline.Runner,
	orgSvc *org.Service,
	runID, orgID int64,
	log *slog.Logger,
) error {
	run, err := repo.GetRunByID(ctx, orgID, runID)
	if err != nil {
		return err
	}

	// Only a queued run is ours to take. Anything else has been claimed, is
	// already finished, or was reset by the buyer.
	if run.Status != smartorder.StatusQueued {
		log.InfoContext(ctx, "smart order run is not queued; leaving it alone",
			"run_id", runID, "status", run.Status)
		return nil
	}

	cfg, err := repo.GetConfig(ctx, runID)
	if err != nil {
		return err
	}

	branch := pipeline.BranchLocation{BranchID: run.BranchID}
	if b, err := orgSvc.GetBranch(ctx, run.BranchID); err == nil && b != nil &&
		b.Latitude != nil && b.Longitude != nil {
		branch.Lat, branch.Lng, branch.HasCoord = *b.Latitude, *b.Longitude, true
	} else {
		// Worth saying out loud: without coordinates every coverage check passes,
		// so the buyer may be offered suppliers who cannot reach them.
		log.WarnContext(ctx, "branch has no coordinates; coverage cannot be enforced",
			"run_id", runID, "branch_id", run.BranchID)
	}

	if claimer, ok := repo.(smartorder.RunClaimer); ok {
		claimed, err := claimer.ClaimRun(ctx, orgID, runID)
		if err != nil {
			return err
		}
		if !claimed {
			log.InfoContext(ctx, "smart order run claim lost", "run_id", runID)
			return nil
		}
		run.Status = smartorder.StatusProcessing
	} else {
		if err := run.TransitionTo(smartorder.StatusProcessing); err != nil {
			return err
		}
		if err := repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, ""); err != nil {
			return err
		}
	}

	if err := runner.Execute(ctx, run, cfg, branch); err != nil {
		return err
	}

	if err := run.TransitionTo(smartorder.StatusCompleted); err != nil {
		return err
	}
	return repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, "")
}

func serverCoverageGate(cs *workflow.CoverageService) pipeline.CoverageGate {
	return smartorder.CoverageFunc(func(ctx context.Context, vendorOrgID int64,
		day time.Weekday, lat, lng float64) (bool, int, error) {
		return cs.ServesPoint(ctx, vendorOrgID, day, workflow.Coord{Lat: lat, Lon: lng})
	})
}

// serverInstitutionalGate applies Corporate Operations in Simple mode.
func serverInstitutionalGate(svc *org.Service) pipeline.InstitutionalGate {
	return smartorder.InstitutionalFunc(func(ctx context.Context, buyerOrgID int64, workIDs []int64) (bool, error) {
		if len(workIDs) == 0 {
			return true, nil // unrestricted products are visible to everyone
		}
		assignments, err := svc.ListOrgEmployeeInstitutionalWorks(ctx, buyerOrgID)
		if err != nil {
			// Failing closed would hide the whole catalogue because one lookup
			// timed out; checkout re-validates before anything is bought.
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

// serverEnhanceAdapter bridges aicapabilities to the pipeline's gateway contract.
//
// The translation is mechanical and lives here rather than in either module,
// because this is the only place allowed to know about both.
type serverEnhanceAdapter struct {
	caps *aicapabilities.Service
}

func (b *serverEnhanceAdapter) EnhanceBatch(ctx context.Context, batch pipeline.GatewayBatch) ([]pipeline.GatewayOutcome, error) {
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
