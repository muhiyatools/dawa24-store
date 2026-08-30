package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type testMockIdentityRepo struct {
	users      map[int64]*identity.User
	usersByEm  map[string]*identity.User
	security   map[int64]*identity.UserSecurity
	mfa        map[int64]*identity.UserMFA
	nextUserID int64
}

func newTestMockIdentityRepo() *testMockIdentityRepo {
	return &testMockIdentityRepo{
		users:      make(map[int64]*identity.User),
		usersByEm:  make(map[string]*identity.User),
		security:   make(map[int64]*identity.UserSecurity),
		mfa:        make(map[int64]*identity.UserMFA),
		nextUserID: 1,
	}
}

func (m *testMockIdentityRepo) CreateUser(ctx context.Context, u *identity.User) error {
	u.ID = m.nextUserID
	m.nextUserID++
	u.Status = identity.StatusActive
	m.users[u.ID] = u
	m.usersByEm[u.Email] = u
	return nil
}

func (m *testMockIdentityRepo) RegisterOrganization(ctx context.Context, u *identity.User, org identity.RegisterOrgInput) (*identity.RegisterOrgResult, error) {
	_ = m.CreateUser(ctx, u)
	return &identity.RegisterOrgResult{
		OrganizationID:     10,
		OrganizationType:   "customer",
		OrganizationStatus: "approved",
	}, nil
}

func (m *testMockIdentityRepo) GetUserByID(ctx context.Context, id int64) (*identity.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, apperr.NotFound("user")
	}
	return u, nil
}

func (m *testMockIdentityRepo) GetUserByEmail(ctx context.Context, email string) (*identity.User, error) {
	u, ok := m.usersByEm[email]
	if !ok {
		return nil, apperr.NotFound("user")
	}
	return u, nil
}

func (m *testMockIdentityRepo) UpdateUser(ctx context.Context, u *identity.User) error {
	m.users[u.ID] = u
	m.usersByEm[u.Email] = u
	return nil
}

func (m *testMockIdentityRepo) GetSecurity(ctx context.Context, userID int64) (*identity.UserSecurity, error) {
	s, ok := m.security[userID]
	if !ok {
		s = &identity.UserSecurity{UserID: userID}
		m.security[userID] = s
	}
	return s, nil
}

func (m *testMockIdentityRepo) UpsertSecurity(ctx context.Context, s *identity.UserSecurity) error {
	m.security[s.UserID] = s
	return nil
}

func (m *testMockIdentityRepo) GetMFA(ctx context.Context, userID int64) (*identity.UserMFA, error) {
	val, ok := m.mfa[userID]
	if !ok {
		val = &identity.UserMFA{UserID: userID}
	}
	return val, nil
}

func (m *testMockIdentityRepo) UpsertMFA(ctx context.Context, mfa *identity.UserMFA) error {
	m.mfa[mfa.UserID] = mfa
	return nil
}

func (m *testMockIdentityRepo) GetPermissionsForUser(ctx context.Context, userID, orgID int64) ([]string, error) {
	return []string{"pharmacy.dashboard.view", "vendor.dashboard.view", "vendor.session.view"}, nil
}

func (m *testMockIdentityRepo) GetRolesForUser(ctx context.Context, userID int64) ([]string, error) {
	return []string{"user"}, nil
}

func (m *testMockIdentityRepo) UserBelongsToOrg(ctx context.Context, userID, orgID int64) (bool, error) {
	return true, nil
}

func (m *testMockIdentityRepo) CreateAddress(ctx context.Context, addr *identity.UserAddress) error {
	return nil
}
func (m *testMockIdentityRepo) GetAddressByID(ctx context.Context, id, userID int64) (*identity.UserAddress, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) ListAddresses(ctx context.Context, userID int64) ([]*identity.UserAddress, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) UpdateAddress(ctx context.Context, addr *identity.UserAddress) error {
	return nil
}
func (m *testMockIdentityRepo) DeleteAddress(ctx context.Context, id, userID int64) error { return nil }
func (m *testMockIdentityRepo) ListAddressHistory(ctx context.Context, userID int64, limit int) ([]*identity.UserAddressHistory, error) {
	return nil, nil
}

func (m *testMockIdentityRepo) AddFavorite(ctx context.Context, userID, productID int64) error {
	return nil
}
func (m *testMockIdentityRepo) RemoveFavorite(ctx context.Context, userID, productID int64) error {
	return nil
}
func (m *testMockIdentityRepo) ListFavorites(ctx context.Context, userID int64) ([]int64, error) {
	return nil, nil
}

