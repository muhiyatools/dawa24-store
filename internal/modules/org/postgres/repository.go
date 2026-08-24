package postgres

import (
	"context"
	"math"

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
			       COALESCE(pharmacist_license, ''),
			       COALESCE(verification_notes, ''), COALESCE(rejection_reason, ''),
			       COALESCE(owner_id, 0), approved_at, approved_by,
			       COALESCE(ai_virtual_key, ''), COALESCE(ai_user_id, ''),
			       type, status, credit_limit, payment_terms_days, created_at, updated_at
			FROM org.organizations
			WHERE id = $1;
		`
		var typeStr, statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
			&o.PharmacistLicense, &o.VerificationNotes, &o.RejectionReason,
			&o.OwnerID, &o.ApprovedAt, &o.ApprovedBy,
			&o.AIVirtualKey, &o.AIUserID,
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

// GetSupplierProfile retrieves full commercial details of a supplier/vendor organization.
func (r *Repository) GetSupplierProfile(ctx context.Context, id int64) (*org.SupplierOrgProfile, error) {
	var p org.SupplierOrgProfile
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT 
				id, public_id,
				COALESCE(name->>'ar', name->>'en', ''),
				COALESCE(name->>'en', name->>'ar', ''),
				COALESCE(type, 'company'),
				COALESCE(min_order_price, 10.00),
				COALESCE(max_order_price, 50.00),
				COALESCE(organization_number, ''),
				COALESCE(email, ''),
				COALESCE(phone, ''),
				COALESCE(tax_number, ''),
				COALESCE(address, ''),
				COALESCE(description->>'ar', ''),
				COALESCE(description->>'en', ''),
				COALESCE(image, ''),
				COALESCE(coverage_image, ''),
				COALESCE(status, 'approved'),
				COALESCE(rating, 5),
				created_at, updated_at
			FROM org.organizations
			WHERE id = $1 AND deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.NameAr, &p.NameEn, &p.Type,
			&p.MinOrderPrice, &p.MaxOrderPrice,
			&p.OrganizationNumber, &p.Email, &p.Phone, &p.TaxNumber, &p.Address,
			&p.DescriptionAr, &p.DescriptionEn,
			&p.Image, &p.CoverageImage,
			&p.Status, &p.Rating,
			&p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("organization")
		}
		return nil, err
	}
	return &p, nil
}

// UpdateSupplierProfile updates the commercial details and order price limits of a supplier/vendor organization.
func (r *Repository) UpdateSupplierProfile(ctx context.Context, p *org.SupplierOrgProfile) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.organizations
			SET 
				name = jsonb_build_object('ar', $1::text, 'en', $2::text),
				type = $3,
				min_order_price = $4,
				max_order_price = $5,
				organization_number = $6,
				email = $7,
				phone = $8,
				tax_number = $9,
				address = $10,
				description = jsonb_build_object('ar', $11::text, 'en', $12::text),
				image = CASE WHEN $13::text <> '' THEN $13::text ELSE image END,
				coverage_image = CASE WHEN $14::text <> '' THEN $14::text ELSE coverage_image END,
				updated_at = now()
			WHERE id = $15 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, query,
			p.NameAr, p.NameEn, p.Type,
			p.MinOrderPrice, p.MaxOrderPrice,
			p.OrganizationNumber, p.Email, p.Phone, p.TaxNumber, p.Address,
			p.DescriptionAr, p.DescriptionEn,
			p.Image, p.CoverageImage,
			p.ID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}
		return nil
	})
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

// UpdateOrganizationAICredentials saves the AI Gateway user ID and virtual API key for an organization.
func (r *Repository) UpdateOrganizationAICredentials(ctx context.Context, id int64, aiUserID, aiVirtualKey string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.organizations SET ai_user_id = $1, ai_virtual_key = $2, updated_at = now() WHERE id = $3;`
		tag, err := tx.Exec(txCtx, query, aiUserID, aiVirtualKey, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}
		return nil
	})
}

