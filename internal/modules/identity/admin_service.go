package identity

import (
	"context"
)

func (s *Service) AdminListUsers(ctx context.Context, role, status string) ([]*User, error) {
	return s.repo.AdminListUsers(ctx, role, status)
}

func (s *Service) SearchUsers(ctx context.Context, query, role string, limit int) ([]*User, error) {
	return s.repo.SearchUsers(ctx, query, role, limit)
}

// AdminCountUsers returns the total number of accounts, for dashboards that
// need a figure rather than a page of rows.
func (s *Service) AdminCountUsers(ctx context.Context) (int, error) {
	return s.repo.AdminCountUsers(ctx)
}

func (s *Service) AdminGetUser(ctx context.Context, id int64) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

// AdminSuspendUser blocks an account and ends the sessions it already holds.
//
// Changing the stored status alone is not enough: sessions live in Redis and
// are checked against the token, not re-read from the row, so a suspended user
// would keep working from an already-open tab until their cookie expired. That
// window is exactly what suspending an account is meant to close.
func (s *Service) AdminSuspendUser(ctx context.Context, id, actorID int64) error {
	if err := s.repo.AdminUpdateUserStatus(ctx, id, string(StatusSuspended), actorID); err != nil {
		return err
	}
	if s.sessionStore == nil {
		return nil
	}
	if err := s.sessionStore.DeleteAllForUser(ctx, id); err != nil {
		// The account is already suspended in the database, which is the part
		// that must not be rolled back. A failure to reach Redis is logged and
		// surfaced rather than swallowed, because it leaves live sessions
		// behind and an operator needs to know that.
		s.log.ErrorContext(ctx, "suspended user but could not revoke sessions",
			"error", err, "user_id", id, "actor_id", actorID)
		return err
	}
	return nil
}

func (s *Service) AdminReactivateUser(ctx context.Context, id, actorID int64) error {
	return s.repo.AdminUpdateUserStatus(ctx, id, string(StatusActive), actorID)
}

// AdminResetMFA clears a user's second factor and ends their sessions, so a
// session established with the old factor cannot outlive it.
func (s *Service) AdminResetMFA(ctx context.Context, id, actorID int64) error {
	if err := s.repo.AdminResetMFA(ctx, id, actorID); err != nil {
		return err
	}
	if s.sessionStore == nil {
		return nil
	}
	if err := s.sessionStore.DeleteAllForUser(ctx, id); err != nil {
		s.log.ErrorContext(ctx, "reset MFA but could not revoke sessions",
			"error", err, "user_id", id, "actor_id", actorID)
		return err
	}
	return nil
}

// AdminAssignRole changes a user's role and ends their sessions.
//
// Permissions are resolved from the role when a request is authorised, so a
// user demoted mid-session could otherwise keep acting with the permissions
// they held when they signed in.
func (s *Service) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	if err := s.repo.AdminAssignRole(ctx, id, role, actorID); err != nil {
		return err
	}
	if s.sessionStore == nil {
		return nil
	}
	if err := s.sessionStore.DeleteAllForUser(ctx, id); err != nil {
		s.log.ErrorContext(ctx, "assigned role but could not revoke sessions",
			"error", err, "user_id", id, "actor_id", actorID)
		return err
	}
	return nil
}

// RequestAccountDeletion allows a user to submit a formal request to delete their account.
func (s *Service) RequestAccountDeletion(ctx context.Context, userID int64, orgID *int64, reason string) error {
	req := &AccountDeletionRequest{
		UserID:         userID,
		OrganizationID: orgID,
		Reason:         reason,
		Status:         "pending",
	}
	return s.repo.CreateAccountDeletionRequest(ctx, req)
}

// AdminListDeletionRequests lists all pending or past account deletion requests.
func (s *Service) AdminListDeletionRequests(ctx context.Context, status string) ([]*AccountDeletionRequest, error) {
	return s.repo.ListAccountDeletionRequests(ctx, status)
}

// AdminReviewDeletionRequest approves or rejects an account deletion request.
func (s *Service) AdminReviewDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) error {
	return s.repo.ReviewAccountDeletionRequest(ctx, requestID, reviewerID, approve, adminNotes)
}
