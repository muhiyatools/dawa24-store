package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Platform administration over user accounts.
//
// Every function in this file was previously a stub: AdminListUsers returned
// (nil, nil) and the three mutations returned nil without touching the
// database. Four live endpoints and the admin users screen sat on top of them,
// so the user list was permanently empty and suspending an account, resetting
// its MFA or changing its role all reported success while doing nothing. A
// silent no-op on a privileged action is worse than an error: the operator
// believes the account is locked and moves on.
//
// These read and write across every tenant, so they run under
// database.AsSystem, and each mutation records an audit row in the same
// transaction as the change it describes.

// AdminListUsers returns users across all tenants, optionally filtered.
func (r *Repository) AdminListUsers(ctx context.Context, role, status string) ([]*identity.User, error) {
	var list []*identity.User
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, email, name, role, status, language, timezone,
			       phone, email_verified_at, phone_verified_at, created_at, updated_at
			FROM identity.users
			WHERE deleted_at IS NULL
			  AND ($1::text IS NULL OR role = $1)
			  AND ($2::text IS NULL OR status = $2)
			ORDER BY created_at DESC
			LIMIT 200;
		`
		// Empty filters mean "no filter" rather than "match the empty string".
		var rolePtr, statusPtr *string
		if role != "" {
			rolePtr = &role
		}
		if status != "" {
			statusPtr = &status
		}

		rows, err := tx.Query(txCtx, query, rolePtr, statusPtr)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var u identity.User
			var statusStr, langStr string
			if err := rows.Scan(
				&u.ID, &u.PublicID, &u.Email, &u.Name, &u.Role, &statusStr, &langStr,
				&u.Timezone, &u.Phone, &u.EmailVerifiedAt, &u.PhoneVerifiedAt,
				&u.CreatedAt, &u.UpdatedAt,
			); err != nil {
				return err
			}
			u.Status = identity.UserStatus(statusStr)
			u.Language = i18n.Lang(langStr)
			list = append(list, &u)
		}
		return rows.Err()
	})
	return list, err
}

// AdminUpdateUserStatus activates, suspends or bans an account.
func (r *Repository) AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var before string
		if err := tx.QueryRow(txCtx,
			`SELECT status FROM identity.users WHERE id = $1 AND deleted_at IS NULL;`, id,
		).Scan(&before); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user")
			}
			return fmt.Errorf("identity postgres: read user status: %w", err)
		}

		tag, err := tx.Exec(txCtx,
			`UPDATE identity.users SET status = $1, updated_at = now() WHERE id = $2 AND deleted_at IS NULL;`,
			status, id,
		)
		if err != nil {
			return fmt.Errorf("identity postgres: update user status: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("user")
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "identity.user.status_changed",
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(id, 10),
			Before:      map[string]string{"status": before},
			After:       map[string]string{"status": status},
		})
	})
}

// AdminResetMFA clears a user's second factor so they can enrol again.
func (r *Repository) AdminResetMFA(ctx context.Context, id int64, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var existed bool
		if err := tx.QueryRow(txCtx,
			`SELECT EXISTS (SELECT 1 FROM identity.user_mfa WHERE user_id = $1);`, id,
		).Scan(&existed); err != nil {
			return fmt.Errorf("identity postgres: check mfa: %w", err)
		}

		// The row is removed rather than disabled. mfa_enabled_requires_secret
		// permits an enabled record only with a secret and a confirmation, and
		// leaving a stale secret behind for a factor the user no longer holds
		// serves no purpose.
		if _, err := tx.Exec(txCtx, `DELETE FROM identity.user_mfa WHERE user_id = $1;`, id); err != nil {
			return fmt.Errorf("identity postgres: reset mfa: %w", err)
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "identity.user.mfa_reset",
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(id, 10),
			Before:      map[string]bool{"mfa_configured": existed},
			After:       map[string]bool{"mfa_configured": false},
		})
	})
}

// AdminAssignRole changes a user's platform role.
func (r *Repository) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var before string
		if err := tx.QueryRow(txCtx,
			`SELECT role FROM identity.users WHERE id = $1 AND deleted_at IS NULL;`, id,
		).Scan(&before); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user")
			}
			return fmt.Errorf("identity postgres: read user role: %w", err)
		}

		tag, err := tx.Exec(txCtx,
			`UPDATE identity.users SET role = $1, updated_at = now() WHERE id = $2 AND deleted_at IS NULL;`,
			role, id,
		)
		if err != nil {
			return fmt.Errorf("identity postgres: assign role: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("user")
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "identity.user.role_assigned",
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(id, 10),
			Before:      map[string]string{"role": before},
			After:       map[string]string{"role": role},
		})
	})
}

// AdminCountUsers returns the total number of active user accounts.
func (r *Repository) AdminCountUsers(ctx context.Context) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx,
			`SELECT COUNT(*) FROM identity.users WHERE deleted_at IS NULL;`).Scan(&total)
	})
	return total, err
}

// CreateAccountDeletionRequest submits a new deletion request.
func (r *Repository) CreateAccountDeletionRequest(ctx context.Context, req *identity.AccountDeletionRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO identity.account_deletion_requests (
				user_id, organization_id, reason, status, admin_notes, created_at, updated_at
			) VALUES ($1, $2, $3, 'pending', '', now(), now())
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, req.UserID, req.OrganizationID, req.Reason).Scan(
			&req.ID, &req.CreatedAt, &req.UpdatedAt,
		)
	})
}

// ListAccountDeletionRequests lists all deletion requests with user info.
func (r *Repository) ListAccountDeletionRequests(ctx context.Context, status string) ([]*identity.AccountDeletionRequest, error) {
	var list []*identity.AccountDeletionRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT r.id, r.user_id, COALESCE(u.name->>'ar', u.name->>'en', ''), COALESCE(u.email, ''), COALESCE(u.role, ''),
			       r.organization_id, COALESCE(o.name->>'ar', o.name->>'en', ''), r.reason, r.status,
			       r.admin_notes, r.reviewed_by, r.reviewed_at, r.created_at, r.updated_at
			FROM identity.account_deletion_requests r
			JOIN identity.users u ON u.id = r.user_id
			LEFT JOIN org.organizations o ON o.id = r.organization_id
			WHERE ($1::text IS NULL OR r.status = $1)
			ORDER BY r.created_at DESC
			LIMIT 100;
		`
		var statusPtr *string
		if status != "" {
			statusPtr = &status
		}
		rows, err := tx.Query(txCtx, query, statusPtr)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item identity.AccountDeletionRequest
			if err := rows.Scan(
				&item.ID, &item.UserID, &item.UserName, &item.UserEmail, &item.UserRole,
				&item.OrganizationID, &item.OrganizationName, &item.Reason, &item.Status,
				&item.AdminNotes, &item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

// ReviewAccountDeletionRequest approves or rejects an account deletion request.
func (r *Repository) ReviewAccountDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var userID int64
		var currentStatus string
		if err := tx.QueryRow(txCtx,
			`SELECT user_id, status FROM identity.account_deletion_requests WHERE id = $1;`, requestID,
		).Scan(&userID, &currentStatus); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("account_deletion_request")
			}
			return err
		}

		newStatus := "rejected"
		if approve {
			newStatus = "approved"
			_, _ = tx.Exec(txCtx,
				`UPDATE identity.users SET status = 'suspended', deleted_at = now(), updated_at = now() WHERE id = $1;`, userID)
		}

		_, err := tx.Exec(txCtx, `
			UPDATE identity.account_deletion_requests
			SET status = $1, admin_notes = $2, reviewed_by = $3, reviewed_at = now(), updated_at = now()
			WHERE id = $4;
		`, newStatus, adminNotes, reviewerID, requestID)
		return err
	})
}
