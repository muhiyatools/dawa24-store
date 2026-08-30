package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	cachepkg "github.com/muhiya/dawa24-store/internal/platform/cache"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Session holds the active user session stored in Redis.
type Session struct {
	Token    string `json:"token"`
	UserID   int64  `json:"user_id"`
	PublicID string `json:"public_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	// StaffRole records whether the platform role this session was issued
	// under reaches the admin dashboard. It is written at login from
	// identity.roles.is_staff, so a role an operator invented after this code
	// was written is recognised as staff without a code change.
	StaffRole    bool      `json:"staff_role"`
	ActiveOrgID  int64     `json:"active_org_id,omitempty"`
	OrgType      string    `json:"org_type,omitempty"`
	OrgStatus    string    `json:"org_status,omitempty"`
	Permissions  []string  `json:"permissions"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	IP           string    `json:"ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	DeviceName   string    `json:"device_name,omitempty"`
	DeviceType   string    `json:"device_type,omitempty"`
	Browser      string    `json:"browser,omitempty"`
	OS           string    `json:"os,omitempty"`
	Icon         string    `json:"icon,omitempty"`
	LastActiveAt time.Time `json:"last_active_at,omitempty"`
	// MaxLoginSessions, when set, is the concurrent-sign-in limit enforced by
	// SessionStore.Create (evicting the oldest session beyond the limit).
	MaxLoginSessions *int `json:"max_login_sessions,omitempty"`
}

// IsStaff reports whether this session belongs to platform staff. Platform
// admin is staff, not an account type (Rebuild V2 rule 1); an organization
// member's capability comes from the membership, never from the platform role.
//
// The answer comes from StaffRole, stamped at login from the role row. The
// four hardcoded names remain only as a fallback for sessions issued before
// that field existed — without it, every logged-in administrator would be
// bounced out of /admin the moment this version deployed.
func (s *Session) IsStaff() bool {
	if s.StaffRole {
		return true
	}
	return s.Role == "super_admin" || s.Role == "admin" || s.Role == "support" || s.Role == "developer"
}

// SessionStore handles session persistence in Redis.
// SessionStore holds the cache handle rather than a redis.Client.
//
// The client does not exist when routes are mounted — the server starts before
// its dependencies connect — so capturing one at construction time captures nil
// forever. Asking the handle at each use gets whatever is live now.
type SessionStore struct {
	cache      *cachepkg.Cache
	cookieName string
	ttl        time.Duration
	secure     bool

	memMu           sync.RWMutex
	memSessions     map[string]*Session
	memUserSessions map[int64]map[string]bool
	memOrgSessions  map[int64]map[string]bool
}

// NewSessionStore creates a session store wrapping Redis.
func NewSessionStore(c *cachepkg.Cache, cfg config.Session) *SessionStore {
	return &SessionStore{
		cache:           c,
		cookieName:      cfg.CookieName,
		ttl:             cfg.TTL,
		secure:          cfg.SecureOnly,
		memSessions:     make(map[string]*Session),
		memUserSessions: make(map[int64]map[string]bool),
		memOrgSessions:  make(map[int64]map[string]bool),
	}
}

// GenerateToken generates a cryptographically secure 32-byte hex token.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session: generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func sessionKey(token string) string {
	return fmt.Sprintf("session:%s", token)
}

func sessionEvictedKey(token string) string {
	return fmt.Sprintf("session_evicted:%s", token)
}

func userSessionsKey(userID int64) string {
	return fmt.Sprintf("user_sessions:%d", userID)
}

func orgSessionsKey(orgID int64) string {
	return fmt.Sprintf("org_sessions:%d", orgID)
}

// Create stores a new session in Redis with configured TTL.
func (s *SessionStore) Create(ctx context.Context, sess *Session) error {
	if sess.Token == "" {
		token, err := GenerateToken()
		if err != nil {
			return err
		}
		sess.Token = token
	}

	sess.CreatedAt = time.Now().UTC()
	sess.LastActiveAt = sess.CreatedAt
	sess.ExpiresAt = sess.CreatedAt.Add(s.ttl)

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}

	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if s.memSessions == nil {
			s.memSessions = make(map[string]*Session)
			s.memUserSessions = make(map[int64]map[string]bool)
			s.memOrgSessions = make(map[int64]map[string]bool)
		}
		s.memSessions[sess.Token] = sess
		if s.memUserSessions[sess.UserID] == nil {
			s.memUserSessions[sess.UserID] = make(map[string]bool)
		}
		s.memUserSessions[sess.UserID][sess.Token] = true
		if sess.ActiveOrgID > 0 {
			if s.memOrgSessions[sess.ActiveOrgID] == nil {
				s.memOrgSessions[sess.ActiveOrgID] = make(map[string]bool)
			}
			s.memOrgSessions[sess.ActiveOrgID][sess.Token] = true
		}
		return nil
	}
	pipe := rdb.TxPipeline()
	pipe.Set(ctx, sessionKey(sess.Token), data, s.ttl)
	pipe.SAdd(ctx, userSessionsKey(sess.UserID), sess.Token)
	pipe.Expire(ctx, userSessionsKey(sess.UserID), s.ttl)
	if sess.ActiveOrgID > 0 {
		pipe.SAdd(ctx, orgSessionsKey(sess.ActiveOrgID), sess.Token)
		pipe.Expire(ctx, orgSessionsKey(sess.ActiveOrgID), s.ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return apperr.Unavailable("redis", err)
	}

	// Enforce the concurrent-sign-in limit:
	// If the user belongs to an organization, enforce the limit across the entire organization!
	// All staff members belonging to the organization share the organization subscription's concurrent session limit.
	if sess.MaxLoginSessions != nil && *sess.MaxLoginSessions > 0 {
		if sess.ActiveOrgID > 0 {
			if err := s.enforceOrgLimit(ctx, sess.ActiveOrgID, *sess.MaxLoginSessions); err != nil {
				return err
			}
		} else {
			if err := s.enforceLimit(ctx, sess.UserID, *sess.MaxLoginSessions); err != nil {
				return err
			}
		}
	}
	return nil
}

// enforceOrgLimit keeps the organization's total live session count at or below max by evicting
// the oldest active sessions across all members of that organization.
func (s *SessionStore) enforceOrgLimit(ctx context.Context, orgID int64, max int) error {
	rdb, err := s.client()
	if err != nil {
		return err
	}
	tokens, err := rdb.SMembers(ctx, orgSessionsKey(orgID)).Result()
	if err != nil || len(tokens) <= max {
		return err
	}

	type live struct {
		token   string
		userID  int64
		created time.Time
	}
	var sessions []live
	for _, tok := range tokens {
		if sess, err := s.Get(ctx, tok); err == nil && sess != nil {
			sessions = append(sessions, live{token: tok, userID: sess.UserID, created: sess.CreatedAt})
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].created.Before(sessions[j].created) })

	toEvict := len(sessions) - max
	for i := 0; i < toEvict && i < len(sessions); i++ {
		tok := sessions[i].token
		_ = rdb.Set(ctx, sessionEvictedKey(tok), "concurrent_limit", 24*time.Hour).Err()
		_ = rdb.SRem(ctx, orgSessionsKey(orgID), tok)
		_ = rdb.SRem(ctx, userSessionsKey(sessions[i].userID), tok)
		_ = rdb.Del(ctx, sessionKey(tok))
	}
	return nil
}

// enforceLimit keeps the user's live session count at or below max by evicting
// the oldest sessions first.
func (s *SessionStore) enforceLimit(ctx context.Context, userID int64, max int) error {
	rdb, err := s.client()
	if err != nil {
		return err
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil || len(tokens) <= max {
		return err
	}

	type live struct {
		token   string
		created time.Time
	}
	var sessions []live
	for _, tok := range tokens {
		if sess, err := s.Get(ctx, tok); err == nil && sess != nil {
			sessions = append(sessions, live{token: tok, created: sess.CreatedAt})
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].created.Before(sessions[j].created) })

	toEvict := len(sessions) - max
	for i := 0; i < toEvict && i < len(sessions); i++ {
		tok := sessions[i].token
		_ = rdb.Set(ctx, sessionEvictedKey(tok), "concurrent_limit", 24*time.Hour).Err()
		_ = rdb.SRem(ctx, userSessionsKey(userID), tok)
		_ = rdb.Del(ctx, sessionKey(tok))
	}
	return nil
}

// ListForUser returns the user's live sessions, newest first.
func (s *SessionStore) ListForUser(ctx context.Context, userID int64) ([]*Session, error) {
	rdb, err := s.client()
	if err != nil {
		s.memMu.RLock()
		defer s.memMu.RUnlock()
		var list []*Session
		if set, ok := s.memUserSessions[userID]; ok {
			for tok := range set {
				if sess, exists := s.memSessions[tok]; exists {
					list = append(list, sess)
				}
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
		return list, nil
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, apperr.Unavailable("redis", err)
	}
	var list []*Session
	for _, tok := range tokens {
		if sess, err := s.Get(ctx, tok); err == nil && sess != nil {
			list = append(list, sess)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

// ListForOrg returns all live sessions across an entire organization, newest first.
func (s *SessionStore) ListForOrg(ctx context.Context, orgID int64) ([]*Session, error) {
	rdb, err := s.client()
	if err != nil {
		s.memMu.RLock()
		defer s.memMu.RUnlock()
		var list []*Session
		if set, ok := s.memOrgSessions[orgID]; ok {
			for tok := range set {
				if sess, exists := s.memSessions[tok]; exists {
					list = append(list, sess)
				}
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
		return list, nil
	}
	tokens, err := rdb.SMembers(ctx, orgSessionsKey(orgID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, apperr.Unavailable("redis", err)
	}
	var list []*Session
	for _, tok := range tokens {
		if sess, err := s.Get(ctx, tok); err == nil && sess != nil {
			list = append(list, sess)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

// Get retrieves a session by token.
func (s *SessionStore) Get(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, apperr.Unauthorized()
	}

	rdb, err := s.client()
	if err != nil {
		s.memMu.RLock()
		defer s.memMu.RUnlock()
		if sess, ok := s.memSessions[token]; ok {
			return sess, nil
		}
		return nil, apperr.Unauthorized()
	}
	val, err := rdb.Get(ctx, sessionKey(token)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Check if this token was evicted due to concurrent session limit
			if reason, evErr := rdb.Get(ctx, sessionEvictedKey(token)).Result(); evErr == nil && reason == "concurrent_limit" {
				return nil, ErrSessionEvictedConcurrentLimit
			}
			return nil, apperr.Unauthorized()
		}
		return nil, apperr.Unavailable("redis", err)
	}

	var sess Session
	if err := json.Unmarshal(val, &sess); err != nil {
		return nil, fmt.Errorf("session: unmarshal: %w", err)
	}

	return &sess, nil
}

// Delete invalidates a session token.
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if sess, ok := s.memSessions[token]; ok {
			delete(s.memSessions, token)
			if s.memUserSessions[sess.UserID] != nil {
				delete(s.memUserSessions[sess.UserID], token)
			}
			if sess.ActiveOrgID > 0 && s.memOrgSessions[sess.ActiveOrgID] != nil {
				delete(s.memOrgSessions[sess.ActiveOrgID], token)
			}
		}
		return nil
	}

	sess, err := s.Get(ctx, token)
	if err == nil && sess != nil {
		rdb.SRem(ctx, userSessionsKey(sess.UserID), token)
		if sess.ActiveOrgID > 0 {
			rdb.SRem(ctx, orgSessionsKey(sess.ActiveOrgID), token)
		}
	}

	if err := rdb.Del(ctx, sessionKey(token)).Err(); err != nil {
		return apperr.Unavailable("redis", err)
	}
	return nil
}

// DeleteAllForUser invalidates all active sessions for a given user.
func (s *SessionStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if set, ok := s.memUserSessions[userID]; ok {
			for tok := range set {
				delete(s.memSessions, tok)
			}
			delete(s.memUserSessions, userID)
		}
		return nil
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Unavailable("redis", err)
	}

	if len(tokens) > 0 {
		keys := make([]string, 0, len(tokens)+1)
		for _, tok := range tokens {
			keys = append(keys, sessionKey(tok))
		}
		keys = append(keys, userSessionsKey(userID))
		if err := rdb.Del(ctx, keys...).Err(); err != nil {
			return apperr.Unavailable("redis", err)
		}
	}
	return nil
}

// DeleteAllOtherForOrg revokes all active sessions for an organization EXCEPT the given current token.
func (s *SessionStore) DeleteAllOtherForOrg(ctx context.Context, orgID int64, currentToken string) error {
	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if set, ok := s.memOrgSessions[orgID]; ok {
			for tok := range set {
				if tok != currentToken {
					if sess, ok := s.memSessions[tok]; ok {
						if s.memUserSessions[sess.UserID] != nil {
							delete(s.memUserSessions[sess.UserID], tok)
						}
					}
					delete(s.memSessions, tok)
					delete(set, tok)
				}
			}
		}
		return nil
	}
	tokens, err := rdb.SMembers(ctx, orgSessionsKey(orgID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Unavailable("redis", err)
	}

	for _, tok := range tokens {
		if tok != currentToken && tok != "" {
			_ = s.Delete(ctx, tok)
		}
	}
	return nil
}

// DeleteAllOtherForUser revokes all active sessions for a user EXCEPT the given current token.
func (s *SessionStore) DeleteAllOtherForUser(ctx context.Context, userID int64, currentToken string) error {
	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if set, ok := s.memUserSessions[userID]; ok {
			for tok := range set {
				if tok != currentToken {
					if sess, ok := s.memSessions[tok]; ok {
						if sess.ActiveOrgID > 0 && s.memOrgSessions[sess.ActiveOrgID] != nil {
							delete(s.memOrgSessions[sess.ActiveOrgID], tok)
						}
					}
					delete(s.memSessions, tok)
					delete(set, tok)
				}
			}
		}
		return nil
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Unavailable("redis", err)
	}

	for _, tok := range tokens {
		if tok != currentToken && tok != "" {
			_ = s.Delete(ctx, tok)
		}
	}
	return nil
}

// client resolves the live Redis client, or reports that sessions are
// unavailable because the cache has not connected yet.
func (s *SessionStore) client() (*redis.Client, error) {
	if s == nil || s.cache == nil {
		return nil, apperr.Unavailable("session", cachepkg.ErrNotConnected)
	}
	rdb := s.cache.Redis()
	if rdb == nil {
		return nil, apperr.Unavailable("session", cachepkg.ErrNotConnected)
	}
	return rdb, nil
}
