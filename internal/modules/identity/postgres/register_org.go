package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// RegisterOrganization creates a user, organization, owner membership and main
// branch in one transaction.
//
// This is the front door of the marketplace. Registration previously called
// CreateUser alone with role 'customer', so every signup landed with no
// organization and every tenant-scoped screen rendered empty. The account type
// chosen on the form is honoured here: the platform role on identity.users
// stays 'customer' (low privilege), while the organization membership carries
// role_key 'org_owner' — which the permission resolver reads.
//
// The transaction spans the identity and org schemas because the two facts
// (this person, this tenant) must commit or roll back together. A failure
// halfway leaves no orphan user and no orphan organization.
func (r *Repository) RegisterOrganization(ctx context.Context, u *identity.User, orgIn identity.RegisterOrgInput) (*identity.RegisterOrgResult, error) {
	result := &identity.RegisterOrgResult{}

	if orgIn.Type == "individual" {
		u.Role = "individual"
		if orgIn.LegalName == "" {
			orgIn.LegalName = u.Name.Get(i18n.AR)
			if orgIn.LegalName == "" {
				orgIn.LegalName = u.Email
			}
		}
		if orgIn.TradeNameAr == "" {
			orgIn.TradeNameAr = orgIn.LegalName
		}
	}

	// trade_name is NOT NULL in the schema; an empty trade name falls back to
	// the legal name, matching CreateOrganization's reader-facing behaviour.
	tradeName := i18n.New(orgIn.TradeNameAr, orgIn.TradeNameEn)
	if orgIn.TradeNameAr == "" && orgIn.TradeNameEn == "" {
		tradeName = i18n.New(orgIn.LegalName, orgIn.LegalName)
	}

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. The user, with the low platform role.
		err := tx.QueryRow(txCtx, `INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone, phone)`+
			`VALUES ($1, $2, COALESCE($3, '{"ar":"","en":""}'::jsonb), $4, $5, $6, $7, $8)`+
			`RETURNING id, public_id, created_at, updated_at;`,
			identity.NormalizeEmail(u.Email), u.PasswordHash, u.Name, u.Role,
			string(u.Status), string(u.Language), u.Timezone, u.Phone,
		).Scan(&u.ID, &u.PublicID, &u.CreatedAt, &u.UpdatedAt)
		if err != nil {
			if database.IsUniqueViolation(err) {
				return apperr.Conflict("user.email_exists", "An account with this email already exists.")
			}
			return fmt.Errorf("identity postgres: register user: %w", err)
		}

		// Login security tracking.
		if _, err := tx.Exec(txCtx, `INSERT INTO identity.user_security (user_id, login_attempts) VALUES ($1, 0) ON CONFLICT (user_id) DO NOTHING;`, u.ID); err != nil {
			return fmt.Errorf("identity postgres: register security: %w", err)
		}

		// 2. The organization (individual accounts are active by default).
		orgStatus := "pending"
		cr := orgIn.CommercialRegister
		if orgIn.Type == "individual" {
			orgStatus = "active"
			if cr == "" {
				cr = fmt.Sprintf("IND-%d", u.ID)
			}
		}

		var status string
		err = tx.QueryRow(txCtx, `INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status, pharmacist_license, license_document_url, branch_count, owner_id)`+
			`VALUES (jsonb_build_object('ar', $1::text, 'en', $1::text), $1, $2, NULLIF($3, ''), $4, $5, $6, NULLIF($7, ''), $8, $9, $10)`+
			`RETURNING id, public_id, type, status;`,
			orgIn.LegalName, tradeName, orgIn.TaxNumber, cr,
			orgIn.Type, orgStatus, orgIn.PharmacistLicense, orgIn.LicenseDocumentURL, orgIn.BranchCount, u.ID,
		).Scan(&result.OrganizationID, &result.OrganizationPublicID, &result.OrganizationType, &status)
		if err != nil {
			if database.IsUniqueViolation(err) {
				return apperr.Conflict("org.commercial_register_exists", "An organization with this commercial registration already exists.")
			}
			return fmt.Errorf("identity postgres: register organization: %w", err)
		}
		result.OrganizationStatus = status

		// 3. The owner membership. role_id stays NULL; role_key carries the
		// authorization decision and is what the permission resolver reads.
		if _, err := tx.Exec(txCtx, `INSERT INTO org.members (organization_id, user_id, role_key, status) VALUES ($1, $2, 'org_owner', 'active');`, result.OrganizationID, u.ID); err != nil {
			return fmt.Errorf("identity postgres: register owner membership: %w", err)
		}

		// 4. The main branch. city_id is the only registration form field that
		// belongs on the branch rather than the organization.
		if _, err := tx.Exec(txCtx, `INSERT INTO org.branches (organization_id, name, city_id, is_main) VALUES ($1, $2, $3, true);`, result.OrganizationID, tradeName, orgIn.CityID); err != nil {
			return fmt.Errorf("identity postgres: register main branch: %w", err)
		}

		// 5. Audit the privileged creation in the same transaction.
		if err := database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &result.OrganizationID,
			ActorUserID:    u.ID,
			Action:         "org.registered",
			EntityType:     "organization",
			EntityID:       result.OrganizationPublicID,
			After: map[string]any{
				"type": result.OrganizationType,
			},
		}); err != nil {
			return fmt.Errorf("identity postgres: audit registration: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
