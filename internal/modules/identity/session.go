package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"

	cachepkg "github.com/muhiya/dawa24-store/internal/platform/cache"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Session holds the active user session stored in Redis.
type Session struct {
	Token       string    `json:"token"`
	UserID      int64     `json:"user_id"`
	PublicID    string    `json:"public_id"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	ActiveOrgID int64     `json:"active_org_id,omitempty"`
	OrgType     string    `json:"org_type,omitempty"`
	OrgStatus   string    `json:"org_status,omitempty"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	IP          string    `json:"ip,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	// MaxLoginSessions, when set, is the concurrent-sign-in limit enforced by
	// SessionStore.Create (evicting the oldest session beyond the limit).
	MaxLoginSessions *int `json:"max_login_sessions,omitempty"`
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
}

// NewSessionStore creates a session store wrapping Redis.
func NewSessionStore(c *cachepkg.Cache, cfg config.Session) *SessionStore {
	return &SessionStore{
		cache:      c,
		cookieName: cfg.CookieName,
		ttl:        cfg.TTL,
		secure:     cfg.SecureOnly,
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

func userSessionsKey(userID int64) string {
	return fmt.Sprintf("user_sessions:%d", userID)
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
	sess.ExpiresAt = sess.CreatedAt.Add(s.ttl)

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}

	rdb, err := s.client()
	if err != nil {
		return err
	}
	pipe := rdb.TxPipeline()
	pipe.Set(ctx, sessionKey(sess.Token), data, s.ttl)
	pipe.SAdd(ctx, userSessionsKey(sess.UserID), sess.Token)
	pipe.Expire(ctx, userSessionsKey(sess.UserID), s.ttl)

	if _, err := pipe.Exec(ctx); err != nil {
		return apperr.Unavailable("redis", err)
	}

	// Enforce the concurrent-sign-in limit: if this session exceeds it, evict
	// the oldest sessions until the user is back within budget. This is the
	// licensing boundary for the session plans (Phase 4.6).
	if sess.MaxLoginSessions != nil && *sess.MaxLoginSessions > 0 {
		if err := s.enforceLimit(ctx, sess.UserID, *sess.MaxLoginSessions); err != nil {
			return err
		}
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
		_ = rdb.SRem(ctx, userSessionsKey(userID), sessions[i].token)
		_ = rdb.Del(ctx, sessionKey(sessions[i].token))
	}
	return nil
}

// ListForUser returns the user's live sessions, newest first.
func (s *SessionStore) ListForUser(ctx context.Context, userID int64) ([]*Session, error) {
	rdb, err := s.client()
	if err != nil {
		return nil, err
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

// Get retrieves a session by token.
func (s *SessionStore) Get(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, apperr.Unauthorized()
	}

	rdb, err := s.client()
	if err != nil {
		return nil, err
	}
	val, err := rdb.Get(ctx, sessionKey(token)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
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
		return err
	}

	sess, err := s.Get(ctx, token)
	if err == nil && sess != nil {
		rdb.SRem(ctx, userSessionsKey(sess.UserID), token)
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
		return err
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
