package identity_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// mockRepo is an in-memory repository for unit testing identity workflows.
type mockRepo struct {
	users       map[int64]*identity.User
	usersByMail map[string]*identity.User
	securities  map[int64]*identity.UserSecurity
	mfas        map[int64]*identity.UserMFA
	permissions map[int64][]string
	orgMembers  map[string]bool
	nextID      int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:       map[int64]*identity.User{},
		usersByMail: map[string]*identity.User{},
		securities:  map[int64]*identity.UserSecurity{},
		mfas:        map[int64]*identity.UserMFA{},
		permissions: map[int64][]string{},
		orgMembers:  map[string]bool{},
		nextID:      1,
	}
}

func (m *mockRepo) CreateUser(_ context.Context, u *identity.User) error {
	cleanEmail := identity.NormalizeEmail(u.Email)
	if _, exists := m.usersByMail[cleanEmail]; exists {
		return apperr.Conflict("user.email_exists", "Email already exists")
	}
	u.ID = m.nextID
	u.PublicID = "pub-test-uuid"
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	m.nextID++

	m.users[u.ID] = u
	m.usersByMail[cleanEmail] = u
	return nil
}

func (m *mockRepo) GetUserByID(_ context.Context, id int64) (*identity.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, apperr.NotFound("user")
	}
	return u, nil
}

func (m *mockRepo) GetUserByEmail(_ context.Context, email string) (*identity.User, error) {
	u, ok := m.usersByMail[identity.NormalizeEmail(email)]
	if !ok {
		return nil, apperr.NotFound("user")
	}
	return u, nil
}

func (m *mockRepo) UpdateUser(_ context.Context, u *identity.User) error {
	m.users[u.ID] = u
	m.usersByMail[identity.NormalizeEmail(u.Email)] = u
	return nil
}

func (m *mockRepo) GetSecurity(_ context.Context, userID int64) (*identity.UserSecurity, error) {
	s, ok := m.securities[userID]
	if !ok {
		return &identity.UserSecurity{UserID: userID}, nil
	}
	return s, nil
}

func (m *mockRepo) UpsertSecurity(_ context.Context, s *identity.UserSecurity) error {
	m.securities[s.UserID] = s
	return nil
}

func (m *mockRepo) GetMFA(_ context.Context, userID int64) (*identity.UserMFA, error) {
	mfa, ok := m.mfas[userID]
	if !ok {
		return &identity.UserMFA{UserID: userID}, nil
	}
	return mfa, nil
}

func (m *mockRepo) UpsertMFA(_ context.Context, mfa *identity.UserMFA) error {
	m.mfas[mfa.UserID] = mfa
	return nil
}

func (m *mockRepo) GetPermissionsForUser(_ context.Context, userID int64, orgID int64) ([]string, error) {
	return m.permissions[userID], nil
}

func (m *mockRepo) GetRolesForUser(_ context.Context, userID int64) ([]string, error) {
	if u, ok := m.users[userID]; ok {
		return []string{u.Role}, nil
	}
	return []string{}, nil
}

func (m *mockRepo) UserBelongsToOrg(_ context.Context, userID int64, orgID int64) (bool, error) {
	return true, nil
}

func (m *mockRepo) CreateAddress(_ context.Context, addr *identity.UserAddress) error {
	addr.ID = m.nextID
	m.nextID++
	return nil
}
func (m *mockRepo) GetAddressByID(_ context.Context, id, userID int64) (*identity.UserAddress, error) {
	return nil, apperr.NotFound("user_address")
}
func (m *mockRepo) ListAddresses(_ context.Context, userID int64) ([]*identity.UserAddress, error) {
	return nil, nil
}
func (m *mockRepo) UpdateAddress(_ context.Context, addr *identity.UserAddress) error {
	return nil
}
func (m *mockRepo) DeleteAddress(_ context.Context, id, userID int64) error {
	return nil
}

func (m *mockRepo) AddFavorite(_ context.Context, userID, productID int64) error {
	return nil
}
func (m *mockRepo) RemoveFavorite(_ context.Context, userID, productID int64) error {
	return nil
}
func (m *mockRepo) ListFavorites(_ context.Context, userID int64) ([]int64, error) {
	return nil, nil
}
func (m *mockRepo) AdminListUsers(ctx context.Context, role, status string) ([]*identity.User, error) {
	return nil, nil
}
func (m *mockRepo) AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error {
	return nil
}
func (m *mockRepo) AdminResetMFA(ctx context.Context, id int64, actorID int64) error { return nil }
func (m *mockRepo) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	return nil
}

func TestServiceRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := identity.NewService(repo, nil, logger)

	// 1. Register a new user
	regInput := identity.RegisterInput{
		Email:    "pharmacist@dawa24.eg",
		Password: "SecurePassword123!",
		NameAr:   "صيدلية الشفاء",
		NameEn:   "El-Shefaa Pharmacy",
		Role:     "customer",
		Language: i18n.AR,
		Phone:    "+201012345678",
	}

	user, _, err := svc.Register(ctx, regInput)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if user.Email != "pharmacist@dawa24.eg" {
		t.Errorf("User email = %q; want %q", user.Email, "pharmacist@dawa24.eg")
	}

	// 2. Duplicate registration fails
	_, _, err = svc.Register(ctx, regInput)
	if err == nil || apperr.KindOf(err) != apperr.KindConflict {
		t.Errorf("expected Conflict error on duplicate email, got %v", err)
	}

	// 3. Login with correct password
	loginRes, err := svc.Login(ctx, identity.LoginInput{
		Email:    "pharmacist@dawa24.eg",
		Password: "SecurePassword123!",
	})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if loginRes.User.ID != user.ID {
		t.Errorf("logged in user ID = %d; want %d", loginRes.User.ID, user.ID)
	}

	// 4. Login with wrong password
	_, err = svc.Login(ctx, identity.LoginInput{
		Email:    "pharmacist@dawa24.eg",
		Password: "WrongPassword!",
	})
	if err != identity.ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials on wrong password, got %v", err)
	}

	// 5. Account lockout after 5 consecutive failures
	for i := 0; i < 4; i++ {
		_, _ = svc.Login(ctx, identity.LoginInput{
			Email:    "pharmacist@dawa24.eg",
			Password: "WrongPassword!",
		})
	}

	// Next attempt should be locked
	_, err = svc.Login(ctx, identity.LoginInput{
		Email:    "pharmacist@dawa24.eg",
		Password: "SecurePassword123!",
	})
	if err == nil || apperr.KindOf(err) != apperr.KindForbidden {
		t.Errorf("expected Forbidden error due to account lockout, got %v", err)
	}
}
