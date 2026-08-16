package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements hr.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates an HR PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateEmployee inserts an employee under the active tenant.
func (r *Repository) CreateEmployee(ctx context.Context, e *hr.Employee) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO hr.employees (
				organization_id, user_id, employee_code, job_title,
				base_salary, variable_salary, status, hired_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			e.OrganizationID, e.UserID, e.EmployeeCode, e.JobTitle,
			e.BaseSalary, e.VariableSalary, e.Status, e.HiredAt,
		).Scan(&e.ID, &e.PublicID, &e.CreatedAt, &e.UpdatedAt)
	})
}

// GetEmployeeByID retrieves an employee profile.
func (r *Repository) GetEmployeeByID(ctx context.Context, id int64) (*hr.Employee, error) {
	var e hr.Employee
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, employee_code, job_title,
			       base_salary, variable_salary, status, hired_at, created_at, updated_at
			FROM hr.employees
			WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&e.ID, &e.PublicID, &e.OrganizationID, &e.UserID, &e.EmployeeCode, &e.JobTitle,
			&e.BaseSalary, &e.VariableSalary, &e.Status, &e.HiredAt, &e.CreatedAt, &e.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("employee")
		}
		return nil, err
	}
	return &e, nil
}

// ListEmployees retrieves paginated employees for the tenant.
func (r *Repository) ListEmployees(ctx context.Context, limit, offset int) ([]*hr.Employee, error) {
	var list []*hr.Employee
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, employee_code, job_title,
			       base_salary, variable_salary, status, hired_at, created_at, updated_at
			FROM hr.employees
			ORDER BY id ASC
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
			var e hr.Employee
			if err := rows.Scan(
				&e.ID, &e.PublicID, &e.OrganizationID, &e.UserID, &e.EmployeeCode, &e.JobTitle,
				&e.BaseSalary, &e.VariableSalary, &e.Status, &e.HiredAt, &e.CreatedAt, &e.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &e)
		}
		return rows.Err()
	})
	return list, err
}

// SaveWorkTimes saves recurring business hours.
func (r *Repository) SaveWorkTimes(ctx context.Context, times []*hr.WorkTime) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for _, wt := range times {
			query := `
				INSERT INTO hr.work_times (
					organization_id, day_name_ar, day_name_en, open_time, close_time, is_closed, sort_order
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
				RETURNING id, public_id, created_at, updated_at;
			`
			if err := tx.QueryRow(txCtx, query,
				wt.OrganizationID, wt.DayNameAr, wt.DayNameEn, wt.OpenTime, wt.CloseTime, wt.IsClosed, wt.SortOrder,
			).Scan(&wt.ID, &wt.PublicID, &wt.CreatedAt, &wt.UpdatedAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListWorkTimes returns business hours for the active tenant.
func (r *Repository) ListWorkTimes(ctx context.Context) ([]*hr.WorkTime, error) {
	var list []*hr.WorkTime
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, day_name_ar, day_name_en, open_time, close_time, is_closed, sort_order, created_at, updated_at
			FROM hr.work_times
			ORDER BY sort_order ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var wt hr.WorkTime
			if err := rows.Scan(
				&wt.ID, &wt.PublicID, &wt.OrganizationID, &wt.DayNameAr, &wt.DayNameEn,
				&wt.OpenTime, &wt.CloseTime, &wt.IsClosed, &wt.SortOrder, &wt.CreatedAt, &wt.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &wt)
		}
		return rows.Err()
	})
	return list, err
}
