package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Runner drives a smart order from staged rows to selected suppliers.
type Runner struct {
	repo          smartorder.Repository
	coverage      CoverageGate
	institutional InstitutionalGate
	ai            Adjudicator
	log           *slog.Logger
}

// NewRunner constructs the pipeline.
func NewRunner(repo smartorder.Repository, cov CoverageGate, inst InstitutionalGate,
	ai Adjudicator, log *slog.Logger) *Runner {
	return &Runner{repo: repo, coverage: cov, institutional: inst, ai: ai, log: log}
}

// Execute runs every stage for one smart order.
//
// The stage order is the performance design: exact tiers settle the bulk over
// the whole file, the in-memory scorer handles what is left, and AI sees only
// the residue of the residue. Progress is written after each stage so a buyer
// watching the page sees movement, and a buyer who left can tell where it got to.
func (r *Runner) Execute(ctx context.Context, run *smartorder.Run, cfg *smartorder.Config, branch BranchLocation) error {
	started := time.Now()

	lines, _, err := r.repo.ListLines(ctx, run.ID, smartorder.LineFilter{All: true})
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	run.Stats.TotalRows = len(lines)

	// Stage 1 — normalise. Pure CPU, no I/O.
	r.emit(ctx, run, smartorder.StageNormalize, 0, len(lines), "جارٍ تجهيز الأصناف", "Preparing items")
	Normalize(lines)
	smartorder.ApplyQuantities(cfg, lines)

	// Stage 2 — the exact tiers, each one query for the whole file.
	r.emit(ctx, run, smartorder.StageResolve, 0, len(lines), "مطابقة الأكواد والأسماء", "Matching codes and names")
	resolver := NewResolver(r.repo, cfg)
	if err := resolver.Resolve(ctx, lines); err != nil {
		return err
	}

	// Stage 3 — score what is left against an in-memory catalogue.
	residual := Unresolved(lines)
	r.emit(ctx, run, smartorder.StageCandidates, len(lines)-len(residual), len(lines),
		"مطابقة الأصناف المتبقية", "Matching remaining items")

	matcher := NewMatcher(r.repo)
	if err := matcher.Load(ctx); err != nil {
		return err
	}
	if matcher.Size() == 0 {
		// An empty catalogue is a broken run, not a file full of unknown
		// products. Saying so prevents a support ticket that starts "nothing
		// matched".
		r.log.ErrorContext(ctx, "catalogue index is empty; every line will be unmatched",
			"run_id", run.ID)
	}
	stillResidual := matcher.Score(residual)

	// Stage 4 — AI, only on what remains and only if the buyer asked for it.
	deterministicMS := int(time.Since(started).Milliseconds())
	run.DeterministicMS = &deterministicMS

	if cfg.UseAIMatching && r.ai != nil && len(stillResidual) > 0 {
		r.emit(ctx, run, smartorder.StageAdjudicate, 0, len(stillResidual),
			"مطابقة ذكية للأصناف الصعبة", "Resolving difficult items")
		adj := NewAdjudication(r.repo, r.ai)
		adj.Run(ctx, stillResidual)

		run.AI.Calls = adj.Stats.Requests
		run.AI.LinesAdjudicated = adj.Stats.Adjudicated
		run.AI.CeilingHit = adj.Stats.CeilingHit
		if adj.Stats.CeilingHit {
			r.emitWarn(ctx, run, smartorder.StageAdjudicate,
				"تم بلوغ حد المطابقة الذكية؛ بقيت بعض الأصناف بلا مطابقة",
				"AI matching limit reached; some items were left unmatched")
		}
	}

	if err := r.repo.UpdateLines(ctx, lines); err != nil {
		return err
	}

	// Stage 5 — suppliers, coverage, Corporate Operations, selection.
	r.emit(ctx, run, smartorder.StageSelect, 0, len(lines), "البحث عن الموردين", "Finding suppliers")
	supplier := NewSupplier(r.repo, r.coverage, r.institutional, cfg, branch)
	total, err := supplier.Resolve(ctx, lines)
	if err != nil {
		return err
	}

	if err := r.repo.UpdateLines(ctx, lines); err != nil {
		return err
	}

	run.Stats = smartorder.CountByOutcome(lines)
	run.EstimatedTotal = total
	exceeded, overage, hasBudget := cfg.BudgetStatus(total)
	if hasBudget {
		run.BudgetExceeded = &exceeded
		if exceeded {
			run.BudgetOverage = &overage
		}
	}
	totalMS := int(time.Since(started).Milliseconds())
	run.TotalMS = &totalMS

	return r.repo.UpdateRunStats(ctx, run)
}

func (r *Runner) emit(ctx context.Context, run *smartorder.Run, stage smartorder.Stage,
	processed, total int, ar, en string) {
	p, t := processed, total
	_ = r.repo.AppendEvent(ctx, &smartorder.Event{
		RunID:          run.ID,
		OrganizationID: run.OrganizationID,
		Stage:          stage,
		Processed:      &p,
		Total:          &t,
		Message:        i18n.Text{"ar": ar, "en": en},
	})
}

func (r *Runner) emitWarn(ctx context.Context, run *smartorder.Run, stage smartorder.Stage, ar, en string) {
	_ = r.repo.AppendEvent(ctx, &smartorder.Event{
		RunID:          run.ID,
		OrganizationID: run.OrganizationID,
		Stage:          stage,
		Message:        i18n.Text{"ar": ar, "en": en},
		Level:          "warn",
	})
}
