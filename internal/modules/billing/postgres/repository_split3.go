package postgres

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateDepositRequest inserts a new deposit request with pending status.
func (r *Repository) CreateDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.wallet_deposits (
				wallet_id, user_id, organization_id, amount, currency, payment_method, 
				reference_number, attachment_url, user_notes, status
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
			RETURNING id, public_id, status, created_at, updated_at;
		`
		var statusStr string
		if err := tx.QueryRow(
			txCtx, query,
			dep.WalletID, dep.UserID, dep.OrganizationID, dep.Amount, dep.Currency,
			dep.PaymentMethod, dep.ReferenceNumber, dep.AttachmentURL, dep.UserNotes,
		).Scan(&dep.ID, &dep.PublicID, &statusStr, &dep.CreatedAt, &dep.UpdatedAt); err != nil {
			return fmt.Errorf("create deposit request: %w", err)
		}
		dep.Status = billing.DepositStatus(statusStr)
		return nil
	})
}

// GetDepositRequestByID retrieves a deposit request by ID.
func (r *Repository) GetDepositRequestByID(ctx context.Context, id int64) (*billing.WalletDeposit, error) {
	var dep billing.WalletDeposit
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payment_method, reference_number, COALESCE(attachment_url, ''), COALESCE(user_notes, ''),
			       status, COALESCE(rejection_reason, ''), reviewed_by, reviewed_at, transaction_id,
			       created_at, updated_at
			FROM billing.wallet_deposits
			WHERE id = $1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&dep.ID, &dep.PublicID, &dep.WalletID, &dep.UserID, &dep.OrganizationID, &dep.Amount, &dep.Currency,
			&dep.PaymentMethod, &dep.ReferenceNumber, &dep.AttachmentURL, &dep.UserNotes,
			&statusStr, &dep.RejectionReason, &dep.ReviewedBy, &dep.ReviewedAt, &dep.TransactionID,
			&dep.CreatedAt, &dep.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("deposit_request")
			}
			return err
		}
		dep.Status = billing.DepositStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &dep, nil
}

// UpdatePendingDepositRequest updates the parameters of a pending deposit request.
func (r *Repository) UpdatePendingDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE billing.wallet_deposits
			SET amount = $1, payment_method = $2, reference_number = $3,
			    attachment_url = CASE WHEN $4 <> '' THEN $4 ELSE attachment_url END,
			    user_notes = $5, updated_at = now()
			WHERE id = $6 AND user_id = $7 AND status = 'pending'
			RETURNING updated_at;
		`
		res, err := tx.Exec(
			txCtx, query,
			dep.Amount, dep.PaymentMethod, dep.ReferenceNumber,
			dep.AttachmentURL, dep.UserNotes, dep.ID, dep.UserID,
		)
		if err != nil {
			return fmt.Errorf("update pending deposit: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.Validation("deposit.not_editable", i18n.TDefault("w4_mod.w4str_77_77"), nil)
		}
		return nil
	})
}

// ListDepositRequestsByUser lists all deposit requests submitted by a user.
func (r *Repository) ListDepositRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*billing.WalletDeposit, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []*billing.WalletDeposit
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payment_method, reference_number, COALESCE(attachment_url, ''), COALESCE(user_notes, ''),
			       status, COALESCE(rejection_reason, ''), reviewed_by, reviewed_at, transaction_id,
			       created_at, updated_at
			FROM billing.wallet_deposits
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, userID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var dep billing.WalletDeposit
			var statusStr string
			if err := rows.Scan(
				&dep.ID, &dep.PublicID, &dep.WalletID, &dep.UserID, &dep.OrganizationID, &dep.Amount, &dep.Currency,
				&dep.PaymentMethod, &dep.ReferenceNumber, &dep.AttachmentURL, &dep.UserNotes,
				&statusStr, &dep.RejectionReason, &dep.ReviewedBy, &dep.ReviewedAt, &dep.TransactionID,
				&dep.CreatedAt, &dep.UpdatedAt,
			); err != nil {
				return err
			}
			dep.Status = billing.DepositStatus(statusStr)
			list = append(list, &dep)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}
