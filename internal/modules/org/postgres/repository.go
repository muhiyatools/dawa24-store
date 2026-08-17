package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements org.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new organization repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateOrganization inserts a new organization record.
func (r *Repository) CreateOrganization(ctx context.Context, o *org.Organization) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.organizations (
				name, legal_name, trade_name, tax_number, commercial_register, type, status, credit_limit, payment_terms_days
			) VALUES (
				jsonb_build_object('ar', $1::text, 'en', $1::text),
				-- trade_name is optional on the domain type but NOT NULL in the
				-- schema, so an organisation registered without one marshals to
				-- NULL and fails the insert. Falling back to the legal name is
				-- what a reader of the record would expect anyway.
				$1, COALESCE($2, jsonb_build_object('ar', $1::text, 'en', $1::text)),
				$3, $4, $5, $6, $7, $8
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			o.LegalName, o.TradeName, o.TaxNumber, o.CommercialRegister,
			string(o.Type), string(o.Status), o.CreditLimit, o.PaymentTermsDays,
		).Scan(&o.ID, &o.PublicID, &o.CreatedAt, &o.UpdatedAt)
	})
}

// GetOrganizationByID retrieves an organization by ID.
func (r *Repository) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	var o org.Organization
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, legal_name, trade_name, tax_number, commercial_register,
			       type, status, credit_limit, payment_terms_days, created_at, updated_at
			FROM org.organizations
			WHERE id = $1;
		`
		var typeStr, statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
			&typeStr, &statusStr, &o.CreditLimit, &o.PaymentTermsDays, &o.CreatedAt, &o.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("organization")
			}
			return err
		}
		o.Type = org.OrganizationType(typeStr)
		o.Status = org.OrganizationStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// UpdateOrganizationStatus modifies organization approval state.
func (r *Repository) UpdateOrganizationStatus(ctx context.Context, id int64, status org.OrganizationStatus) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.organizations SET status = $1, updated_at = now() WHERE id = $2;`
		tag, err := tx.Exec(txCtx, query, string(status), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}
		return nil
	})
}

// ListOrganizations returns filtered organizations.
func (r *Repository) ListOrganizations(
	ctx context.Context,
	orgType *org.OrganizationType,
	status *org.OrganizationStatus,
	limit, offset int,
) ([]*org.Organization, error) {
	var list []*org.Organization
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, legal_name, trade_name, tax_number, commercial_register,
			       type, status, credit_limit, payment_terms_days, created_at, updated_at
			FROM org.organizations
			WHERE ($1::text IS NULL OR type = $1)
			  AND ($2::text IS NULL OR status = $2)
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4;
		`
		var typeStr, statusStr *string
		if orgType != nil {
			s := string(*orgType)
			typeStr = &s
		}
		if status != nil {
			s := string(*status)
			statusStr = &s
		}
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, typeStr, statusStr, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o org.Organization
			var tStr, sStr string
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
				&tStr, &sStr, &o.CreditLimit, &o.PaymentTermsDays, &o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			o.Type = org.OrganizationType(tStr)
			o.Status = org.OrganizationStatus(sStr)
			list = append(list, &o)
		}
		return rows.Err()
	})
	return list, err
}

// CreateBranch inserts a branch.
func (r *Repository) CreateBranch(ctx context.Context, b *org.Branch) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.branches (organization_id, name, code, address, city_id, is_main, phone)
			VALUES ($1, COALESCE($2, '{"ar":"الفرع","en":"Branch"}'::jsonb), NULLIF($3, ''), $4, $5, $6, $7)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			b.OrganizationID, b.Name, b.Code, b.Address, b.CityID, b.IsMain, b.Phone,
		).Scan(&b.ID, &b.PublicID, &b.CreatedAt, &b.UpdatedAt)
	})
}

// UnsetMainBranches unsets is_main on all branches for an organization.
func (r *Repository) UnsetMainBranches(ctx context.Context, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.branches SET is_main = false WHERE organization_id = $1;`
		_, err := tx.Exec(txCtx, query, orgID)
		return err
	})
}

// GetBranchByID retrieves a branch by ID.
func (r *Repository) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	var b org.Branch
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, name,
			       -- code carries a unique index, so it stays nullable: NULLs do not
			       -- collide but empty strings would, and most branches have no code.
			       -- COALESCE keeps the Go field a plain string without that risk.
			       COALESCE(code, ''), address, city_id, is_main, phone, created_at, updated_at
			FROM org.branches WHERE id = $1;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&b.ID, &b.PublicID, &b.OrganizationID, &b.Name, &b.Code, &b.Address, &b.CityID, &b.IsMain, &b.Phone, &b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("branch")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBranchesByOrg returns all branches for an organization.
func (r *Repository) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	var list []*org.Branch
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, name,
			       -- code carries a unique index, so it stays nullable: NULLs do not
			       -- collide but empty strings would, and most branches have no code.
			       -- COALESCE keeps the Go field a plain string without that risk.
			       COALESCE(code, ''), address, city_id, is_main, phone, created_at, updated_at
			FROM org.branches WHERE organization_id = $1 ORDER BY is_main DESC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b org.Branch
			if err := rows.Scan(
				&b.ID, &b.PublicID, &b.OrganizationID, &b.Name, &b.Code, &b.Address, &b.CityID, &b.IsMain, &b.Phone, &b.CreatedAt, &b.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &b)
		}
		return rows.Err()
	})
	return list, err
}