// ReviewOrganization updates the approval status, admin notes, rejection reason, and audit info.
func (r *Repository) ReviewOrganization(ctx context.Context, id int64, status org.OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var approvedAt *string
		var adminPtr *int64
		if adminID > 0 {
			adminPtr = &adminID
		}
		query := `
			UPDATE org.organizations 
			SET status = $1, 
			    verification_notes = $2, 
			    rejection_reason = $3,
			    approved_at = CASE WHEN $1 = 'approved' THEN NOW() ELSE approved_at END,
			    approved_by = CASE WHEN $1 = 'approved' THEN $4 ELSE approved_by END,
			    updated_at = NOW()
			WHERE id = $5;
		`
		_ = approvedAt
		tag, err := tx.Exec(txCtx, query, string(status), notes, rejectionReason, adminPtr, id)
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
			SELECT id, public_id, COALESCE(NULLIF(legal_name, ''), NULLIF(name->>'ar', ''), NULLIF(trade_name->>'ar', ''), ''), trade_name, tax_number, commercial_register,
			       COALESCE(pharmacist_license, ''),
			       COALESCE(verification_notes, ''), COALESCE(rejection_reason, ''),
			       COALESCE(owner_id, 0), approved_at, approved_by,
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
				&o.PharmacistLicense, &o.VerificationNotes, &o.RejectionReason,
				&o.OwnerID, &o.ApprovedAt, &o.ApprovedBy,
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
			INSERT INTO org.branches (
				organization_id, name, code, address, city_id, latitude, longitude,
				google_maps_url, manager_id, manager_name, warehouse_type, has_cold_storage,
				capacity_sqm, operating_hours, status, is_main, phone
			) VALUES (
				$1, COALESCE($2, '{"ar":"الفرع","en":"Branch"}'::jsonb), NULLIF($3, ''), $4, $5, $6, $7,
				COALESCE($8, ''), $9, COALESCE($10, ''), COALESCE(NULLIF($11, ''), 'warehouse'), COALESCE($12, false),
				COALESCE($13, 0), COALESCE($14, ''), COALESCE(NULLIF($15, ''), 'active'), $16, $17
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query,
			b.OrganizationID, b.Name, b.Code, b.Address, b.CityID, b.Latitude, b.Longitude,
			b.GoogleMapsURL, b.ManagerID, b.ManagerName, b.WarehouseType, b.HasColdStorage,
			b.CapacitySQM, b.OperatingHours, b.Status, b.IsMain, b.Phone,
		).Scan(&b.ID, &b.PublicID, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return err
		}

		if len(b.InstitutionalWorks) > 0 {
			for _, cat := range b.InstitutionalWorks {
				if cat != "" {
					_, _ = tx.Exec(txCtx, `
						INSERT INTO org.branch_institutional_works (branch_id, work_category)
						VALUES ($1, $2) ON CONFLICT DO NOTHING;
					`, b.ID, cat)
				}
			}
		}
		return nil
	})
}

// UpdateBranch updates an existing branch.
func (r *Repository) UpdateBranch(ctx context.Context, b *org.Branch) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.branches
			SET name = COALESCE($1, name),
			    code = COALESCE(NULLIF($2, ''), code),
			    address = COALESCE($3, address),
			    city_id = $4,
			    latitude = $5,
			    longitude = $6,
			    google_maps_url = COALESCE($7, google_maps_url),
			    manager_id = $8,
			    manager_name = COALESCE($9, manager_name),
			    warehouse_type = COALESCE(NULLIF($10, ''), warehouse_type),
			    has_cold_storage = COALESCE($11, has_cold_storage),
			    capacity_sqm = COALESCE($12, capacity_sqm),
			    operating_hours = COALESCE($13, operating_hours),
			    status = COALESCE(NULLIF($14, ''), status),
			    is_main = COALESCE($15, is_main),
			    phone = COALESCE($16, phone),
			    updated_at = now()
			WHERE id = $17 AND organization_id = $18;
		`
		tag, err := tx.Exec(txCtx, query,
			b.Name, b.Code, b.Address, b.CityID, b.Latitude, b.Longitude,
			b.GoogleMapsURL, b.ManagerID, b.ManagerName, b.WarehouseType, b.HasColdStorage,
			b.CapacitySQM, b.OperatingHours, b.Status, b.IsMain, b.Phone,
			b.ID, b.OrganizationID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("branch")
		}

		_, _ = tx.Exec(txCtx, `DELETE FROM org.branch_institutional_works WHERE branch_id = $1;`, b.ID)
		if len(b.InstitutionalWorks) > 0 {
			for _, cat := range b.InstitutionalWorks {
				if cat != "" {
					_, _ = tx.Exec(txCtx, `
						INSERT INTO org.branch_institutional_works (branch_id, work_category)
						VALUES ($1, $2) ON CONFLICT DO NOTHING;
					`, b.ID, cat)
				}
			}
		}
		return nil
	})
}

// DeleteBranch soft-deletes a branch.
func (r *Repository) DeleteBranch(ctx context.Context, id, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.branches SET deleted_at = now(), status = 'inactive' WHERE id = $1 AND organization_id = $2;`
		tag, err := tx.Exec(txCtx, query, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("branch")
		}
		return nil
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

