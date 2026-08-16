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

	CreateAddress(ctx context.Context, addr *UserAddress) error
	GetAddressByID(ctx context.Context, id, userID int64) (*UserAddress, error)
	ListAddresses(ctx context.Context, userID int64) ([]*UserAddress, error)
	UpdateAddress(ctx context.Context, addr *UserAddress) error
	DeleteAddress(ctx context.Context, id, userID int64) error

	AddFavorite(ctx context.Context, userID, productID int64) error
	RemoveFavorite(ctx context.Context, userID, productID int64) error
	ListFavorites(ctx context.Context, userID int64) ([]int64, error)

	AdminListUsers(ctx context.Context, role, status string) ([]*User, error)
	AdminCountUsers(ctx context.Context) (int, error)
	DefaultOrgForUser(ctx context.Context, userID int64) (int64, error)
	AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error
	AdminResetMFA(ctx context.Context, id int64, actorID int64) error
	AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error
}