// AddMember adds a user to an organization with a role.
func (r *Repository) AddMember(ctx context.Context, m *org.Member) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.members (organization_id, user_id, role_id, role_key, is_active)
			VALUES ($1, $2, $3, COALESCE(NULLIF($5, ''), 'org_employee'), $4)
			ON CONFLICT (organization_id, user_id) DO UPDATE SET role_id = EXCLUDED.role_id, role_key = EXCLUDED.role_key, is_active = EXCLUDED.is_active, updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, m.OrganizationID, m.UserID, m.RoleID, m.IsActive, m.RoleKey).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	})
}

// ListMembersByOrg returns members of an organization.
func (r *Repository) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	var list []*org.Member
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, organization_id, user_id, role_id, role_key, is_active, created_at, updated_at FROM org.members WHERE organization_id = $1;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m org.Member
			if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.RoleID, &m.RoleKey, &m.IsActive, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// RemoveMember removes a user from an organization.
func (r *Repository) RemoveMember(ctx context.Context, orgID, userID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM org.members WHERE organization_id = $1 AND user_id = $2;`
		_, err := tx.Exec(txCtx, query, orgID, userID)
		return err
	})
}

// AddReview adds a review for an organization.
func (r *Repository) AddReview(ctx context.Context, rev *org.Review) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.organization_reviews (organization_id, user_id, rating, review_text, is_approved)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, rev.OrganizationID, rev.UserID, rev.Rating, rev.ReviewText, rev.IsApproved).
			Scan(&rev.ID, &rev.PublicID, &rev.CreatedAt, &rev.UpdatedAt)
	})
}

// ListReviewsByOrg returns approved reviews for an organization.
func (r *Repository) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	var list []*org.Review
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, rating, review_text, is_approved, created_at, updated_at
			FROM org.organization_reviews WHERE organization_id = $1 AND is_approved = true ORDER BY created_at DESC LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rev org.Review
			var revText *string
			if err := rows.Scan(&rev.ID, &rev.PublicID, &rev.OrganizationID, &rev.UserID, &rev.Rating, &revText, &rev.IsApproved, &rev.CreatedAt, &rev.UpdatedAt); err != nil {
				return err
			}
			if revText != nil {
				rev.ReviewText = *revText
			}
			list = append(list, &rev)
		}
		return rows.Err()
	})
	return list, err
}

// ToggleFollower toggles follower status for a user and organization.
func (r *Repository) ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error) {
	var following bool
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		queryCheck := `DELETE FROM org.organization_followers WHERE organization_id = $1 AND user_id = $2;`
		tag, err := tx.Exec(txCtx, queryCheck, orgID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			following = false
			return nil
		}
		queryInsert := `INSERT INTO org.organization_followers (organization_id, user_id) VALUES ($1, $2);`
		_, err = tx.Exec(txCtx, queryInsert, orgID, userID)
		following = (err == nil)
		return err
	})
	return following, err
}

// IsFollowing checks if a user follows an organization.
func (r *Repository) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	var exists bool
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT EXISTS(SELECT 1 FROM org.organization_followers WHERE organization_id = $1 AND user_id = $2);`
		return tx.QueryRow(txCtx, query, orgID, userID).Scan(&exists)
	})
	return exists, err
}

// CreatePolicy creates an organization policy.
func (r *Repository) CreatePolicy(ctx context.Context, p *org.Policy) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.organization_policies (organization_id, title, content, policy_type, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, p.OrganizationID, p.Title, p.Content, p.PolicyType, p.IsActive).
			Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
	})
}

// ListPoliciesByOrg lists policies for an organization.
func (r *Repository) ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*org.Policy, error) {
	var list []*org.Policy
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, organization_id, title, content, policy_type, is_active, created_at, updated_at FROM org.organization_policies WHERE organization_id = $1 AND is_active = true;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p org.Policy
			if err := rows.Scan(&p.ID, &p.PublicID, &p.OrganizationID, &p.Title, &p.Content, &p.PolicyType, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// CountOrganizations returns how many organizations match the filter.
//
// The admin dashboard previously derived this from len() of a page capped at
// 100 rows, so every figure on it stopped counting at 100 and quietly
// under-reported from the hundred-and-first organization onward. A count
// belongs in SQL.
func (r *Repository) CountOrganizations(
	ctx context.Context,
	orgType *org.OrganizationType,
	status *org.OrganizationStatus,
) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*)
			FROM org.organizations
			WHERE ($1::text IS NULL OR type = $1)
			  AND ($2::text IS NULL OR status = $2);
		`
		var typeStr, statusStr *string
		if orgType != nil {
			s := string(*orgType)
			typeStr = &s
		}
		if status != nil {
			s := string(*status)
			statusStr = &s
		}
		return tx.QueryRow(txCtx, query, typeStr, statusStr).Scan(&total)
	})
	return total, err
}
