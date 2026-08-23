package identity

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Service encapsulates authentication and identity business workflows.
type Service struct {
	repo         Repository
	sessionStore *SessionStore
	log          *slog.Logger
}

// NewService creates a new Identity Service.
func NewService(repo Repository, sessionStore *SessionStore, log *slog.Logger) *Service {
	return &Service{
		repo:         repo,
		sessionStore: sessionStore,
		log:          log,
	}
}

// RegisterInput captures parameters required for creating an account.
type RegisterInput struct {
	Email    string    `json:"email"`
	Password string    `json:"password"`
	NameAr   string    `json:"name_ar"`
	NameEn   string    `json:"name_en"`
	Role     string    `json:"role,omitempty"`
	Language i18n.Lang `json:"language,omitempty"`
	Timezone string    `json:"timezone,omitempty"`
	Phone    string    `json:"phone,omitempty"`
}

// LoginInput captures authentication parameters.
type LoginInput struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	OrgID     int64  `json:"org_id,omitempty"`
	IP        string `json:"-"`
	UserAgent string `json:"-"`
}

// LoginResult contains the output of a successful login or an MFA prompt.
type LoginResult struct {
	User        *User    `json:"user"`
	Session     *Session `json:"session,omitempty"`
	RequiresMFA bool     `json:"requires_mfa"`
}

// Register creates a new user account and sets up security tracking.
func (s *Service) Register(ctx context.Context, input RegisterInput) (*User, *Session, error) {
	cleanEmail := NormalizeEmail(input.Email)
	if cleanEmail == "" || !stringsContains(cleanEmail, "@") {
		return nil, nil, apperr.Validation("email.invalid", "A valid email address is required.", nil)
	}

	hash, err := HashPassword(input.Password)
	if err != nil {
		return nil, nil, err
	}

	role := input.Role
	if role != "support" && role != "admin" && role != "super_admin" && role != "developer" {
		role = "user"
	}

	lang := input.Language
	if lang == "" {
		lang = i18n.Default
	}

	tz := input.Timezone
	if tz == "" {
		tz = "Africa/Cairo"
	}

	user := &User{
		Email:        cleanEmail,
		PasswordHash: hash,
		Name:         i18n.New(input.NameAr, input.NameEn),
		Role:         role,
		Status:       StatusActive,
		Language:     lang,
		Timezone:     tz,
		Phone:        input.Phone,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, nil, err
	}

	// Initialize security record
	sec := &UserSecurity{
		UserID:        user.ID,
		LoginAttempts: 0,
	}
	_ = s.repo.UpsertSecurity(ctx, sec)

	// Issue initial session
	permissions, err := s.repo.GetPermissionsForUser(ctx, user.ID, 0)
	if err != nil {
		permissions = []string{}
	}

	sess := &Session{
		UserID:      user.ID,
		PublicID:    user.PublicID,
		Email:       user.Email,
		Role:        user.Role,
		Permissions: permissions,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, nil, err
		}
	}

	s.log.InfoContext(ctx, "user registered", "user_id", user.ID, "email", user.Email, "role", user.Role)
	return user, sess, nil
}

