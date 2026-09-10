package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

var _ assistant.ProjectionReader = (*Repository)(nil)

// ReadProjection dispatches only to SQL selected by a server-owned projection
// constant. Model text is bound as a value and can never choose a table,
// column, or SQL fragment.
func (r *Repository) ReadProjection(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	if q.Limit <= 0 || q.Limit > assistant.PageLimit {
		q.Limit = assistant.PageLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	switch q.Kind {
	case assistant.ProjectionAccountProfile:
		if actor.DashboardScope() == "vendor" {
			return r.readVendorProjection(ctx, actor, q)
		}
		return r.readPharmacyProjection(ctx, actor, q)
	case assistant.ProjectionReorderSuggestions, assistant.ProjectionCatalogSearch,
		assistant.ProjectionOfferDetails, assistant.ProjectionCartSummary,
		assistant.ProjectionPurchaseRequests, assistant.ProjectionInvoices,
		assistant.ProjectionInvoiceDetails, assistant.ProjectionPayments,
		assistant.ProjectionSavingProducts, assistant.ProjectionSmartOrderRuns,
		assistant.ProjectionSmartOrderDetails, assistant.ProjectionDecisionMemory,
		assistant.ProjectionBranchQuota, assistant.ProjectionSupplierProfile,
		assistant.ProjectionFavourites, assistant.ProjectionNotifications,
		assistant.ProjectionSpendingInsights, assistant.ProjectionPurchaseRequestDetails:
		return r.readPharmacyProjection(ctx, actor, q)
	case assistant.ProjectionVariantDetails, assistant.ProjectionStockByWarehouse,
		assistant.ProjectionWarehouseTransfers, assistant.ProjectionQuotaReport,
		assistant.ProjectionCoverageReport, assistant.ProjectionOrdersByStatus,
		assistant.ProjectionRevenueByPeriod, assistant.ProjectionRevenueByProduct,
		assistant.ProjectionCustomers, assistant.ProjectionImportRuns,
		assistant.ProjectionImportRunDetails, assistant.ProjectionOffersPerformance,
		assistant.ProjectionSponsorshipStatus, assistant.ProjectionTeam,
		assistant.ProjectionReviews, assistant.ProjectionInventoryHealth,
		assistant.ProjectionSalesInsights, assistant.ProjectionIncomingQuotes,
		assistant.ProjectionIncomingQuoteDetails:
		return r.readVendorProjection(ctx, actor, q)
	case assistant.ProjectionOrganizations, assistant.ProjectionOrganizationDetails,
		assistant.ProjectionApprovals, assistant.ProjectionDeletionRequests,
		assistant.ProjectionUsers, assistant.ProjectionUserDetails,
		assistant.ProjectionErrorLogs, assistant.ProjectionAuditLog,
		assistant.ProjectionFinance, assistant.ProjectionWalletTransactions,
		assistant.ProjectionSubscriptions, assistant.ProjectionVisitors,
		assistant.ProjectionHealth, assistant.ProjectionMatchDecisions,
		assistant.ProjectionInstitutionalGraph:
		return r.readAdminProjection(ctx, actor, q)
	default:
		return assistant.Page[assistant.ProjectionRow]{}, fmt.Errorf("assistant: unknown projection %q", q.Kind)
	}
}

func projectionArgs(orgID int64, q assistant.ProjectionQuery) []any {
	return []any{
		pgtype.Int8{Int64: orgID, Valid: true}, textParam(q.Search), textParam(q.Status),
		timeParam(q.Range.From), timeParam(q.Range.To), q.Limit + 1, q.Offset,
	}
}

func adminProjectionArgs(q assistant.ProjectionQuery) []any {
	return []any{
		textParam(q.Search), textParam(q.Status), timeParam(q.Range.From), timeParam(q.Range.To),
		q.Limit + 1, q.Offset,
	}
}

func textParam(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func timeParam(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero()}
}

func (r *Repository) readProjectionRows(
	ctx context.Context, actor authctx.Actor, sql string, args []any,
	limit, offset int, system bool,
) (assistant.Page[assistant.ProjectionRow], error) {
	var out []assistant.ProjectionRow
	readCtx := ctx
	if system {
		readCtx = database.AsSystem(ctx)
	}
	err := r.db.InReadTx(readCtx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.ProjectionRow
			var raw []byte
			if err := rows.Scan(&row.ID, &raw); err != nil {
				return err
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if err := dec.Decode(&row.Values); err != nil {
				return err
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	return pageOf(out, limit, offset), nil
}