func (m *testMockIdentityRepo) AdminListUsers(ctx context.Context, role, status string) ([]*identity.User, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) SearchUsers(ctx context.Context, query, role string, limit int) ([]*identity.User, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) AdminCountUsers(ctx context.Context) (int, error) {
	return len(m.users), nil
}
func (m *testMockIdentityRepo) DefaultOrgForUser(ctx context.Context, userID int64) (int64, error) {
	return 10, nil
}
func (m *testMockIdentityRepo) DefaultOrgInfoForUser(ctx context.Context, userID int64) (int64, string, string, error) {
	return 10, "customer", "approved", nil
}
func (m *testMockIdentityRepo) ListUserOrganizations(ctx context.Context, userID int64) ([]*identity.UserOrgMembership, error) {
	return []*identity.UserOrgMembership{
		{OrganizationID: 10, OrgType: "customer", OrgStatus: "approved", RoleKey: "owner", IsActive: true},
	}, nil
}
func (m *testMockIdentityRepo) AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) AdminResetMFA(ctx context.Context, id int64, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	return nil
}

func (m *testMockIdentityRepo) ListPlatformRoles(ctx context.Context) ([]*identity.PlatformRole, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) GetPlatformRole(ctx context.Context, key string) (*identity.PlatformRole, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) CreatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) UpdatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) DeletePlatformRole(ctx context.Context, key string) error { return nil }

func (m *testMockIdentityRepo) GetPreferences(ctx context.Context, userID int64) (*identity.UserPreferences, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) UpdatePreferences(ctx context.Context, p *identity.UserPreferences) error {
	return nil
}

func (m *testMockIdentityRepo) ListSessionPlans(ctx context.Context) ([]*identity.SessionPlan, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) GetSessionPlanByID(ctx context.Context, id int64) (*identity.SessionPlan, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) SetMaxLoginSessions(ctx context.Context, userID int64, max int) error {
	return nil
}
func (m *testMockIdentityRepo) GetOrgPlanLimits(ctx context.Context, orgID int64) (int, int, string, error) {
	return 5, 5, "باقة احترافية", nil
}

func (m *testMockIdentityRepo) CreateAccountDeletionRequest(ctx context.Context, req *identity.AccountDeletionRequest) error {
	return nil
}
func (m *testMockIdentityRepo) ListAccountDeletionRequests(ctx context.Context, status string) ([]*identity.AccountDeletionRequest, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) ReviewAccountDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) error {
	return nil
}

func setupTestMFAHandler(t *testing.T) (*UIHandler, *identity.Service, *testMockIdentityRepo, *identity.SessionStore) {
	t.Helper()
	repo := newTestMockIdentityRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := identity.NewSessionStore(nil, config.Session{
		CookieName: "dawa24_session",
		TTL:        24 * time.Hour,
		SecureOnly: false,
	})
	idSvc := identity.NewService(repo, store, logger)

	h := &UIHandler{
		idSvc: idSvc,
		log:   logger,
	}
	return h, idSvc, repo, store
}