// Login authenticates a user against credentials, lockouts, and MFA requirements.
func (s *Service) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	now := time.Now().UTC()
	cleanEmail := NormalizeEmail(input.Email)

	user, err := s.repo.GetUserByEmail(ctx, cleanEmail)
	if err != nil {
		if errors.Is(err, apperr.NotFound("user")) || apperr.KindOf(err) == apperr.KindNotFound {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Check login security and lockout
	sec, err := s.repo.GetSecurity(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	if sec.IsLocked(now) {
		s.log.WarnContext(ctx, "login attempt on locked account", "user_id", user.ID, "locked_until", sec.LockedUntil)
		return nil, apperr.Forbidden("auth.locked", "Account is temporarily locked due to multiple failed login attempts. Please try again later.")
	}

	if err := user.ValidateLogin(); err != nil {
		return nil, err
	}

	if !CheckPassword(user.PasswordHash, input.Password) {
		sec.RecordFailedAttempt(now)
		_ = s.repo.UpsertSecurity(ctx, sec)
		s.log.WarnContext(ctx, "failed login attempt", "user_id", user.ID, "attempts", sec.LoginAttempts)
		return nil, ErrInvalidCredentials
	}

	// Password is correct; reset attempts
	sec.ResetAttempts(now, input.IP, input.UserAgent)
	_ = s.repo.UpsertSecurity(ctx, sec)

	// Check MFA
	mfa, err := s.repo.GetMFA(ctx, user.ID)
	if err == nil && mfa != nil && mfa.Enabled {
		return &LoginResult{
			User:        user,
			RequiresMFA: true,
		}, nil
	}

	// Resolve organization context & permissions
	var orgID int64
	var orgType, orgStatus string
	if input.OrgID > 0 {
		belongs, err := s.repo.UserBelongsToOrg(ctx, user.ID, input.OrgID)
		if err != nil {
			s.log.WarnContext(ctx, "verify user belongs to org", "user_id", user.ID, "org_id", input.OrgID, "error", err)
		} else if belongs {
			orgID = input.OrgID
		}
	}
	// No organization named - and the web sign-in form has no field for one, so
	// this is the usual case. Fall back to the user's own membership rather
	// than leaving the session on organization 0, which every tenant-scoped
	// query then filters against and finds nothing.
	if orgID == 0 {
		if o, t, st, err := s.repo.DefaultOrgInfoForUser(ctx, user.ID); err == nil {
			orgID, orgType, orgStatus = o, t, st
		} else {
			s.log.WarnContext(ctx, "could not resolve default organization at login",
				"error", err, "user_id", user.ID)
		}
	}

	if orgType != "" {
		normType, wasLegacy := NormalizeOrgType(orgType)
		if wasLegacy {
			s.log.WarnContext(ctx, "legacy organization type normalized at login",
				"original", orgType, "normalized", normType, "user_id", user.ID)
		}
		orgType = normType
	}

	permissions, err := s.repo.GetPermissionsForUser(ctx, user.ID, orgID)

	if err != nil {
		permissions = []string{}
	}

	sess := &Session{
		UserID:           user.ID,
		PublicID:         user.PublicID,
		Email:            user.Email,
		Role:             user.Role,
		ActiveOrgID:      orgID,
		OrgType:          orgType,
		OrgStatus:        orgStatus,
		Permissions:      permissions,
		IP:               input.IP,
		UserAgent:        input.UserAgent,
		MaxLoginSessions: sec.MaxLoginSessions,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, err
		}
	}

	s.log.InfoContext(ctx, "user logged in", "user_id", user.ID, "active_org", orgID)
	return &LoginResult{
		User:    user,
		Session: sess,
	}, nil
}

// ChangePassword verifies current password and updates to the new hashed password.
func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !CheckPassword(user.PasswordHash, currentPassword) {
		return apperr.Validation("auth.invalid_password", "كلمة المرور الحالية غير صحيحة.", nil)
	}
	if len(newPassword) < 8 {
		return apperr.Validation("auth.weak_password", "كلمة المرور الجديدة يجب أن تكون 8 أحرف على الأقل.", nil)
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	user.UpdatedAt = time.Now()
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "user password updated", "user_id", userID)
	return nil
}

// Logout terminates a session.
func (s *Service) Logout(ctx context.Context, token string) error {

	if s.sessionStore == nil {
		return nil
	}
	return s.sessionStore.Delete(ctx, token)
}

// ListSessions returns the user's active sessions, newest first.
func (s *Service) ListSessions(ctx context.Context, userID int64) ([]*Session, error) {
	if s.sessionStore == nil {
		return nil, nil
	}
	return s.sessionStore.ListForUser(ctx, userID)
}

// RevokeSession invalidates one of the user's own sessions.
func (s *Service) RevokeSession(ctx context.Context, token string, userID int64) error {
	if s.sessionStore == nil {
		return nil
	}
	sess, err := s.sessionStore.Get(ctx, token)
	if err != nil {
		return err
	}
	if sess.UserID != userID {
		return apperr.Forbidden("session.not_owner", "You can only revoke your own sessions.")
	}
	return s.sessionStore.Delete(ctx, token)
}