// AssignBranchManager assigns a designated manager user to a branch.
func (r *Repository) AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.branches SET manager_id = $1, updated_at = now() WHERE id = $2 AND organization_id = $3;`
		tag, err := tx.Exec(txCtx, query, managerUserID, branchID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("branch")
		}
		return nil
	})
}

// GetBranchByID retrieves a branch by ID with manager info.
func (r *Repository) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	var b org.Branch
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT b.id, b.public_id, b.organization_id, b.name,
			       COALESCE(b.code, ''), b.address, b.city_id, b.latitude, b.longitude,
			       COALESCE(b.google_maps_url, ''), b.manager_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), b.manager_name, ''),
			       COALESCE(u.email, ''), COALESCE(u.phone, ''),
			       COALESCE(b.warehouse_type, 'warehouse'), COALESCE(b.has_cold_storage, false),
			       COALESCE(b.capacity_sqm, 0), COALESCE(b.operating_hours, ''),
			       COALESCE(b.status, 'active'), b.is_main, COALESCE(b.phone, ''), b.created_at, b.updated_at
			FROM org.branches b
			LEFT JOIN identity.users u ON u.id = b.manager_id
			WHERE b.id = $1 AND b.deleted_at IS NULL;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&b.ID, &b.PublicID, &b.OrganizationID, &b.Name,
			&b.Code, &b.Address, &b.CityID, &b.Latitude, &b.Longitude,
			&b.GoogleMapsURL, &b.ManagerID, &b.ManagerName,
			&b.ManagerEmail, &b.ManagerPhone,
			&b.WarehouseType, &b.HasColdStorage,
			&b.CapacitySQM, &b.OperatingHours,
			&b.Status, &b.IsMain, &b.Phone, &b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("branch")
			}
			return err
		}

		iwRows, _ := tx.Query(txCtx, `SELECT work_category FROM org.branch_institutional_works WHERE branch_id = $1`, b.ID)
		if iwRows != nil {
			for iwRows.Next() {
				var cat string
				if err := iwRows.Scan(&cat); err == nil {
					b.InstitutionalWorks = append(b.InstitutionalWorks, cat)
				}
			}
			iwRows.Close()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBranchesByOrg returns all active branches for an organization.
func (r *Repository) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	var list []*org.Branch
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT b.id, b.public_id, b.organization_id, b.name,
			       COALESCE(b.code, ''), b.address, b.city_id, b.latitude, b.longitude,
			       COALESCE(b.google_maps_url, ''), b.manager_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), b.manager_name, ''),
			       COALESCE(u.email, ''), COALESCE(u.phone, ''),
			       COALESCE(b.warehouse_type, 'warehouse'), COALESCE(b.has_cold_storage, false),
			       COALESCE(b.capacity_sqm, 0), COALESCE(b.operating_hours, ''),
			       COALESCE(b.status, 'active'), b.is_main, COALESCE(b.phone, ''), b.created_at, b.updated_at
			FROM org.branches b
			LEFT JOIN identity.users u ON u.id = b.manager_id
			WHERE ($1::bigint = 0 OR b.organization_id = $1) AND b.deleted_at IS NULL
			ORDER BY b.is_main DESC, b.id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b org.Branch
			if err := rows.Scan(
				&b.ID, &b.PublicID, &b.OrganizationID, &b.Name,
				&b.Code, &b.Address, &b.CityID, &b.Latitude, &b.Longitude,
				&b.GoogleMapsURL, &b.ManagerID, &b.ManagerName,
				&b.ManagerEmail, &b.ManagerPhone,
				&b.WarehouseType, &b.HasColdStorage,
				&b.CapacitySQM, &b.OperatingHours,
				&b.Status, &b.IsMain, &b.Phone, &b.CreatedAt, &b.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &b)
		}

		for _, b := range list {
			iwRows, _ := tx.Query(txCtx, `SELECT work_category FROM org.branch_institutional_works WHERE branch_id = $1`, b.ID)
			if iwRows != nil {
				for iwRows.Next() {
					var cat string
					if err := iwRows.Scan(&cat); err == nil {
						b.InstitutionalWorks = append(b.InstitutionalWorks, cat)
					}
				}
				iwRows.Close()
			}
		}
		return rows.Err()
	})
	return list, err
}

// ListEmployees returns comprehensive employee rows with user, role, and branch details.
func (r *Repository) ListEmployees(ctx context.Context, orgID int64) ([]*org.EmployeeView, error) {
	var list []*org.EmployeeView
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {

		query := `
			SELECT m.id, m.organization_id, m.user_id, m.branch_id, m.role_id, m.role_key,
			       m.org_role_id, COALESCE(m.employee_code, ''), COALESCE(m.job_title, ''),
			       m.base_salary, m.variable_salary, m.is_active, m.created_at, m.updated_at,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email),
			       COALESCE(u.email, ''), COALESCE(u.phone, ''), COALESCE(u.status, 'active'),
			       COALESCE(NULLIF(r.name->>'ar', ''), NULLIF(r.name->>'en', ''), NULLIF(ir.name->>'ar', ''), NULLIF(ir.name->>'en', ''), m.role_key),
			       COALESCE(b.name->>'ar', b.name->>'en', ''),
			       CASE WHEN b.manager_id = m.user_id THEN true ELSE false END AS is_manager
			FROM org.members m
			JOIN identity.users u ON u.id = m.user_id
			LEFT JOIN org.roles r ON r.id = m.org_role_id
			LEFT JOIN identity.roles ir ON ir.key = m.role_key
			LEFT JOIN org.branches b ON b.id = m.branch_id
			WHERE m.organization_id = $1
			ORDER BY m.id DESC;

		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m org.Member
			var roleID *int64
			var userName, userEmail, userPhone, userStatus, roleName, branchName string
			var isManager bool

			if err := rows.Scan(
				&m.ID, &m.OrganizationID, &m.UserID, &m.BranchID, &roleID, &m.RoleKey,
				&m.OrgRoleID, &m.EmployeeCode, &m.JobTitle,
				&m.BaseSalary, &m.VariableSalary, &m.IsActive, &m.CreatedAt, &m.UpdatedAt,
				&userName, &userEmail, &userPhone, &userStatus,
				&roleName, &branchName, &isManager,
			); err != nil {
				return err
			}
			if roleID != nil {
				m.RoleID = *roleID
			}

			list = append(list, &org.EmployeeView{
				Member:     &m,
				UserName:   userName,
				UserEmail:  userEmail,
				UserPhone:  userPhone,
				UserStatus: userStatus,
				RoleName:   roleName,
				BranchName: branchName,
				IsManager:  isManager,
			})
		}
		return rows.Err()
	})
	return list, err
}

