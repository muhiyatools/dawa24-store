package postgres

import (
	"context"
	"fmt"
	"strings"

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

		// 2. The organization. Both account types wait for platform approval:
		// the documents uploaded at registration are reviewed before commerce
		// begins (Rebuild V2 §4.1).
		orgStatus := "pending"

		var status string
		err = tx.QueryRow(txCtx, `INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status, pharmacist_license, branch_count, owner_id)`+
			`VALUES (jsonb_build_object('ar', $1::text, 'en', $1::text), $1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, NULLIF($7, ''), $8, $9)`+
			`RETURNING id, public_id, type, status;`,
			orgIn.LegalName, tradeName, orgIn.TaxNumber, orgIn.CommercialRegister,
			orgIn.Type, orgStatus, orgIn.PharmacistLicense, orgIn.BranchCount, u.ID,
		).Scan(&result.OrganizationID, &result.OrganizationPublicID, &result.OrganizationType, &status)
		if err != nil {
			if database.IsUniqueViolation(err) {
				return apperr.Conflict("org.commercial_register_exists", "رقم السجل التجاري مسجل مسبقاً لمنشأة أخرى.")
			}
			return fmt.Errorf("identity postgres: register organization: %w", err)
		}
		result.OrganizationStatus = status

		// 2b. The license file uploaded at registration survives as a document
		// (Rebuild V2 §4.1) — it is reviewed but never consumed, so the admin
		// documents registry sees it after approval.
		if imgURL := strings.TrimSpace(orgIn.LicenseDocumentURL); imgURL != "" {
			if _, err := tx.Exec(txCtx,
				`INSERT INTO platform_admin.documents (organization_id, title, document_type, storage_key, status, original_name, file_url) `+
					`VALUES ($1, 'السجل التجاري / الترخيص', 'commercial_register', $2, 'pending', '', '')`,
				result.OrganizationID, imgURL,
			); err != nil {
				return fmt.Errorf("identity postgres: register license document: %w", err)
			}
		}

		// 3. The owner membership. role_id stays NULL; role_key carries the
		// authorization decision and is what the permission resolver reads.
		if _, err := tx.Exec(txCtx, `INSERT INTO org.members (organization_id, user_id, role_key, status) VALUES ($1, $2, 'org_owner', 'active');`, result.OrganizationID, u.ID); err != nil {
			return fmt.Errorf("identity postgres: register owner membership: %w", err)
		}

		// 4. Validate or fallback city_id for main branch FK to ensure zero foreign key errors
		var branchCityID *int64 = orgIn.CityID
		if branchCityID != nil {
			var cityExists bool
			_ = tx.QueryRow(txCtx, `SELECT EXISTS (SELECT 1 FROM platform_admin.cities WHERE id = $1);`, *branchCityID).Scan(&cityExists)
			if !cityExists {
				branchCityID = nil
			}
		}

		// 4b. The main branch with full address and GPS coordinates.
		if _, err := tx.Exec(txCtx, `INSERT INTO org.branches (organization_id, name, city_id, address, latitude, longitude, google_maps_url, is_main)`+
			`VALUES ($1, $2, $3, $4, $5, $6, $7, true);`,
			result.OrganizationID, tradeName, branchCityID, orgIn.Address, orgIn.Latitude, orgIn.Longitude, orgIn.GoogleMapsURL,
		); err != nil {
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