// ValidateSession verifies a session token.
func (s *Service) ValidateSession(ctx context.Context, token string) (*Session, error) {
	if s.sessionStore == nil {
		return nil, apperr.Unauthorized()
	}
	return s.sessionStore.Get(ctx, token)
}

// GetUserByID looks up a user by ID.
func (s *Service) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

// GetUserByEmail looks up a user by email address.
func (s *Service) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.repo.GetUserByEmail(ctx, email)
}

// UserBelongsToOrg reports whether a user is a member of an organization.
//
// This backs the tenant-switch check in ResolveTenant. Without it, a caller
// could name any organization in the X-Dawa-Org-ID header and row-level
// security would faithfully scope every query to that tenant's data — the
// isolation mechanism working perfectly against an attacker-supplied input.
func (s *Service) UserBelongsToOrg(ctx context.Context, userID, orgID int64) (bool, error) {
	return s.repo.UserBelongsToOrg(ctx, userID, orgID)
}

// ResolvePermissions computes effective permissions for a user within an org.
func (s *Service) ResolvePermissions(ctx context.Context, userID int64, orgID int64) ([]string, error) {
	return s.repo.GetPermissionsForUser(ctx, userID, orgID)
}

// ListUserOrganizations lists all organizations a user has active membership in.
func (s *Service) ListUserOrganizations(ctx context.Context, userID int64) ([]*UserOrgMembership, error) {
	return s.repo.ListUserOrganizations(ctx, userID)
}

// SwitchActiveOrg verifies membership in target organization and constructs an updated active Session.
func (s *Service) SwitchActiveOrg(ctx context.Context, userID, targetOrgID int64) (*Session, error) {
	belongs, err := s.repo.UserBelongsToOrg(ctx, userID, targetOrgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, apperr.Forbidden("auth.org_unauthorized", "User does not have access to this organization.")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	perms, err := s.repo.GetPermissionsForUser(ctx, userID, targetOrgID)
	if err != nil {
		perms = []string{}
	}

	// Lookup target org info
	orgs, err := s.repo.ListUserOrganizations(ctx, userID)
	if err != nil {
		return nil, err
	}

	var orgType, orgStatus string
	for _, o := range orgs {
		if o.OrganizationID == targetOrgID {
			orgType = o.OrgType
			orgStatus = o.OrgStatus
			break
		}
	}

	normType, wasLegacy := NormalizeOrgType(orgType)
	if wasLegacy {
		s.log.WarnContext(ctx, "legacy organization type normalized on switch",
			"original", orgType, "normalized", normType, "user_id", userID, "org_id", targetOrgID)
	}
	orgType = normType

	sess := &Session{
		UserID:      user.ID,
		PublicID:    user.PublicID,
		Email:       user.Email,
		Role:        user.Role,
		ActiveOrgID: targetOrgID,
		OrgType:     orgType,
		OrgStatus:   orgStatus,
		Permissions: perms,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, err
		}
	}

	s.log.InfoContext(ctx, "switched active organization", "user_id", userID, "org_id", targetOrgID, "org_type", orgType)
	return sess, nil
}

// GetOrgInfoForUser retrieves and normalizes organization details and permissions for a user membership.
func (s *Service) GetOrgInfoForUser(ctx context.Context, userID, orgID int64) (orgType, orgStatus string, perms []string, err error) {
	orgs, err := s.repo.ListUserOrganizations(ctx, userID)
	if err != nil {
		return "", "", nil, err
	}
	found := false
	for _, o := range orgs {
		if o.OrganizationID == orgID {
			orgType = o.OrgType
			orgStatus = o.OrgStatus
			found = true
			break
		}
	}
	if !found {
		return "", "", nil, apperr.Forbidden("org.not_a_member", "You are not a member of that organization.")
	}
	normType, wasLegacy := NormalizeOrgType(orgType)
	if wasLegacy {
		s.log.WarnContext(ctx, "legacy organization type normalized on membership resolution",
			"original", orgType, "normalized", normType, "user_id", userID, "org_id", orgID)
	}
	orgType = normType
	perms, _ = s.repo.GetPermissionsForUser(ctx, userID, orgID)
	return orgType, orgStatus, perms, nil
}

func stringsContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
