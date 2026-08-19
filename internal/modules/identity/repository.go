package identity

import (
	"context"
)

// Repository defines the storage contract for the identity module.
type Repository interface {
	CreateUser(ctx context.Context, u *User) error
	RegisterOrganization(ctx context.Context, u *User, org RegisterOrgInput) (*RegisterOrgResult, error)
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
	ListAddressHistory(ctx context.Context, userID int64, limit int) ([]*UserAddressHistory, error)

	AddFavorite(ctx context.Context, userID, productID int64) error
	RemoveFavorite(ctx context.Context, userID, productID int64) error
	ListFavorites(ctx context.Context, userID int64) ([]int64, error)

	AdminListUsers(ctx context.Context, role, status string) ([]*User, error)
	AdminCountUsers(ctx context.Context) (int, error)
	DefaultOrgForUser(ctx context.Context, userID int64) (int64, error)
	DefaultOrgInfoForUser(ctx context.Context, userID int64) (orgID int64, orgType, orgStatus string, err error)
	ListUserOrganizations(ctx context.Context, userID int64) ([]*UserOrgMembership, error)
	AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error
	AdminResetMFA(ctx context.Context, id int64, actorID int64) error
	AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error

	GetPreferences(ctx context.Context, userID int64) (*UserPreferences, error)
	UpdatePreferences(ctx context.Context, p *UserPreferences) error

	ListSessionPlans(ctx context.Context) ([]*SessionPlan, error)
	GetSessionPlanByID(ctx context.Context, id int64) (*SessionPlan, error)
	SetMaxLoginSessions(ctx context.Context, userID int64, max int) error

	CreateAccountDeletionRequest(ctx context.Context, req *AccountDeletionRequest) error
	ListAccountDeletionRequests(ctx context.Context, status string) ([]*AccountDeletionRequest, error)
	ReviewAccountDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) error
}
