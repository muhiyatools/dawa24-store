// Package identity manages user authentication, roles, permissions, sessions,
// and identity security.
package identity

import (
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// User status enum matching PostgreSQL CHECK constraint in 002_identity.
type UserStatus string

const (
	StatusActive    UserStatus = "active"
	StatusInactive  UserStatus = "inactive"
	StatusSuspended UserStatus = "suspended"
	StatusPending   UserStatus = "pending"
)

// MaxFailedLoginsBeforeLockout defines when an account is temporarily locked.
const (
	MaxFailedLoginsBeforeLockout = 5
	DefaultLockoutDuration       = 15 * time.Minute
)

// User represents the core identity entity.
type User struct {
	ID              int64      `json:"id"`
	PublicID        string     `json:"public_id"`
	Email           string     `json:"email"`
	PasswordHash    string     `json:"-"`
	Name            i18n.Text  `json:"name"`
	Role            string     `json:"role"`
	Status          UserStatus `json:"status"`
	Language        i18n.Lang  `json:"language"`
	Timezone        string     `json:"timezone"`
	Phone           string     `json:"phone,omitempty"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	PhoneVerifiedAt *time.Time `json:"phone_verified_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

// UserSecurity tracks login security, attempts, and lockouts.
type UserSecurity struct {
	UserID              int64      `json:"user_id"`
	LoginAttempts       int        `json:"login_attempts"`
	LockedUntil         *time.Time `json:"locked_until,omitempty"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP         string     `json:"last_login_ip,omitempty"`
	LastUserAgent       string     `json:"last_user_agent,omitempty"`
	LastPasswordChange  *time.Time `json:"last_password_change,omitempty"`
	PasswordChangeCount int        `json:"password_change_count"`
	MaxLoginSessions    *int       `json:"max_login_sessions,omitempty"`
}

// UserMFA holds two-factor authentication state.
type UserMFA struct {
	UserID        int64      `json:"user_id"`
	TOTPSecret    []byte     `json:"-"`
	RecoveryCodes []byte     `json:"-"`
	Enabled       bool       `json:"enabled"`
	ConfirmedAt   *time.Time `json:"confirmed_at,omitempty"`
}

// Role represents a platform or organization RBAC role.
type Role struct {
	Key         string    `json:"key"`
	Name        i18n.Text `json:"name"`
	Scope       string    `json:"scope"` // "platform" or "organization"
	IsSystem    bool      `json:"is_system"`
	Description string    `json:"description,omitempty"`
}

// Permission represents a fine-grained capability.
type Permission struct {
	Key         string    `json:"key"`
	Name        i18n.Text `json:"name"`
	Module      string    `json:"module"`
	Description string    `json:"description,omitempty"`
}

// HashPassword hashes a plain-text password using bcrypt (cost 10).
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", apperr.Validation("password.too_short", "Password must be at least 8 characters long.", nil)
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", apperr.Internal(err)
	}
	return string(bytes), nil
}

// CheckPassword verifies a plaintext password against a bcrypt hash ($2y$ or $2a$).
func CheckPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// IsLocked checks whether the account is currently locked out.
func (s *UserSecurity) IsLocked(now time.Time) bool {
	if s == nil || s.LockedUntil == nil {
		return false
	}
	return s.LockedUntil.After(now)
}

// RecordFailedAttempt increments the failed attempt count and applies a lockout if threshold is exceeded.
func (s *UserSecurity) RecordFailedAttempt(now time.Time) {
	if s == nil {
		return
	}
	s.LoginAttempts++
	if s.LoginAttempts >= MaxFailedLoginsBeforeLockout {
		lockout := now.Add(DefaultLockoutDuration)
		s.LockedUntil = &lockout
	}
}

// ResetAttempts clears the failed attempt counter upon a successful authentication.
func (s *UserSecurity) ResetAttempts(now time.Time, ip, userAgent string) {
	if s == nil {
		return
	}
	s.LoginAttempts = 0
	s.LockedUntil = nil
	s.LastLoginAt = &now
	s.LastLoginIP = ip
	s.LastUserAgent = userAgent
}

// ValidateLogin checks if the user's status permits logging in.
func (u *User) ValidateLogin() error {
	if u.DeletedAt != nil {
		return apperr.NotFound("user")
	}
	switch u.Status {
	case StatusActive:
		return nil
	case StatusSuspended:
		return apperr.Forbidden("auth.suspended", "Account has been suspended. Please contact support.")
	case StatusPending:
		return apperr.Forbidden("auth.pending", "Account approval is pending.")
	case StatusInactive:
		return apperr.Forbidden("auth.inactive", "Account is inactive.")
	default:
		return apperr.Unauthorized()
	}
}

// NormalizeEmail cleans and lowercases emails for consistent lookup.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ErrInvalidCredentials is a reusable authentication failure error.
var ErrInvalidCredentials = apperr.New(apperr.KindUnauthorized, "auth.invalid_credentials", "Invalid email or password.")

// UserAddress represents a saved shipping/billing address.
type UserAddress struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Title     string    `json:"title"`
	Recipient string    `json:"recipient"`
	Phone     string    `json:"phone"`
	CityID    int64     `json:"city_id"`
	Address   string    `json:"address"`
	Building  string    `json:"building,omitempty"`
	Floor     string    `json:"floor,omitempty"`
	Apartment string    `json:"apartment,omitempty"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserFavorite represents a bookmarked product.
type UserFavorite struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	ProductID int64     `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
}

// MeResponse represents the authenticated user's profile and active session context.
type MeResponse struct {
	User        *User    `json:"user"`
	ActiveOrgID *int64   `json:"active_org_id,omitempty"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}
