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

	// Every stage reports when it FINISHES, with what it actually settled.
	//
	// It used to report on entry, always with processed=0, so the screen showed
	// "0 / 804" under four stage names for the whole run and the buyer learned
	// nothing from any of it. A count is only worth showing once it counts
	// something.

	// Stage 1 — normalise. Pure CPU, no I/O.
	Normalize(lines)
	smartorder.ApplyQuantities(cfg, lines)
	r.emit(ctx, run, smartorder.StageNormalize, len(lines), len(lines),
		"جارٍ تجهيز الأصناف", "Preparing items")

	// Stage 2 — the exact tiers, each one query for the whole file.
	resolver := NewResolver(r.repo, cfg)
	if err := resolver.Resolve(ctx, lines); err != nil {
		return err
	}
	residual := Unresolved(lines)
	r.emit(ctx, run, smartorder.StageResolve, len(lines)-len(residual), len(lines),
		"مطابقة الأكواد والأسماء", "Matching codes and names")

	// Stage 3 — score what is left against an in-memory catalogue.
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
	r.emit(ctx, run, smartorder.StageCandidates, len(lines)-len(stillResidual), len(lines),
		"مطابقة الأصناف المتبقية", "Matching remaining items")

	// Stage 4 — AI, only on what remains and only if the buyer asked for it.
	deterministicMS := int(time.Since(started).Milliseconds())
	run.DeterministicMS = &deterministicMS

	if cfg.UseAIMatching && r.ai != nil && len(stillResidual) > 0 {
		total := len(stillResidual)
		adj := NewAdjudication(r.repo, r.ai)
		// This is the stage that waits on a network, so it is the one that has
		// to move the bar while it works. Reporting only on entry and exit left
		// the buyer watching a still number through the slowest minute of the
		// run.
		adj.OnProgress = func(done int) {
			r.emit(ctx, run, smartorder.StageAdjudicate, done, total,
				"مطابقة ذكية للأصناف الصعبة", "Resolving difficult items")
		}
		adj.Run(ctx, stillResidual)

		run.AI.Calls = adj.Stats.Requests
		run.AI.LinesAdjudicated = adj.Stats.Adjudicated
		run.AI.CeilingHit = adj.Stats.CeilingHit
		r.emit(ctx, run, smartorder.StageAdjudicate, total, total,
			"مطابقة ذكية للأصناف الصعبة", "Resolving difficult items")
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
	r.emit(ctx, run, smartorder.StageSelect, len(lines), len(lines),
		"البحث عن الموردين", "Finding suppliers")

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

	// A terminal event, so the bar reaches 100 rather than stopping at 99 and
	// being replaced by the results page mid-animation.
	r.emit(ctx, run, smartorder.StageFinalize, 1, 1,
		"اكتملت المطابقة", "Matching complete")

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
