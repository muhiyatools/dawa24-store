package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

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
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	IP          string    `json:"ip,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
}

// SessionStore handles session persistence in Redis.
type SessionStore struct {
	redisClient *redis.Client
	cookieName  string
	ttl         time.Duration
	secure      bool
}

// NewSessionStore creates a session store wrapping Redis.
func NewSessionStore(rdb *redis.Client, cfg config.Session) *SessionStore {
	return &SessionStore{
		redisClient: rdb,
		cookieName:  cfg.CookieName,
		ttl:         cfg.TTL,
		secure:      cfg.SecureOnly,
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

	pipe := s.redisClient.TxPipeline()
	pipe.Set(ctx, sessionKey(sess.Token), data, s.ttl)
	pipe.SAdd(ctx, userSessionsKey(sess.UserID), sess.Token)
	pipe.Expire(ctx, userSessionsKey(sess.UserID), s.ttl)

	if _, err := pipe.Exec(ctx); err != nil {
		return apperr.Unavailable("redis", err)
	}
	return nil
}

// Get retrieves a session by token.
func (s *SessionStore) Get(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, apperr.Unauthorized()
	}

	val, err := s.redisClient.Get(ctx, sessionKey(token)).Bytes()
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

	sess, err := s.Get(ctx, token)
	if err == nil && sess != nil {
		s.redisClient.SRem(ctx, userSessionsKey(sess.UserID), token)
	}

	if err := s.redisClient.Del(ctx, sessionKey(token)).Err(); err != nil {
		return apperr.Unavailable("redis", err)
	}
	return nil
}

// DeleteAllForUser invalidates all active sessions for a given user.
func (s *SessionStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	tokens, err := s.redisClient.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Unavailable("redis", err)
	}

	if len(tokens) > 0 {
		keys := make([]string, 0, len(tokens)+1)
		for _, tok := range tokens {
			keys = append(keys, sessionKey(tok))
		}
		keys = append(keys, userSessionsKey(userID))
		if err := s.redisClient.Del(ctx, keys...).Err(); err != nil {
			return apperr.Unavailable("redis", err)
		}
	}
	return nil
}
