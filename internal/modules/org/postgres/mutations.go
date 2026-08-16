package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// UpdateOrganization updates organization profile fields.
func (r *Repository) UpdateOrganization(ctx context.Context, o *org.Organization) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.organizations
			SET legal_name = $1, trade_name = $2, tax_number = $3, commercial_register = $4,
			    credit_limit = $5, payment_terms_days = $6, updated_at = now()
			WHERE id = $7;
		`
		tag, err := tx.Exec(txCtx, query,
			o.LegalName, o.TradeName, o.TaxNumber, o.CommercialRegister,
			o.CreditLimit, o.PaymentTermsDays, o.ID,
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

// DeleteOrganization soft-deletes or suspends an organization.
func (r *Repository) DeleteOrganization(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `UPDATE org.organizations SET status = 'suspended', updated_at = now() WHERE id = $1;`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("organization")
		}
		return nil
	})
}

// UpdateBranch updates branch information.
func (r *Repository) UpdateBranch(ctx context.Context, b *org.Branch) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.branches
			SET name = $1, code = $2, phone = $3, address = $4,
			    city_id = $5, is_main = $6, updated_at = now()
			WHERE id = $7 AND organization_id = $8;
		`
		tag, err := tx.Exec(txCtx, query,
			b.Name, b.Code, b.Phone, b.Address,
			b.CityID, b.IsMain, b.ID, b.OrganizationID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("branch")
		}
		return nil
	})
}

// DeleteBranch removes a branch.
func (r *Repository) DeleteBranch(ctx context.Context, id, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `DELETE FROM org.branches WHERE id = $1 AND organization_id = $2;`, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("branch")
		}
		return nil
	})
}

// UpdateMemberRole changes a member's role in the organization.
func (r *Repository) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `UPDATE org.members SET role_key = $1 WHERE organization_id = $2 AND user_id = $3;`, role, orgID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("org_member")
		}
		return nil
	})
}
