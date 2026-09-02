package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Platform-wide reads.
//
// These are the only assistant queries that use AsSystem, and every one of them
// is guarded twice before it gets here: the tool declares rbac.ScopeAdmin, and
// Dispatch requires the caller to hold the specific admin permission the
// equivalent screen requires. Being staff is not sufficient for any of them.
//
// The shapes are deliberately coarse — registration records and counts. There
// is no admin tool that reads one tenant's order book, prices or documents. A
// staff member who needs that opens the screen, where the access is a page view
// somebody can audit rather than a sentence in a chat log.
//
// requireStaff is the third guard, and it exists because AsSystem is exactly
// the kind of call that must not be reachable by accident from a refactor.

func requireStaff(actor authctx.Actor) error {
	if !actor.IsStaff {
		return fmt.Errorf("assistant: platform read attempted by non-staff actor")
	}
	return nil
}

// Organizations searches the registry of companies.
func (r *Repository) Organizations(
	ctx context.Context, actor authctx.Actor, q assistant.ProductQuery,
) (assistant.Page[assistant.OrganizationRow], error) {
	var empty assistant.Page[assistant.OrganizationRow]
	if err := requireStaff(actor); err != nil {
		return empty, err
	}

	args := []any{}
	where := ` WHERE o.deleted_at IS NULL`
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (o.name->>'ar' ILIKE $%d OR o.name->>'en' ILIKE $%d)", n, n)
	}
	if q.Status != "" {
		args = append(args, q.Status)
		where += fmt.Sprintf(" AND o.status = $%d", len(args))
	}
	args = append(args, q.Limit+1, q.Offset)
	limitArg, offsetArg := len(args)-1, len(args)

	var out []assistant.OrganizationRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT o.id, `+nameExpr("o.name")+`, COALESCE(o.type,''),
			       COALESCE(o.status,''), o.created_at
			  FROM org.organizations o`+where+`
			 ORDER BY o.created_at DESC, o.id DESC
			 LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg)+`;
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.OrganizationRow
			if err := rows.Scan(&row.ID, &row.Name, &row.Type, &row.Status, &row.CreatedAt); err != nil {
				return err
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: organizations: %w", err)
	}
	return pageOf(out, q.Limit, q.Offset), nil
}

// PlatformOverview returns the operator's headline counts.
func (r *Repository) PlatformOverview(
	ctx context.Context, actor authctx.Actor, rng assistant.DateRange,
) (*assistant.PlatformSummary, error) {
	if err := requireStaff(actor); err != nil {
		return nil, err
	}

	orderWhere := ` WHERE o.deleted_at IS NULL`
	orderArgs := []any{}
	frag, orderArgs := dateFilter("o.created_at", rng, orderArgs)
	orderWhere += frag

	summary := &assistant.PlatformSummary{GMV: money.FromMinor(0)}
	summary.From, summary.To = rangeBounds(rng)

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// One pass over organisations rather than four COUNT queries: these
		// tables are small enough that the round trips cost more than the scan.
		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*),
			       COUNT(*) FILTER (WHERE type IN ('customer','pharmacy','chain_pharmacy','individual')),
			       COUNT(*) FILTER (WHERE type IN ('vendor','supplier','company','agency')),
			       COUNT(*) FILTER (WHERE status = 'pending')
			  FROM org.organizations
			 WHERE deleted_at IS NULL;
		`).Scan(&summary.Organizations, &summary.Pharmacies,
			&summary.Vendors, &summary.PendingApproval); err != nil {
			return err
		}

		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*) FROM identity.users WHERE deleted_at IS NULL;
		`).Scan(&summary.Users); err != nil {
			return err
		}

		var gmv string
		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*), COALESCE(SUM(o.total_amount),0)::text
			  FROM commerce.orders o`+orderWhere+`;
		`, orderArgs...).Scan(&summary.Orders, &gmv); err != nil {
			return err
		}
		summary.GMV = amount(gmv)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: platform overview: %w", err)
	}
	return summary, nil
}

// AIUsage summarises AI consumption per organisation and feature.
func (r *Repository) AIUsage(
	ctx context.Context, actor authctx.Actor, rng assistant.DateRange, limit int,
) (assistant.Page[assistant.AIUsageRow], error) {
	var empty assistant.Page[assistant.AIUsageRow]
	if err := requireStaff(actor); err != nil {
		return empty, err
	}

	args := []any{}
	where := ` WHERE 1 = 1`
	frag, args := dateFilter("u.created_at", rng, args)
	where += frag
	args = append(args, limit+1)

	var out []assistant.AIUsageRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT COALESCE(`+nameExpr("org.name")+`, 'المنصة'),
			       COALESCE(u.feature,''),
			       COUNT(*)::int,
			       COALESCE(SUM(u.input_tokens),0)::bigint,
			       COALESCE(SUM(u.output_tokens),0)::bigint,
			       COALESCE(SUM(u.cost_nano_usd),0)::bigint
			  FROM ai.usage_events u
			  LEFT JOIN org.organizations org ON org.id = u.organization_id`+where+`
			 GROUP BY 1, 2
			 ORDER BY 6 DESC, 3 DESC
			 LIMIT $`+fmt.Sprint(len(args))+`;
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.AIUsageRow
			var costNano int64
			if err := rows.Scan(&row.Organization, &row.Feature, &row.Calls,
				&row.InputTokens, &row.OutputTokens, &costNano); err != nil {
				return err
			}
			// Nano-USD to a two-decimal string, integer arithmetic throughout:
			// this is a bill, and a float here is a rounding argument later.
			row.CostUSD = fmt.Sprintf("%d.%02d", costNano/1_000_000_000,
				(costNano%1_000_000_000)/10_000_000)
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: ai usage: %w", err)
	}
	return pageOf(out, limit, 0), nil
}
