package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// GetRoleByID retrieves a role by ID along with its permissions.
func (r *Repository) GetRoleByID(ctx context.Context, id int64) (*org.Role, error) {
	var role org.Role
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, organization_id, key, name, description, is_system, created_at FROM org.roles WHERE id = $1;`
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&role.ID, &role.OrganizationID, &role.Key, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt,
		); err != nil {
			return err
		}

		permRows, err := tx.Query(txCtx, `SELECT permission_key FROM org.role_permissions WHERE role_id = $1 ORDER BY permission_key ASC;`, id)
		if err != nil {
			return err
		}
		defer permRows.Close()

		for permRows.Next() {
			var perm string
			if err := permRows.Scan(&perm); err == nil {
				role.Permissions = append(role.Permissions, perm)
			}
		}
		return permRows.Err()
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperr.NotFound("role")
		}
		return nil, err
	}
	return &role, nil
}

// UpdateRole updates an existing role's name, description, and permissions.
func (r *Repository) UpdateRole(ctx context.Context, role *org.Role) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.roles
			SET name = $1, description = $2
			WHERE id = $3;
		`
		tag, err := tx.Exec(txCtx, query, role.Name, role.Description, role.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("role")
		}

		if _, err := tx.Exec(txCtx, `DELETE FROM org.role_permissions WHERE role_id = $1;`, role.ID); err != nil {
			return err
		}

		for _, perm := range role.Permissions {
			if perm != "" {
				queryPerm := `INSERT INTO org.role_permissions (role_id, permission_key) VALUES ($1, $2) ON CONFLICT DO NOTHING;`
				if _, err := tx.Exec(txCtx, queryPerm, role.ID, perm); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DeleteRole removes a custom role.
func (r *Repository) DeleteRole(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `DELETE FROM org.roles WHERE id = $1 AND is_system = false;`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("role")
		}
		return nil
	})
}