// AddMember adds a user to an organization with full employee attributes.
func (r *Repository) AddMember(ctx context.Context, m *org.Member) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.members (
				organization_id, user_id, branch_id, role_id, role_key,
				employee_code, job_title, base_salary, variable_salary, is_active
			) VALUES (
				$1, $2, $3, $4, COALESCE(NULLIF($5, ''), 'org_employee'),
				NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10
			)
			ON CONFLICT (organization_id, user_id) DO UPDATE
			SET branch_id = COALESCE(EXCLUDED.branch_id, org.members.branch_id),
			    role_id = EXCLUDED.role_id,
			    role_key = EXCLUDED.role_key,
			    employee_code = COALESCE(NULLIF(EXCLUDED.employee_code, ''), org.members.employee_code),
			    job_title = COALESCE(NULLIF(EXCLUDED.job_title, ''), org.members.job_title),
			    base_salary = CASE WHEN EXCLUDED.base_salary > 0 THEN EXCLUDED.base_salary ELSE org.members.base_salary END,
			    is_active = EXCLUDED.is_active,
			    updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		var roleID *int64
		if m.RoleID > 0 {
			roleID = &m.RoleID
		}
		return tx.QueryRow(txCtx, query,
			m.OrganizationID, m.UserID, m.BranchID, roleID, m.RoleKey,
			m.EmployeeCode, m.JobTitle, m.BaseSalary, m.VariableSalary, m.IsActive,
		).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	})
}