func TestMFA_FullLoginChallengeFlow(t *testing.T) {
	h, idSvc, repo, _ := setupTestMFAHandler(t)
	ctx := context.Background()

	// 1. Register a user
	user, _, err := idSvc.Register(ctx, identity.RegisterInput{
		Email:    "pharmacist-mfa@dawa24.eg",
		Password: "Password123!",
		NameAr:   "صيدلي تجريبي",
		NameEn:   "Test Pharmacist",
		Role:     "customer",
		Language: i18n.AR,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 2. Normal Login when MFA is DISABLED
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=pharmacist-mfa@dawa24.eg&password=Password123!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.LoginSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 see other, got %d", res.StatusCode)
	}
	cookies := res.Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "dawa24_session" {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatalf("expected dawa24_session cookie on login without MFA")
	}

	// 3. Setup and Enable MFA for this user
	secret, err := identity.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	_ = repo.UpsertMFA(ctx, &identity.UserMFA{
		UserID:     user.ID,
		TOTPSecret: []byte(secret),
	})
	validCode, _ := identity.GenerateTOTPCode(secret, time.Now().UTC())
	actResult, err := idSvc.ConfirmEnableMFA(ctx, user.ID, validCode)
	if err != nil || !actResult.Success {
		t.Fatalf("ConfirmEnableMFA failed: %v", err)
	}

	// 4. Login when MFA is ENABLED -> MUST redirect to /auth/mfa-verify and issue pending cookie
	reqMFA := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=pharmacist-mfa@dawa24.eg&password=Password123!"))
	reqMFA.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMFA := httptest.NewRecorder()
	h.LoginSubmit(recMFA, reqMFA)

	resMFA := recMFA.Result()
	loc := resMFA.Header.Get("Location")
	if !strings.HasPrefix(loc, "/auth/mfa-verify") {
		t.Fatalf("expected redirect to /auth/mfa-verify, got %s", loc)
	}

	var pendingCookie *http.Cookie
	for _, c := range resMFA.Cookies() {
		if c.Name == "dawa24_mfa_pending" {
			pendingCookie = c
			break
		}
	}
	if pendingCookie == nil || pendingCookie.Value == "" {
		t.Fatalf("expected dawa24_mfa_pending cookie issued")
	}

	// 5. Test MFA Verify with WRONG code
	reqWrong := httptest.NewRequest("POST", "/auth/mfa-verify", strings.NewReader("code=000000"))
	reqWrong.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqWrong.AddCookie(pendingCookie)
	recWrong := httptest.NewRecorder()
	h.MFAVerifySubmit(recWrong, reqWrong)

	resWrong := recWrong.Result()
	if !strings.Contains(resWrong.Header.Get("Location"), "error=invalid_code") {
		t.Fatalf("expected invalid_code redirect on wrong code, got %s", resWrong.Header.Get("Location"))
	}

	// 6. Test MFA Verify with VALID TOTP code
	freshCode, _ := identity.GenerateTOTPCode(secret, time.Now().UTC())
	reqValid := httptest.NewRequest("POST", "/auth/mfa-verify", strings.NewReader("code="+freshCode))
	reqValid.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqValid.AddCookie(pendingCookie)
	recValid := httptest.NewRecorder()
	h.MFAVerifySubmit(recValid, reqValid)

	resValid := recValid.Result()
	if resValid.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 see other, got %d", resValid.StatusCode)
	}

	var finalSessCookie *http.Cookie
	var clearedPendingCookie *http.Cookie
	for _, c := range resValid.Cookies() {
		if c.Name == "dawa24_session" {
			finalSessCookie = c
		}
		if c.Name == "dawa24_mfa_pending" {
			clearedPendingCookie = c
		}
	}
	if finalSessCookie == nil || finalSessCookie.Value == "" {
		t.Fatalf("expected dawa24_session cookie after successful MFA verification")
	}
	if clearedPendingCookie == nil || clearedPendingCookie.MaxAge != -1 {
		t.Fatalf("expected dawa24_mfa_pending cookie to be cleared (MaxAge=-1)")
	}

	// 7. Test MFA Verify with single-use RECOVERY CODE
	reqMFA2 := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=pharmacist-mfa@dawa24.eg&password=Password123!"))
	reqMFA2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMFA2 := httptest.NewRecorder()
	h.LoginSubmit(recMFA2, reqMFA2)

	var pendingCookie2 *http.Cookie
	for _, c := range recMFA2.Result().Cookies() {
		if c.Name == "dawa24_mfa_pending" {
			pendingCookie2 = c
			break
		}
	}

	recoveryCode := actResult.RecoveryCodes[0]
	form := url.Values{}
	form.Set("recovery_code", recoveryCode)
	reqRec := httptest.NewRequest("POST", "/auth/mfa-verify", strings.NewReader(form.Encode()))
	reqRec.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqRec.AddCookie(pendingCookie2)
	recRec := httptest.NewRecorder()
	h.MFAVerifySubmit(recRec, reqRec)

	if recRec.Result().StatusCode != http.StatusSeeOther {
		t.Fatalf("expected login success via recovery code, got %d", recRec.Result().StatusCode)
	}

	// Reusing same recovery code should now FAIL
	reqMFA3 := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=pharmacist-mfa@dawa24.eg&password=Password123!"))
	reqMFA3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMFA3 := httptest.NewRecorder()
	h.LoginSubmit(recMFA3, reqMFA3)

	var pendingCookie3 *http.Cookie
	for _, c := range recMFA3.Result().Cookies() {
		if c.Name == "dawa24_mfa_pending" {
			pendingCookie3 = c
			break
		}
	}

	reqRec2 := httptest.NewRequest("POST", "/auth/mfa-verify", strings.NewReader(form.Encode()))
	reqRec2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqRec2.AddCookie(pendingCookie3)
	recRec2 := httptest.NewRecorder()
	h.MFAVerifySubmit(recRec2, reqRec2)

	if !strings.Contains(recRec2.Result().Header.Get("Location"), "error=invalid_code") {
		t.Fatalf("expected reuse of consumed recovery code to fail")
	}
}

