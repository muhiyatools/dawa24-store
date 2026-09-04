package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateWithdrawalRequest inserts a new withdrawal request in pending status after validating funds.
func (r *Repository) CreateWithdrawalRequest(ctx context.Context, w *billing.WalletWithdrawal) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Check current available balance
		var currentBalance money.Amount
		queryBal := `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1 FOR UPDATE;`
		err := tx.QueryRow(txCtx, queryBal, w.WalletID).Scan(&currentBalance)
		if err != nil && !database.IsNotFound(err) {
			return fmt.Errorf("read wallet balance: %w", err)
		}
		if currentBalance.Minor() < w.Amount.Minor() {
			return apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة المتاح غير كافٍ لإتمام طلب السحب.", nil)
		}

		query := `
			INSERT INTO billing.wallet_withdrawals (
				wallet_id, user_id, organization_id, amount, currency, payout_method_type,
				destination_details, user_payment_method_id, user_notes, status
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
			RETURNING id, public_id, status, created_at, updated_at;
		`
		var statusStr string
		if err := tx.QueryRow(
			txCtx, query,
			w.WalletID, w.UserID, w.OrganizationID, w.Amount, w.Currency,
			w.PayoutMethodType, w.DestinationDetails, w.UserPaymentMethodID, w.UserNotes,
		).Scan(&w.ID, &w.PublicID, &statusStr, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return fmt.Errorf("create withdrawal request: %w", err)
		}
		w.Status = billing.WithdrawalStatus(statusStr)
		return nil
	})
}

// GetWithdrawalRequestByID retrieves a withdrawal request by ID.
func (r *Repository) GetWithdrawalRequestByID(ctx context.Context, id int64) (*billing.WalletWithdrawal, error) {
	var w billing.WalletWithdrawal
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payout_method_type, destination_details, user_payment_method_id, COALESCE(user_notes, ''),
			       status, COALESCE(rejection_reason, ''), reviewed_by, reviewed_at, transaction_id,
			       created_at, updated_at
			FROM billing.wallet_withdrawals
			WHERE id = $1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&w.ID, &w.PublicID, &w.WalletID, &w.UserID, &w.OrganizationID, &w.Amount, &w.Currency,
			&w.PayoutMethodType, &w.DestinationDetails, &w.UserPaymentMethodID, &w.UserNotes,
			&statusStr, &w.RejectionReason, &w.ReviewedBy, &w.ReviewedAt, &w.TransactionID,
			&w.CreatedAt, &w.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("withdrawal_request")
			}
			return err
		}
		w.Status = billing.WithdrawalStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListWithdrawalRequestsByUserWithStatus returns user withdrawal requests filtered by status.