// ToggleMemberStatus toggles a member's active state.
func (r *Repository) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.members SET is_active = NOT is_active, updated_at = now() WHERE id = $1 AND organization_id = $2;`
		tag, err := tx.Exec(txCtx, query, memberID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("member")
		}
		return nil
	})
}

// ListMembersByOrg returns members of an organization.
func (r *Repository) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	var list []*org.Member
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, organization_id, user_id, role_id, role_key, is_active, created_at, updated_at FROM org.members WHERE organization_id = $1 ORDER BY id DESC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m org.Member
			var roleID *int64
			if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &roleID, &m.RoleKey, &m.IsActive, &m.CreatedAt, &m.UpdatedAt); err != nil {
				return err
			}
			if roleID != nil {
				m.RoleID = *roleID
			}
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// RemoveMember removes a user from an organization.
func (r *Repository) RemoveMember(ctx context.Context, orgID, userID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM org.members WHERE organization_id = $1 AND user_id = $2;`
		_, err := tx.Exec(txCtx, query, orgID, userID)
		return err
	})
}

// AddReview adds a review for an organization with individual criteria ratings.
func (r *Repository) AddReview(ctx context.Context, rev *org.Review) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if len(rev.Ratings) > 0 {
			total := 0
			for _, rating := range rev.Ratings {
				total += rating.Score
			}
			rev.Rating = int(math.Round(float64(total) / float64(len(rev.Ratings))))
		}
		if rev.Rating < 1 {
			rev.Rating = 5
		}
		query := `
			INSERT INTO org.organization_reviews (organization_id, user_id, rating, review_text, is_approved)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query, rev.OrganizationID, rev.UserID, rev.Rating, rev.ReviewText, rev.IsApproved).
			Scan(&rev.ID, &rev.PublicID, &rev.CreatedAt, &rev.UpdatedAt)
		if err != nil {
			return err
		}

		for _, rr := range rev.Ratings {
			crit := rr.Criterion
			if crit == "" {
				continue
			}
			qRating := `
				INSERT INTO org.review_ratings (review_id, criterion, score)
				VALUES ($1, $2, $3)
				ON CONFLICT (review_id, criterion) DO UPDATE SET score = EXCLUDED.score;
			`
			if _, err := tx.Exec(txCtx, qRating, rev.ID, crit, rr.Score); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListReviewsByOrg returns approved reviews for an organization, joining reviewer organization name.
func (r *Repository) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	var list []*org.Review
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT r.id, r.public_id, r.organization_id, r.user_id, r.rating, r.review_text, r.is_approved, r.created_at, r.updated_at,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'ar', ''), NULLIF(o.name->>'en', ''), 'صيدلية معتمدة') AS reviewer_org_name
			FROM org.organization_reviews r
			LEFT JOIN org.members m ON m.user_id = r.user_id
			LEFT JOIN org.organizations o ON o.id = m.organization_id
			WHERE r.organization_id = $1 AND r.is_approved = true
			ORDER BY r.created_at DESC
			LIMIT $2 OFFSET $3;
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
			var orgName string
			if err := rows.Scan(&rev.ID, &rev.PublicID, &rev.OrganizationID, &rev.UserID, &rev.Rating, &revText, &rev.IsApproved, &rev.CreatedAt, &rev.UpdatedAt, &orgName); err != nil {
				return err
			}
			if revText != nil {
				rev.ReviewText = *revText
			}
			rev.ReviewerOrgName = orgName
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

// ListFollowedOrgs returns all organizations followed by a user.
func (r *Repository) ListFollowedOrgs(ctx context.Context, userID int64) ([]*org.Organization, error) {
	var orgs []*org.Organization
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT o.id, o.public_id, o.legal_name, o.trade_name, o.tax_number, o.commercial_register,
			       o.type, o.status, o.credit_limit, o.payment_terms_days, o.created_at, o.updated_at
			FROM org.organizations o
			JOIN org.organization_followers f ON o.id = f.organization_id
			WHERE f.user_id = $1
			ORDER BY f.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o org.Organization
			var typeStr, statusStr string
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
				&typeStr, &statusStr, &o.CreditLimit, &o.PaymentTermsDays, &o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			o.Type = org.OrganizationType(typeStr)
			o.Status = org.OrganizationStatus(statusStr)
			orgs = append(orgs, &o)
		}
		return rows.Err()
	})
	return orgs, err
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

// SavePolicies replaces the policies for an organization with the given set.
func (r *Repository) SavePolicies(ctx context.Context, orgID int64, policies []*org.Policy) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organization_policies WHERE organization_id = $1;`, orgID); err != nil {
			return err
		}
		for _, p := range policies {
			query := `
				INSERT INTO org.organization_policies (organization_id, title, content, policy_type, is_active)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING id, public_id, created_at, updated_at;
			`
			if p.PolicyType == "" {
				p.PolicyType = "terms"
			}
			if err := tx.QueryRow(txCtx, query, orgID, p.Title, p.Content, p.PolicyType, p.IsActive).
				Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt); err != nil {
				return err
			}
			p.OrganizationID = orgID
		}
		return nil
	})
}

