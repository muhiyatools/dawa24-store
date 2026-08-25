package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const candidateColumns = `
	id, line_id, organization_id, vendor_org_id, variant_id, branch_id,
	price, discount, net_unit_price, unit, min_order_qty, stock_qty,
	is_followed, eligible, ineligible_reason, coverage_distance_m`

func scanCandidate(row pgx.Row) (*smartorder.Candidate, error) {
	var c smartorder.Candidate
	var discount float64
	var reason *string
	if err := row.Scan(
		&c.ID, &c.LineID, &c.OrganizationID, &c.VendorOrgID, &c.VariantID, &c.BranchID,
		&c.Price, &discount, &c.NetUnitPrice, &c.Unit, &c.MinOrderQty, &c.StockQty,
		&c.IsFollowed, &c.Eligible, &reason, &c.CoverageDistanceM,
	); err != nil {
		return nil, err
	}
	c.DiscountBps = int64(discount * 100)
	if reason != nil {
		c.IneligibleReason = smartorder.IneligibleReason(*reason)
	}
	return &c, nil
}

// ReplaceCandidates rewrites the candidate set for one line.
//
// Delete-then-insert rather than upsert: a re-run may find fewer vendors than
// before (a supplier deactivated, coverage changed), and leaving the stale ones
// behind would show the buyer offers that no longer exist.
func (r *Repository) ReplaceCandidates(ctx context.Context, lineID int64, candidates []smartorder.Candidate) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx,
			`DELETE FROM smartorder.line_candidates WHERE line_id = $1;`, lineID); err != nil {
			return err
		}
		if len(candidates) == 0 {
			return nil
		}
		return insertCandidateRows(txCtx, tx, candidates)
	})
}

func insertCandidateRows(ctx context.Context, tx pgx.Tx, candidates []smartorder.Candidate) error {
	const cols = 14
	values := make([]string, 0, len(candidates))
	args := make([]any, 0, len(candidates)*cols)

	for i, c := range candidates {
		base := i * cols
		ph := make([]string, cols)
		for j := 0; j < cols; j++ {
			ph[j] = "$" + strconv.Itoa(base+j+1)
		}
		values = append(values, "("+strings.Join(ph, ",")+")")

		var reason any
		if c.IneligibleReason != "" {
			reason = string(c.IneligibleReason)
		}
		args = append(args,
			c.LineID, c.OrganizationID, c.VendorOrgID, c.VariantID, c.BranchID,
			c.Price, float64(c.DiscountBps)/100, c.NetUnitPrice, c.Unit,
			c.MinOrderQty, c.StockQty, c.IsFollowed, c.Eligible, reason)
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO smartorder.line_candidates (
			line_id, organization_id, vendor_org_id, variant_id, branch_id,
			price, discount, net_unit_price, unit, min_order_qty, stock_qty,
			is_followed, eligible, ineligible_reason
		) VALUES `+strings.Join(values, ",")+`;`, args...)
	return err
}

// ListCandidates returns every candidate for a line — eligible and rejected —
// so the buyer's "why not that supplier" question is answerable (FR-038).
func (r *Repository) ListCandidates(ctx context.Context, orgID, lineID int64) ([]smartorder.Candidate, error) {
	var out []smartorder.Candidate
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx,
			`SELECT `+candidateColumns+` FROM smartorder.line_candidates
			 WHERE line_id = $1 AND organization_id = $2
			 ORDER BY eligible DESC, net_unit_price ASC, vendor_org_id ASC;`, lineID, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCandidate(rows)
			if err != nil {
				return err
			}
			out = append(out, *c)
		}
		return rows.Err()
	})
	return out, err
}

// GetCandidate loads one candidate, scoped to the caller's organisation.
func (r *Repository) GetCandidate(ctx context.Context, orgID, candidateID int64) (*smartorder.Candidate, error) {
	var c *smartorder.Candidate
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx,
			`SELECT `+candidateColumns+` FROM smartorder.line_candidates
			 WHERE id = $1 AND organization_id = $2;`, candidateID, orgID)
		var err error
		c, err = scanCandidate(row)
		if err == pgx.ErrNoRows {
			return apperr.NotFound("smart_order_candidate")
		}
		return err
	})
	return c, err
}

// UpsertSelections writes the chosen supplier per line, in batches.
func (r *Repository) UpsertSelections(ctx context.Context, selections []*smartorder.Selection) error {
	if len(selections) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for start := 0; start < len(selections); start += insertBatchSize {
			end := start + insertBatchSize
			if end > len(selections) {
				end = len(selections)
			}
			if err := upsertSelectionBatch(txCtx, tx, selections[start:end]); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertSelectionBatch(ctx context.Context, tx pgx.Tx, batch []*smartorder.Selection) error {
	const cols = 9
	values := make([]string, 0, len(batch))
	args := make([]any, 0, len(batch)*cols)

	for i, s := range batch {
		base := i * cols
		ph := make([]string, cols)
		for j := 0; j < cols; j++ {
			ph[j] = "$" + strconv.Itoa(base+j+1)
		}
		values = append(values, "("+strings.Join(ph, ",")+")")
		args = append(args,
			s.LineID, s.OrganizationID, s.CandidateID, string(s.DecidedBy),
			s.ToleranceApplied, s.SkippedCandidateID, s.SkippedExcessPct,
			s.UserOverridden, s.LineNet)
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO smartorder.line_selections (
			line_id, organization_id, candidate_id, decided_by,
			tolerance_applied, skipped_candidate_id, skipped_excess_pct,
			user_overridden, line_net
		) VALUES `+strings.Join(values, ",")+`
		ON CONFLICT (line_id) DO UPDATE SET
			candidate_id = EXCLUDED.candidate_id,
			decided_by = EXCLUDED.decided_by,
			tolerance_applied = EXCLUDED.tolerance_applied,
			skipped_candidate_id = EXCLUDED.skipped_candidate_id,
			skipped_excess_pct = EXCLUDED.skipped_excess_pct,
			user_overridden = EXCLUDED.user_overridden,
			line_net = EXCLUDED.line_net,
			updated_at = now();`, args...)
	return err
}

// GetSelection loads the chosen supplier for one line.
func (r *Repository) GetSelection(ctx context.Context, orgID, lineID int64) (*smartorder.Selection, error) {
	var s smartorder.Selection
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var decidedBy string
		err := tx.QueryRow(txCtx, `
			SELECT line_id, organization_id, candidate_id, decided_by,
			       tolerance_applied, skipped_candidate_id, skipped_excess_pct,
			       user_overridden, line_net, updated_at
			FROM smartorder.line_selections WHERE line_id = $1 AND organization_id = $2;`,
			lineID, orgID).Scan(
			&s.LineID, &s.OrganizationID, &s.CandidateID, &decidedBy,
			&s.ToleranceApplied, &s.SkippedCandidateID, &s.SkippedExcessPct,
			&s.UserOverridden, &s.LineNet, &s.UpdatedAt)
		if err == pgx.ErrNoRows {
			return apperr.NotFound("smart_order_selection")
		}
		if err != nil {
			return err
		}
		s.DecidedBy = smartorder.DecidedBy(decidedBy)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteSelection removes a line's supplier, used when the buyer removes the
// line or zeroes its quantity.
func (r *Repository) DeleteSelection(ctx context.Context, orgID, lineID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx,
			`DELETE FROM smartorder.line_selections WHERE line_id = $1 AND organization_id = $2;`,
			lineID, orgID)
		return err
	})
}
