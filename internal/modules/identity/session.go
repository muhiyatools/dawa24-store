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

// redisCmdable is the slice of the Redis client these helpers need. Naming it
// keeps the enforcement functions testable without a live server.
type redisCmdable interface {
	Pipeline() redis.Pipeliner
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
}

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
	cache       *cachepkg.Cache
	cookieName  string
	ttl         time.Duration
	idleTimeout time.Duration
	idleMu      sync.RWMutex
	secure      bool

	memMu           sync.RWMutex
	memSessions     map[string]*Session
	memUserSessions map[int64]map[string]bool
	memOrgSessions  map[int64]map[string]bool
}

// NewSessionStore creates a session store wrapping Redis.
func NewSessionStore(c *cachepkg.Cache, cfg config.Session) *SessionStore {
	idle := cfg.IdleTimeout
	if idle <= 0 {
		idle = 30 * time.Minute
	}
	return &SessionStore{
		cache:           c,
		cookieName:      cfg.CookieName,
		ttl:             cfg.TTL,
		idleTimeout:     idle,
		secure:          cfg.SecureOnly,
		memSessions:     make(map[string]*Session),
		memUserSessions: make(map[int64]map[string]bool),
		memOrgSessions:  make(map[int64]map[string]bool),
	}
}

// SetIdleTimeout dynamically sets the maximum inactivity duration allowed before a session is expired.
func (s *SessionStore) SetIdleTimeout(d time.Duration) {
	if s == nil {
		return
	}
	s.idleMu.Lock()
	defer s.idleMu.Unlock()
	if d > 0 {
		s.idleTimeout = d
	}
}

