package ui

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"sort"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

type SavingImportSession = pages.SavingImportSession
type SavingRowFilter = pages.SavingRowFilter
type StagedSavingItem = pages.StagedSavingItem
type SavingDetectedCols = pages.SavingDetectedCols
type SavingImportPhase = pages.SavingImportPhase
type SessionState = pages.SessionState

const (
	SavingPhaseUpload    = pages.SavingPhaseUpload
	SavingPhaseMapping   = pages.SavingPhaseMapping
	SavingPhaseReview    = pages.SavingPhaseReview
	SavingPhaseCompleted = pages.SavingPhaseCompleted

	SessionStateUploaded   = pages.SessionStateUploaded
	SessionStateProcessing = pages.SessionStateProcessing
	SessionStateReady      = pages.SessionStateReady
	SessionStateCommitted  = pages.SessionStateCommitted
	SessionStateCancelled  = pages.SessionStateCancelled
	SessionStateFailed     = pages.SessionStateFailed
)

// SavingImportSessionStore is an in-memory thread-safe store for async staging sessions.
type SavingImportSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*SavingImportSession
}

var globalSavingImportSessionStore = &SavingImportSessionStore{
	sessions: make(map[string]*SavingImportSession),
}

func init() {
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		for range ticker.C {
			globalSavingImportSessionStore.cleanupExpired()
		}
	}()
}

// NewSession creates a new session in processing state.
func (s *SavingImportSessionStore) NewSession(orgID, userID int64, filename string, totalRows int) *SavingImportSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)

	session := &SavingImportSession{
		Success:       true,
		ID:            id,
		OrgID:         orgID,
		UserID:        userID,
		Filename:      filename,
		Status:        SessionStateProcessing,
		Phase:         SavingPhaseReview,
		Progress:      5,
		ProgressPhase: i18n.TDefault("w4_ui.s_94_94"),
		TotalRows:     totalRows,
		Items:         make([]*StagedSavingItem, 0, totalRows),
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(4 * time.Hour),
	}

	s.sessions[id] = session
	return session
}

// ListSessions returns all active sessions of an organization, sorted newest first.
func (s *SavingImportSessionStore) ListSessions(orgID int64) []*SavingImportSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*SavingImportSession
	for _, sess := range s.sessions {
		if sess.OrgID == orgID && time.Now().Before(sess.ExpiresAt) {
			list = append(list, sess)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

// GetSession retrieves a session by ID with ownership check.
func (s *SavingImportSessionStore) GetSession(id string, orgID int64) (*SavingImportSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[id]
	if !ok || sess.OrgID != orgID || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

func (s *SavingImportSessionStore) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}
