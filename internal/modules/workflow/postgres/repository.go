package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements workflow.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a workflow PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreatePriorityRequest writes a purchasing priority task.
func (r *Repository) CreatePriorityRequest(ctx context.Context, req *workflow.PurchasePriorityRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		paramsJSON, _ := json.Marshal(req.Parameters)
		recomJSON, _ := json.Marshal(req.Recommendations)

		query := `
			INSERT INTO workflow.purchase_priority_engines (
				user_id, organization_id, request_number, status, priority_highest_discount,
				priority_lowest_price, priority_fastest_delivery, priority_preferred_suppliers_only,
				budget_constraint, parameters, recommendations
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			req.UserID, req.OrganizationID, req.RequestNumber, req.Status, req.PriorityHighestDiscount,
			req.PriorityLowestPrice, req.PriorityFastestDelivery, req.PriorityPreferredSuppliersOnly,
			req.BudgetConstraint, paramsJSON, recomJSON,
		).Scan(&req.ID, &req.PublicID, &req.CreatedAt, &req.UpdatedAt)
	})
}

// GetPriorityRequestByID retrieves a priority request.
func (r *Repository) GetPriorityRequestByID(ctx context.Context, id int64) (*workflow.PurchasePriorityRequest, error) {
	var req workflow.PurchasePriorityRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, request_number, status,
			       priority_highest_discount, priority_lowest_price, priority_fastest_delivery,
			       priority_preferred_suppliers_only, budget_constraint, parameters, recommendations,
			       processed_at, created_at, updated_at
			FROM workflow.purchase_priority_engines
			WHERE id = $1;
		`
		var paramsJSON, recomJSON []byte
		err := tx.QueryRow(txCtx, query, id).Scan(
			&req.ID, &req.PublicID, &req.UserID, &req.OrganizationID, &req.RequestNumber, &req.Status,
			&req.PriorityHighestDiscount, &req.PriorityLowestPrice, &req.PriorityFastestDelivery,
			&req.PriorityPreferredSuppliersOnly, &req.BudgetConstraint, &paramsJSON, &recomJSON,
			&req.ProcessedAt, &req.CreatedAt, &req.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("priority_request")
			}
			return err
		}
		_ = json.Unmarshal(paramsJSON, &req.Parameters)
		_ = json.Unmarshal(recomJSON, &req.Recommendations)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// SaveWeeklyCoverage creates or updates weekly route coverage for a branch.
func (r *Repository) SaveWeeklyCoverage(ctx context.Context, c *workflow.WeeklyCoverage) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO workflow.weekly_coverages (
				organization_id, branch_id, day_of_week, coverage_from, coverage_to, address, distance_meters, is_active
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			c.OrganizationID, c.BranchID, c.DayOfWeek, c.CoverageFrom, c.CoverageTo,
			c.Address, c.DistanceMeters, c.IsActive,
		).Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt)
	})
}

// ListWeeklyCoverage retrieves route schedules for a branch.
func (r *Repository) ListWeeklyCoverage(ctx context.Context, branchID int64) ([]*workflow.WeeklyCoverage, error) {
	var list []*workflow.WeeklyCoverage
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, branch_id, day_of_week, coverage_from, coverage_to,
			       address, distance_meters, is_active, created_at, updated_at
			FROM workflow.weekly_coverages
			WHERE branch_id = $1
			ORDER BY day_of_week ASC;
		`
		rows, err := tx.Query(txCtx, query, branchID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c workflow.WeeklyCoverage
			if err := rows.Scan(
				&c.ID, &c.PublicID, &c.OrganizationID, &c.BranchID, &c.DayOfWeek,
				&c.CoverageFrom, &c.CoverageTo, &c.Address, &c.DistanceMeters,
				&c.IsActive, &c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// CreateIssue creates a customer support or defect ticket.
func (r *Repository) CreateIssue(ctx context.Context, i *workflow.ReportIssue) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO workflow.report_issues (
				reported_by, organization_id, order_id, issue_type, description, status, priority
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			i.ReportedBy, i.OrganizationID, i.OrderID, i.IssueType, i.Description, i.Status, i.Priority,
		).Scan(&i.ID, &i.PublicID, &i.CreatedAt, &i.UpdatedAt)
	})
}

// GetIssueByID retrieves an issue ticket.
func (r *Repository) GetIssueByID(ctx context.Context, id int64) (*workflow.ReportIssue, error) {
	var i workflow.ReportIssue
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, reported_by, organization_id, order_id, issue_type,
			       description, status, priority, response_notes, created_at, updated_at
			FROM workflow.report_issues
			WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&i.ID, &i.PublicID, &i.ReportedBy, &i.OrganizationID, &i.OrderID,
			&i.IssueType, &i.Description, &i.Status, &i.Priority, &i.ResponseNotes,
			&i.CreatedAt, &i.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("issue")
		}
		return nil, err
	}
	return &i, nil
}

// ListIssues retrieves paginated tickets.
func (r *Repository) ListIssues(ctx context.Context, limit, offset int) ([]*workflow.ReportIssue, error) {
	var list []*workflow.ReportIssue
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, reported_by, organization_id, order_id, issue_type,
			       description, status, priority, response_notes, created_at, updated_at
			FROM workflow.report_issues
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var i workflow.ReportIssue
			if err := rows.Scan(
				&i.ID, &i.PublicID, &i.ReportedBy, &i.OrganizationID, &i.OrderID,
				&i.IssueType, &i.Description, &i.Status, &i.Priority, &i.ResponseNotes,
				&i.CreatedAt, &i.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &i)
		}
		return rows.Err()
	})
	return list, err
}