// ListSocialMediaByOrg lists social media accounts for an organization.
func (r *Repository) ListSocialMediaByOrg(ctx context.Context, orgID int64) ([]*org.SocialMedia, error) {
	var list []*org.SocialMedia
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, organization_id, platform, url, created_at, updated_at FROM org.organization_social_media WHERE organization_id = $1 ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s org.SocialMedia
			if err := rows.Scan(&s.ID, &s.PublicID, &s.OrganizationID, &s.Platform, &s.URL, &s.CreatedAt, &s.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// SaveSocialMedia replaces social media channels for an organization.
func (r *Repository) SaveSocialMedia(ctx context.Context, orgID int64, links []*org.SocialMedia) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organization_social_media WHERE organization_id = $1;`, orgID); err != nil {
			return err
		}
		for _, l := range links {
			if l.Platform == "" || l.URL == "" {
				continue
			}
			query := `
				INSERT INTO org.organization_social_media (organization_id, platform, url)
				VALUES ($1, $2, $3)
				RETURNING id, public_id, created_at, updated_at;
			`
			if err := tx.QueryRow(txCtx, query, orgID, l.Platform, l.URL).
				Scan(&l.ID, &l.PublicID, &l.CreatedAt, &l.UpdatedAt); err != nil {
				return err
			}
			l.OrganizationID = orgID
		}
		return nil
	})
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

// CreateRole inserts a new per-organization role and its permissions.
func (r *Repository) CreateRole(ctx context.Context, role *org.Role) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.roles (organization_id, key, name, description, is_system)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at;
		`
		err := tx.QueryRow(txCtx, query, role.OrganizationID, role.Key, role.Name, role.Description, role.IsSystem).
			Scan(&role.ID, &role.CreatedAt)
		if err != nil {
			return err
		}

		for _, perm := range role.Permissions {
			queryPerm := `INSERT INTO org.role_permissions (role_id, permission_key) VALUES ($1, $2) ON CONFLICT DO NOTHING;`
			if _, err := tx.Exec(txCtx, queryPerm, role.ID, perm); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListRolesByOrg retrieves all roles defined for an organization.
func (r *Repository) ListRolesByOrg(ctx context.Context, orgID int64) ([]*org.Role, error) {
	var list []*org.Role
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, organization_id, key, name, description, is_system, created_at FROM org.roles WHERE organization_id = $1 ORDER BY created_at ASC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var role org.Role
			if err := rows.Scan(&role.ID, &role.OrganizationID, &role.Key, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt); err != nil {
				return err
			}
			list = append(list, &role)
		}
		return rows.Err()
	})
	return list, err
}

