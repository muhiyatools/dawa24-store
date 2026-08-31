package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	cachepkg "github.com/muhiya/dawa24-store/internal/platform/cache"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Get retrieves a session by token and enforces the idle inactivity timeout.
func (s *SessionStore) Get(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, apperr.Unauthorized()
	}

	idleLimit := s.GetIdleTimeout()

	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		sess, ok := s.memSessions[token]
		if !ok {
			return nil, apperr.Unauthorized()
		}

		lastActive := sess.LastActiveAt
		if lastActive.IsZero() {
			lastActive = sess.CreatedAt
		}

		// Enforce Idle Timeout
		if idleLimit > 0 && time.Since(lastActive) > idleLimit {
			delete(s.memSessions, token)
			if s.memUserSessions[sess.UserID] != nil {
				delete(s.memUserSessions[sess.UserID], token)
			}
			if sess.ActiveOrgID > 0 && s.memOrgSessions[sess.ActiveOrgID] != nil {
				delete(s.memOrgSessions[sess.ActiveOrgID], token)
			}
			return nil, ErrSessionIdleTimeout
		}

		// Debounced activity touch
		now := time.Now().UTC()
		if now.Sub(lastActive) >= 15*time.Second {
			sess.LastActiveAt = now
		}
		return sess, nil
	}

	val, err := rdb.Get(ctx, sessionKey(token)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Check if this token was evicted due to concurrent session limit or idle timeout
			if reason, evErr := rdb.Get(ctx, sessionEvictedKey(token)).Result(); evErr == nil {
				if reason == "concurrent_limit" {
					return nil, ErrSessionEvictedConcurrentLimit
				}
				if reason == "idle_timeout" {
					return nil, ErrSessionIdleTimeout
				}
			}
			return nil, apperr.Unauthorized()
		}
		return nil, apperr.Unavailable("redis", err)
	}

	var sess Session
	if err := json.Unmarshal(val, &sess); err != nil {
		return nil, fmt.Errorf("session: unmarshal: %w", err)
	}

	lastActive := sess.LastActiveAt
	if lastActive.IsZero() {
		lastActive = sess.CreatedAt
	}

	// Enforce Idle Timeout
	if idleLimit > 0 && time.Since(lastActive) > idleLimit {
		_ = s.Delete(ctx, token)
		_ = rdb.Set(ctx, sessionEvictedKey(token), "idle_timeout", 24*time.Hour).Err()
		return nil, ErrSessionIdleTimeout
	}

	// Debounced activity update in Redis
	now := time.Now().UTC()
	if now.Sub(lastActive) >= 15*time.Second {
		sess.LastActiveAt = now
		if data, mErr := json.Marshal(&sess); mErr == nil {
			remTTL, tErr := rdb.TTL(ctx, sessionKey(token)).Result()
			if tErr == nil && remTTL > 0 {
				_ = rdb.Set(ctx, sessionKey(token), data, remTTL).Err()
			} else {
				_ = rdb.Set(ctx, sessionKey(token), data, s.ttl).Err()
			}
		}
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
