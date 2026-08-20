package postgres

import (
	"context"
	"encoding/json"
	"fmt"

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
			) VALUES ($1, $2, $3, $4, ` + coverageTimeParam(5) + `, ` + coverageTimeParam(6) + `, $7, $8, $9, $10, $11)
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

// coverageColumns is the canonical SELECT list for workflow.weekly_coverages.
//
// coverage_from and coverage_to are Postgres TIME columns and MUST be read
// through to_char. Scanning a TIME (OID 1083) straight into a Go *string fails
// — pgx maps TIME to pgtype.Time — which is what made the whole coverage screen
// error out. Every read of this table goes through this constant so a new query
// cannot reintroduce the bug.
const coverageColumns = `id, public_id, organization_id, branch_id, city_id, day_of_week,
	       to_char(coverage_from, 'HH24:MI') AS coverage_from,
	       to_char(coverage_to,   'HH24:MI') AS coverage_to,
	       address, latitude, longitude, distance_meters, is_active, created_at, updated_at`

// coverageTimeParam casts a bound parameter to TIME for the write path. The
// domain sends *string ("HH:MM" or nil); NULLIF guards the case where a caller
// bypasses workflow.TimeOfDay and passes an empty string, which Postgres would
// otherwise reject with `invalid input syntax for type time: ""`.
func coverageTimeParam(n int) string {
	return fmt.Sprintf("NULLIF($%d, '')::time", n)
}

// SaveWeeklyCoverage creates or updates weekly route coverage for a branch.
func (r *Repository) SaveWeeklyCoverage(ctx context.Context, c *workflow.WeeklyCoverage) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO workflow.weekly_coverages (
				organization_id, branch_id, city_id, day_of_week, coverage_from, coverage_to,
				address, latitude, longitude, distance_meters, is_active
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			c.OrganizationID, c.BranchID, c.CityID, c.DayOfWeek, c.CoverageFrom, c.CoverageTo,
			c.Address, c.Latitude, c.Longitude, c.DistanceMeters, c.IsActive,
		).Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt)
	})
}

// UpdateWeeklyCoverage updates an existing weekly coverage record.
func (r *Repository) UpdateWeeklyCoverage(ctx context.Context, c *workflow.WeeklyCoverage) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE workflow.weekly_coverages
			SET branch_id = $1, city_id = $2, day_of_week = $3,
			    coverage_from = ` + coverageTimeParam(4) + `, coverage_to = ` + coverageTimeParam(5) + `,
			    address = $6, latitude = $7, longitude = $8, distance_meters = $9, is_active = $10,
			    updated_at = now()
			WHERE id = $11 AND organization_id = $12
			RETURNING updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			c.BranchID, c.CityID, c.DayOfWeek, c.CoverageFrom, c.CoverageTo,
			c.Address, c.Latitude, c.Longitude, c.DistanceMeters, c.IsActive,
			c.ID, c.OrganizationID,
		).Scan(&c.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("weekly_coverage")
			}
			return err
		}
		return nil
	})
}

// DeleteWeeklyCoverage removes a weekly coverage entry.
func (r *Repository) DeleteWeeklyCoverage(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM workflow.weekly_coverages WHERE id = $1;`
		ct, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return apperr.NotFound("weekly_coverage")
		}
		return nil
	})
}

// ToggleWeeklyCoverage toggles the active state of a coverage record.
func (r *Repository) ToggleWeeklyCoverage(ctx context.Context, id int64, isActive bool) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE workflow.weekly_coverages SET is_active = $1, updated_at = now() WHERE id = $2;`
		ct, err := tx.Exec(txCtx, query, isActive, id)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return apperr.NotFound("weekly_coverage")
		}
		return nil
	})
}

// GetWeeklyCoverageByID retrieves a single weekly coverage record.
func (r *Repository) GetWeeklyCoverageByID(ctx context.Context, id int64) (*workflow.WeeklyCoverage, error) {
	var c workflow.WeeklyCoverage
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + coverageColumns + `
			FROM workflow.weekly_coverages
			WHERE id = $1;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&c.ID, &c.PublicID, &c.OrganizationID, &c.BranchID, &c.CityID, &c.DayOfWeek,
			&c.CoverageFrom, &c.CoverageTo, &c.Address, &c.Latitude, &c.Longitude,
			&c.DistanceMeters, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("weekly_coverage")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListWeeklyCoverage retrieves route schedules for a branch.
func (r *Repository) ListWeeklyCoverage(ctx context.Context, branchID int64) ([]*workflow.WeeklyCoverage, error) {
	var list []*workflow.WeeklyCoverage
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + coverageColumns + `
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
				&c.ID, &c.PublicID, &c.OrganizationID, &c.BranchID, &c.CityID, &c.DayOfWeek,
				&c.CoverageFrom, &c.CoverageTo, &c.Address, &c.Latitude, &c.Longitude,
				&c.DistanceMeters, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// ListCoverageForOrganization retrieves all weekly coverage records for an organization with joined branch names.
func (r *Repository) ListCoverageForOrganization(ctx context.Context, orgID int64) ([]*workflow.CoverageView, error) {
	var list []*workflow.CoverageView
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT wc.id, wc.public_id, wc.organization_id, wc.branch_id, wc.city_id,
			       wc.day_of_week,
			       to_char(wc.coverage_from, 'HH24:MI') AS coverage_from,
			       to_char(wc.coverage_to,   'HH24:MI') AS coverage_to,
			       wc.address,
			       wc.latitude, wc.longitude, wc.distance_meters, wc.is_active,
			       wc.created_at, wc.updated_at,
			       COALESCE(b.name, '') AS branch_name,
			       COALESCE(c.name->>'ar', c.name->>'en', '') AS city_name
			FROM workflow.weekly_coverages wc
			JOIN org.branches b ON b.id = wc.branch_id AND b.deleted_at IS NULL
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			WHERE wc.organization_id = $1
			ORDER BY wc.day_of_week ASC, b.name ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v workflow.CoverageView
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.BranchID, &v.CityID,
				&v.DayOfWeek, &v.CoverageFrom, &v.CoverageTo, &v.Address,
				&v.Latitude, &v.Longitude, &v.DistanceMeters, &v.IsActive,
				&v.CreatedAt, &v.UpdatedAt,
				&v.BranchName, &v.CityName,
			); err != nil {
				return err
			}
			list = append(list, &v)
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
