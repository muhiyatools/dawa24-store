package identity

import (
	"context"
)

func (s *Service) AdminListUsers(ctx context.Context, role, status string) ([]*User, error) {
	return s.repo.AdminListUsers(ctx, role, status)
}

func (s *Service) AdminGetUser(ctx context.Context, id int64) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

func (s *Service) AdminSuspendUser(ctx context.Context, id, actorID int64) error {
	return s.repo.AdminUpdateUserStatus(ctx, id, "suspended", actorID)
}

func (s *Service) AdminReactivateUser(ctx context.Context, id, actorID int64) error {
	return s.repo.AdminUpdateUserStatus(ctx, id, "active", actorID)
}

func (s *Service) AdminResetMFA(ctx context.Context, id, actorID int64) error {
	return s.repo.AdminResetMFA(ctx, id, actorID)
}

func (s *Service) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	return s.repo.AdminAssignRole(ctx, id, role, actorID)
}
