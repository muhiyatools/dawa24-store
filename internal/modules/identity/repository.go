package identity

import (
	"context"
)

// Repository defines the storage contract for the identity module.
type Repository interface {
	CreateUser(ctx context.Context, u *User) error
	GetUserByID(ctx context.Context, id int64) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, u *User) error

	GetSecurity(ctx context.Context, userID int64) (*UserSecurity, error)
	UpsertSecurity(ctx context.Context, s *UserSecurity) error

	GetMFA(ctx context.Context, userID int64) (*UserMFA, error)
	UpsertMFA(ctx context.Context, mfa *UserMFA) error

	GetPermissionsForUser(ctx context.Context, userID int64, orgID int64) ([]string, error)
	GetRolesForUser(ctx context.Context, userID int64) ([]string, error)
	UserBelongsToOrg(ctx context.Context, userID int64, orgID int64) (bool, error)
}