func TestMFA_DashboardSettingsHandlers(t *testing.T) {
	h, idSvc, _, _ := setupTestMFAHandler(t)
	ctx := context.Background()

	// Register user
	user, _, err := idSvc.Register(ctx, identity.RegisterInput{
		Email:    "vendor-mfa@dawa24.eg",
		Password: "VendorPassword123!",
		NameAr:   "مورد تجريبي",
		NameEn:   "Test Vendor",
		Role:     "vendor",
		Language: i18n.AR,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	actor := authctx.Actor{
		UserID:         user.ID,
		Email:          user.Email,
		Role:           "vendor",
		OrganizationID: 20,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.session.view"},
	}
	ctxWithActor := authctx.WithActor(context.Background(), actor)

	// 1. GET /vendor/mfa (Unenrolled state)
	reqGet := httptest.NewRequest("GET", "/vendor/mfa", nil).WithContext(ctxWithActor)
	recGet := httptest.NewRecorder()
	h.VendorMFAPage(recGet, reqGet)
	if recGet.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on /vendor/mfa, got %d", recGet.Result().StatusCode)
	}

	// 2. POST /vendor/mfa/setup (Initiate setup)
	reqSetup := httptest.NewRequest("POST", "/vendor/mfa/setup", nil).WithContext(ctxWithActor)
	recSetup := httptest.NewRecorder()
	h.VendorMFASetupSubmit(recSetup, reqSetup)
	if recSetup.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on setup, got %d", recSetup.Result().StatusCode)
	}

	// 3. Disable MFA
	reqDisable := httptest.NewRequest("POST", "/vendor/mfa/disable", strings.NewReader("password=VendorPassword123!")).WithContext(ctxWithActor)
	reqDisable.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recDisable := httptest.NewRecorder()
	h.VendorMFADisableSubmit(recDisable, reqDisable)
	if recDisable.Result().StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 see other on disable, got %d", recDisable.Result().StatusCode)
	}

	mfaAfter, _ := idSvc.GetMFAStatus(ctx, user.ID)
	if mfaAfter.Enabled {
		t.Errorf("expected MFA to be disabled after disable submit")
	}
}

func TestSessionRevocation_PermissionsEnforcement(t *testing.T) {
	h, idSvc, _, store := setupTestMFAHandler(t)
	ctx := context.Background()

	// User 1 (Regular Member)
	u1, _, _ := idSvc.Register(ctx, identity.RegisterInput{Email: "emp1@org.eg", Password: "Password123!", NameAr: "موظف 1", Role: "user"})
	// User 2 (Regular Member)
	u2, _, _ := idSvc.Register(ctx, identity.RegisterInput{Email: "emp2@org.eg", Password: "Password123!", NameAr: "موظف 2", Role: "user"})

	// Create sessions in store
	s1 := &identity.Session{Token: "token-user1", UserID: u1.ID, ActiveOrgID: 10}
	s2 := &identity.Session{Token: "token-user2", UserID: u2.ID, ActiveOrgID: 10}
	_ = store.Create(ctx, s1)
	_ = store.Create(ctx, s2)

	// Case 1: User 1 attempts to revoke User 2's session (NOT super admin) -> MUST BE REJECTED
	actorU1 := authctx.Actor{
		UserID:         u1.ID,
		Email:          u1.Email,
		Role:           "user",
		OrganizationID: 10,
		OrgType:        "customer",
	}
	ctxU1 := authctx.WithActor(context.Background(), actorU1)

	reqRevokeU2 := httptest.NewRequest("POST", "/customer/sessions/revoke", strings.NewReader("token=token-user2")).WithContext(ctxU1)
	reqRevokeU2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recRevokeU2 := httptest.NewRecorder()
	h.TenantSessionRevokeSubmit(recRevokeU2, reqRevokeU2)

	loc := recRevokeU2.Result().Header.Get("Location")
	if !strings.Contains(loc, "notice=error") {
		t.Fatalf("expected error notice when regular user tries to revoke another user's session, got %s", loc)
	}

	// Session 2 should still be alive in store
	if sess2, err := store.Get(ctx, "token-user2"); err != nil || sess2 == nil {
		t.Errorf("user 2 session should NOT have been revoked")
	}

	// Case 2: Super Admin attempts to revoke User 2's session -> ALLOWED
	actorSuper := authctx.Actor{
		UserID:         999,
		Email:          "superadmin@dawa24.eg",
		Role:           "super_admin",
		OrganizationID: 10,
		OrgType:        "customer",
	}
	ctxSuper := authctx.WithActor(context.Background(), actorSuper)

	reqRevokeSuper := httptest.NewRequest("POST", "/customer/sessions/revoke", strings.NewReader("token=token-user2")).WithContext(ctxSuper)
	reqRevokeSuper.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recRevokeSuper := httptest.NewRecorder()
	h.TenantSessionRevokeSubmit(recRevokeSuper, reqRevokeSuper)

	locSuper := recRevokeSuper.Result().Header.Get("Location")
	if strings.Contains(locSuper, "notice=error") {
		t.Fatalf("super admin should be allowed to revoke any session in the org, got error %s", locSuper)
	}
}