// GetIdleTimeout returns the active session idle timeout duration.
func (s *SessionStore) GetIdleTimeout() time.Duration {
	if s == nil {
		return 30 * time.Minute
	}
	s.idleMu.RLock()
	defer s.idleMu.RUnlock()
	if s.idleTimeout <= 0 {
		return 30 * time.Minute
	}
	return s.idleTimeout
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

// Enforcing the concurrent-sign-in limit.
//
// This is what made signing in as an administrator hang until the server's
// 30-second write timeout fired and the proxy answered 502, while pharmacy and
// vendor accounts signed in normally.
//
// The old implementation walked the token set one token at a time, calling
// SessionStore.Get for each — a Redis round trip per token, and TWO for a token
// whose session had expired, because a miss also looks up the eviction reason.
// That alone is only slow in proportion to the set.
//
// What made the set grow without bound is that it was never cleaned. A token
// whose session key had expired made Get return an error, so the loop SKIPPED
// it — leaving it a member of the set forever, since only tokens that resolve
// to a live session are ever removed. Meanwhile Create SAdds the new token
// BEFORE enforcement runs and refreshes the set's TTL on every login, so the
// set outlives every session inside it and gains a permanent entry each time.
//
// The result is a ratchet, and it turns fastest on exactly the account that
// signs in most — the administrator's. Every sign-in, successful or timed out,
// added one more dead token that every later sign-in then had to pay two round
// trips to discover. Once the set is a few thousand entries, login exceeds any
// timeout, and each failed attempt makes the next one worse.
//
// Both limits are now enforced with a fixed number of round trips regardless of
// set size, and dead tokens are pruned out of the set as they are found, so a
// set that has already ballooned collapses back to the live sessions on the
// next sign-in and stays there.

// sessionSetEntry is one live session found while enforcing a limit.
type sessionSetEntry struct {
	token   string
	userID  int64
	created time.Time
}

// fetchSessionBlobs reads the stored session for each token.
//
// One MGET per batch rather than one GET per token: the round trips are the
// whole cost, and this is what turns enforcement from proportional-to-set-size
// into a fixed handful of calls. A nil entry means the token has no session.
func (s *SessionStore) fetchSessionBlobs(
	ctx context.Context, rdb redisCmdable, tokens []string,
) ([][]byte, error) {
	blobs := make([][]byte, 0, len(tokens))
	for _, batch := range chunk(tokens, sessionPipelineBatch) {
		keys := make([]string, len(batch))
		for i, tok := range batch {
			keys[i] = sessionKey(tok)
		}
		vals, err := rdb.MGet(ctx, keys...).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		for i := range batch {
			if i >= len(vals) || vals[i] == nil {
				blobs = append(blobs, nil)
				continue
			}
			switch v := vals[i].(type) {
			case string:
				blobs = append(blobs, []byte(v))
			case []byte:
				blobs = append(blobs, v)
			default:
				blobs = append(blobs, nil)
			}
		}
	}
	return blobs, nil
}

// classifySessions splits a token set into the sessions that still exist and
// the tokens that do not.
//
// The dead half is the important one. The previous implementation had no
// concept of it: a token whose session had expired simply failed to load and
// was skipped, and since only live tokens were ever removed from the set, dead
// ones accumulated in it permanently. Returning them is what lets the caller
// prune the set and stop it growing forever.
func classifySessions(tokens []string, blobs [][]byte) (live []sessionSetEntry, dead []string) {
	for i, tok := range tokens {
		if i >= len(blobs) || blobs[i] == nil {
			dead = append(dead, tok)
			continue
		}
		var sess Session
		if json.Unmarshal(blobs[i], &sess) != nil {
			// Unreadable is as good as gone: it can never authenticate anyone.
			dead = append(dead, tok)
			continue
		}
		created := sess.CreatedAt
		if created.IsZero() {
			created = sess.LastActiveAt
		}
		live = append(live, sessionSetEntry{token: tok, userID: sess.UserID, created: created})
	}
	return live, dead
}

// readSessionSet resolves a token set into its live sessions and dead tokens.
func (s *SessionStore) readSessionSet(
	ctx context.Context, rdb redisCmdable, tokens []string,
) (live []sessionSetEntry, dead []string, err error) {
	if len(tokens) == 0 {
		return nil, nil, nil
	}
	blobs, err := s.fetchSessionBlobs(ctx, rdb, tokens)
	if err != nil {
		return nil, nil, err
	}
	live, dead = classifySessions(tokens, blobs)
	return live, dead, nil
}

// sessionPipelineBatch bounds how many keys go into one call.
//
// The round trips are what cost time, so batching is what fixes the hang; the
// cap only stops a set that has already grown to tens of thousands of dead
// tokens from being turned into one enormous request the first time it is
// cleaned up.
const sessionPipelineBatch = 500

func chunk(tokens []string, size int) [][]string {
	if len(tokens) == 0 {
		return nil
	}
	if size <= 0 || len(tokens) <= size {
		return [][]string{tokens}
	}
	out := make([][]string, 0, (len(tokens)+size-1)/size)
	for start := 0; start < len(tokens); start += size {
		end := start + size
		if end > len(tokens) {
			end = len(tokens)
		}
		out = append(out, tokens[start:end])
	}
	return out
}

// runPipelined applies queued writes in bounded batches.
func runPipelined(ctx context.Context, rdb redisCmdable, n int, add func(redis.Pipeliner, int)) error {
	for start := 0; start < n; start += sessionPipelineBatch {
		end := start + sessionPipelineBatch
		if end > n {
			end = n
		}
		pipe := rdb.Pipeline()
		for i := start; i < end; i++ {
			add(pipe, i)
		}
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
			return err
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
	if err != nil {
		return err
	}

	live, dead, err := s.readSessionSet(ctx, rdb, tokens)
	if err != nil {
		return err
	}

	sort.Slice(live, func(i, j int) bool { return live[i].created.Before(live[j].created) })
	evict := live[:evictCount(len(live), max)]

	if err := runPipelined(ctx, rdb, len(dead), func(pipe redis.Pipeliner, i int) {
		pipe.SRem(ctx, orgSessionsKey(orgID), dead[i])
	}); err != nil {
		return err
	}
	return runPipelined(ctx, rdb, len(evict), func(pipe redis.Pipeliner, i int) {
		e := evict[i]
		pipe.Set(ctx, sessionEvictedKey(e.token), "concurrent_limit", 24*time.Hour)
		pipe.SRem(ctx, orgSessionsKey(orgID), e.token)
		pipe.SRem(ctx, userSessionsKey(e.userID), e.token)
		pipe.Del(ctx, sessionKey(e.token))
	})
}

// enforceLimit keeps the user's live session count at or below max by evicting
// the oldest sessions first.
func (s *SessionStore) enforceLimit(ctx context.Context, userID int64, max int) error {
	rdb, err := s.client()
	if err != nil {
		return err
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil {
		return err
	}

	live, dead, err := s.readSessionSet(ctx, rdb, tokens)
	if err != nil {
		return err
	}

	sort.Slice(live, func(i, j int) bool { return live[i].created.Before(live[j].created) })
	evict := live[:evictCount(len(live), max)]

	if err := runPipelined(ctx, rdb, len(dead), func(pipe redis.Pipeliner, i int) {
		pipe.SRem(ctx, userSessionsKey(userID), dead[i])
	}); err != nil {
		return err
	}
	return runPipelined(ctx, rdb, len(evict), func(pipe redis.Pipeliner, i int) {
		e := evict[i]
		pipe.Set(ctx, sessionEvictedKey(e.token), "concurrent_limit", 24*time.Hour)
		pipe.SRem(ctx, userSessionsKey(userID), e.token)
		pipe.Del(ctx, sessionKey(e.token))
	})
}

// evictCount returns how many of the oldest live sessions must go.
func evictCount(liveCount, max int) int {
	if max <= 0 || liveCount <= max {
		return 0
	}
	return liveCount - max
}