// GetDeliveryBands retrieves active delivery bands for an organization.
func (r *Repository) GetDeliveryBands(ctx context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	var list []*org.DeliveryBand
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, organization_id, from_meters, to_meters, fee, is_active, created_at, updated_at FROM org.delivery_bands WHERE organization_id = $1 AND is_active = true ORDER BY from_meters ASC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b org.DeliveryBand
			if err := rows.Scan(&b.ID, &b.OrganizationID, &b.FromMeters, &b.ToMeters, &b.Fee, &b.IsActive, &b.CreatedAt, &b.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &b)
		}
		return rows.Err()
	})
	return list, err
}

// SaveDeliveryBands replaces the delivery bands for an organization.
func (r *Repository) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*org.DeliveryBand) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		delQuery := `DELETE FROM org.delivery_bands WHERE organization_id = $1;`
		if _, err := tx.Exec(txCtx, delQuery, orgID); err != nil {
			return err
		}

		insertQuery := `
			INSERT INTO org.delivery_bands (organization_id, from_meters, to_meters, fee, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at, updated_at;
		`
		for _, b := range bands {
			b.OrganizationID = orgID
			err := tx.QueryRow(txCtx, insertQuery, orgID, b.FromMeters, b.ToMeters, b.Fee, b.IsActive).
				Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// GetReviewCriteria retrieves all evaluation criteria for a context (e.g. supplier, pharmacy, product).
func (r *Repository) GetReviewCriteria(ctx context.Context, contextType string) ([]*org.ReviewCriterion, error) {
	var list []*org.ReviewCriterion
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT key, name, context, weight, sort_order, is_active FROM org.review_criteria WHERE (context = $1 OR $1 = '') AND is_active = true ORDER BY sort_order ASC;`
		rows, err := tx.Query(txCtx, query, contextType)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c org.ReviewCriterion
			if err := rows.Scan(&c.Key, &c.Name, &c.Context, &c.Weight, &c.SortOrder, &c.IsActive); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// AddReviewWithRatings adds a comprehensive multi-criteria review.
func (r *Repository) AddReviewWithRatings(ctx context.Context, rev *org.Review, ratings []org.ReviewRating) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.organization_reviews (
				organization_id, user_id, order_id, product_id, title, rating, review_text, is_verified, is_public, status, context
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			rev.OrganizationID, rev.UserID, rev.OrderID, rev.ProductID,
			rev.Title, rev.Rating, rev.ReviewText, rev.IsVerified, rev.IsPublic, rev.Status, rev.Context,
		).Scan(&rev.ID, &rev.PublicID, &rev.CreatedAt, &rev.UpdatedAt)
		if err != nil {
			return err
		}

		for _, rat := range ratings {
			queryRat := `INSERT INTO org.review_ratings (review_id, criterion, score) VALUES ($1, $2, $3);`
			if _, err := tx.Exec(txCtx, queryRat, rev.ID, rat.Criterion, rat.Score); err != nil {
				return err
			}
		}

		return nil
	})
}

// ReplyToReview adds a vendor reply to a review.
func (r *Repository) ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.organization_reviews
			SET response = $1, response_at = now(), responded_by = $2, updated_at = now()
			WHERE id = $3 AND organization_id = $4;
		`
		tag, err := tx.Exec(txCtx, query, response, responderID, reviewID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("review")
		}
		return nil
	})
}