func (r *Repository) ListWithdrawalRequestsByUserWithStatus(
	ctx context.Context, userID int64, status string, limit, offset int,
) ([]*billing.WalletWithdrawal, error) {
	var list []*billing.WalletWithdrawal
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payout_method_type, destination_details, user_payment_method_id, COALESCE(user_notes, ''),
			       status, COALESCE(rejection_reason, ''), reviewed_by, reviewed_at, transaction_id,
			       created_at, updated_at
			FROM billing.wallet_withdrawals
			WHERE user_id = $1
			  AND ($2 = '' OR status = $2)
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4;
		`
		rows, err := tx.Query(txCtx, query, userID, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var w billing.WalletWithdrawal
			var statusStr string
			if err := rows.Scan(
				&w.ID, &w.PublicID, &w.WalletID, &w.UserID, &w.OrganizationID, &w.Amount, &w.Currency,
				&w.PayoutMethodType, &w.DestinationDetails, &w.UserPaymentMethodID, &w.UserNotes,
				&statusStr, &w.RejectionReason, &w.ReviewedBy, &w.ReviewedAt, &w.TransactionID,
				&w.CreatedAt, &w.UpdatedAt,
			); err != nil {
				return err
			}
			w.Status = billing.WithdrawalStatus(statusStr)
			list = append(list, &w)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// AdminListDetailedWithdrawals lists withdrawal requests with user and org context for admin review.
func (r *Repository) AdminListDetailedWithdrawals(
	ctx context.Context, filter billing.WithdrawalFilter,
) ([]*billing.AdminWalletWithdrawalView, int, error) {
	var list []*billing.AdminWalletWithdrawalView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		whereClauses := []string{"1=1"}
		args := []any{}
		argIdx := 1

		if filter.UserID > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("w.user_id = $%d", argIdx))
			args = append(args, filter.UserID)
			argIdx++
		}
		if filter.WalletID > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("w.wallet_id = $%d", argIdx))
			args = append(args, filter.WalletID)
			argIdx++
		}
		if filter.Status != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("w.status = $%d", argIdx))
			args = append(args, filter.Status)
			argIdx++
		}
		if filter.PayoutMethodType != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("w.payout_method_type = $%d", argIdx))
			args = append(args, filter.PayoutMethodType)
			argIdx++
		}
		if filter.Search != "" {
			whereClauses = append(whereClauses, fmt.Sprintf(
				"(u.name ILIKE $%d OR u.email ILIKE $%d OR o.name ILIKE $%d OR w.destination_details ILIKE $%d)",
				argIdx, argIdx, argIdx, argIdx,
			))
			args = append(args, "%"+filter.Search+"%")
			argIdx++
		}

		whereSQL := strings.Join(whereClauses, " AND ")

		countQuery := fmt.Sprintf(`
			SELECT COUNT(*)
			FROM billing.wallet_withdrawals w
			JOIN identity.users u ON u.id = w.user_id
			LEFT JOIN org.organizations o ON o.id = w.organization_id
			WHERE %s;
		`, whereSQL)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("count withdrawals: %w", err)
		}

		limit := filter.Limit
		if limit <= 0 {
			limit = 20
		}

		dataQuery := fmt.Sprintf(`
			SELECT w.id, w.public_id::text, w.wallet_id, w.user_id,
			       COALESCE(u.name, '') AS user_name, COALESCE(u.email, '') AS user_email, COALESCE(u.phone, '') AS user_phone,
			       w.organization_id, COALESCE(o.name, '') AS org_name, COALESCE(o.type, '') AS org_type,
			       w.amount, w.currency, w.payout_method_type, w.destination_details, w.user_payment_method_id,
			       COALESCE(w.user_notes, ''), w.status, COALESCE(w.rejection_reason, ''),
			       w.reviewed_by, COALESCE(rev.name, '') AS reviewer_name, w.reviewed_at, w.transaction_id,
			       w.created_at, w.updated_at
			FROM billing.wallet_withdrawals w
			JOIN identity.users u ON u.id = w.user_id
			LEFT JOIN org.organizations o ON o.id = w.organization_id
			LEFT JOIN identity.users rev ON rev.id = w.reviewed_by
			WHERE %s
			ORDER BY w.created_at DESC
			LIMIT $%d OFFSET $%d;
		`, whereSQL, argIdx, argIdx+1)

		args = append(args, limit, filter.Offset)

		rows, err := tx.Query(txCtx, dataQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v billing.AdminWalletWithdrawalView
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.WalletID, &v.UserID,
				&v.UserName, &v.UserEmail, &v.UserPhone,
				&v.OrganizationID, &v.OrganizationName, &v.OrganizationType,
				&v.Amount, &v.Currency, &v.PayoutMethodType, &v.DestinationDetails, &v.UserPaymentMethodID,
				&v.UserNotes, &statusStr, &v.RejectionReason,
				&v.ReviewedBy, &v.ReviewerName, &v.ReviewedAt, &v.TransactionID,
				&v.CreatedAt, &v.UpdatedAt,
			); err != nil {
				return err
			}
			v.Status = billing.WithdrawalStatus(statusStr)
			list = append(list, &v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// AdminApproveWithdrawalRequest approves a pending withdrawal, writes the ledger debit transaction, and updates status.
func (r *Repository) AdminApproveWithdrawalRequest(
	ctx context.Context, withdrawalID int64, reviewerID int64,
) (*billing.WalletWithdrawal, *billing.WalletTransaction, error) {
	var w billing.WalletWithdrawal
	var txRecord *billing.WalletTransaction

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryWith := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payout_method_type, destination_details, user_payment_method_id, COALESCE(user_notes, ''),
			       status, created_at, updated_at
			FROM billing.wallet_withdrawals
			WHERE id = $1
			FOR UPDATE;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, queryWith, withdrawalID).Scan(
			&w.ID, &w.PublicID, &w.WalletID, &w.UserID, &w.OrganizationID, &w.Amount, &w.Currency,
			&w.PayoutMethodType, &w.DestinationDetails, &w.UserPaymentMethodID, &w.UserNotes,
			&statusStr, &w.CreatedAt, &w.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("withdrawal_request")
			}
			return err
		}
		w.Status = billing.WithdrawalStatus(statusStr)
		if w.Status != billing.WithdrawalPending {
			return apperr.Conflict("withdrawal.already_processed", fmt.Sprintf("طلب السحب تمت معالجته مسبقاً بحالة %s", w.Status))
		}

		// Check current balance and ensure no overdraft
		var currentBalance money.Amount
		queryLatest := `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1 FOR UPDATE;`
		err = tx.QueryRow(txCtx, queryLatest, w.WalletID).Scan(&currentBalance)
		if err != nil && !database.IsNotFound(err) {
			return fmt.Errorf("read wallet balance: %w", err)
		}
		if currentBalance.Minor() < w.Amount.Minor() {
			return apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة الحالي غير كافٍ لإتمام عملية السحب.", nil)
		}

		negDelta, err := money.Zero.Sub(w.Amount)
		if err != nil {
			return fmt.Errorf("compute negative delta: %w", err)
		}

		newBalance, err := currentBalance.Add(negDelta)
		if err != nil {
			return fmt.Errorf("compute updated wallet balance: %w", err)
		}

		desc := fmt.Sprintf("سحب رصيد معتمد إلى: %s", w.DestinationDetails)
		if w.UserNotes != "" {
			desc += " - " + w.UserNotes
		}

		queryInsertTx := `
			INSERT INTO billing.wallet_transactions (wallet_id, type, amount, balance_after, reference_type, reference_id, description)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at;
		`
		var tRec billing.WalletTransaction
		var txTypeStr string
		if err := tx.QueryRow(
			txCtx, queryInsertTx,
			w.WalletID, billing.TxWithdrawal, negDelta, newBalance, "withdrawal_approval", w.ID, desc,
		).Scan(
			&tRec.ID, &tRec.WalletID, &txTypeStr, &tRec.Amount, &tRec.BalanceAfter,
			&tRec.ReferenceType, &tRec.ReferenceID, &tRec.Description, &tRec.CreatedAt,
		); err != nil {
			return fmt.Errorf("record wallet debit transaction: %w", err)
		}
		tRec.Type = billing.TransactionType(txTypeStr)
		txRecord = &tRec

		now := time.Now()
		queryUpdateWith := `
			UPDATE billing.wallet_withdrawals
			SET status = 'approved', reviewed_by = $1, reviewed_at = $2, transaction_id = $3, updated_at = now()
			WHERE id = $4
			RETURNING updated_at;
		`
		if err := tx.QueryRow(txCtx, queryUpdateWith, reviewerID, now, tRec.ID, w.ID).Scan(&w.UpdatedAt); err != nil {
			return fmt.Errorf("update withdrawal status: %w", err)
		}
		w.Status = billing.WithdrawalApproved
		w.ReviewedBy = &reviewerID
		w.ReviewedAt = &now
		w.TransactionID = &tRec.ID

		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &w, txRecord, nil
}

// AdminRejectWithdrawalRequest rejects a pending withdrawal and records the rejection reason.
func (r *Repository) AdminRejectWithdrawalRequest(
	ctx context.Context, withdrawalID int64, reviewerID int64, reason string,
) (*billing.WalletWithdrawal, error) {
	var w billing.WalletWithdrawal

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryWith := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payout_method_type, destination_details, user_payment_method_id, COALESCE(user_notes, ''),
			       status, created_at, updated_at
			FROM billing.wallet_withdrawals
			WHERE id = $1
			FOR UPDATE;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, queryWith, withdrawalID).Scan(
			&w.ID, &w.PublicID, &w.WalletID, &w.UserID, &w.OrganizationID, &w.Amount, &w.Currency,
			&w.PayoutMethodType, &w.DestinationDetails, &w.UserPaymentMethodID, &w.UserNotes,
			&statusStr, &w.CreatedAt, &w.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("withdrawal_request")
			}
			return err
		}
		w.Status = billing.WithdrawalStatus(statusStr)
		if w.Status != billing.WithdrawalPending {
			return apperr.Conflict("withdrawal.already_processed", fmt.Sprintf("طلب السحب تمت معالجته مسبقاً بحالة %s", w.Status))
		}

		now := time.Now()
		queryUpdateWith := `
			UPDATE billing.wallet_withdrawals
			SET status = 'rejected', reviewed_by = $1, reviewed_at = $2, rejection_reason = $3, updated_at = now()
			WHERE id = $4
			RETURNING updated_at;
		`
		if err := tx.QueryRow(txCtx, queryUpdateWith, reviewerID, now, reason, w.ID).Scan(&w.UpdatedAt); err != nil {
			return fmt.Errorf("update withdrawal status: %w", err)
		}
		w.Status = billing.WithdrawalRejected
		w.ReviewedBy = &reviewerID
		w.ReviewedAt = &now
		w.RejectionReason = reason

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}
