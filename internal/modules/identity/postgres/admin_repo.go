package postgres

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
)

func (r *Repository) AdminListUsers(ctx context.Context, role, status string) ([]*identity.User, error) {
	return nil, nil
}
func (r *Repository) AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error {
	return nil
}
func (r *Repository) AdminResetMFA(ctx context.Context, id int64, actorID int64) error {
	return nil
}
func (r *Repository) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	return nil
}
