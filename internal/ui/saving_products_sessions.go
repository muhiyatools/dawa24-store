package ui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SavingImportSessionState represents the lifecycle status of an async import.
type SavingImportSessionState string

const (
	SessionStateProcessing SavingImportSessionState = "processing"
	SessionStateReady      SavingImportSessionState = "ready"
	SessionStateCommitted  SavingImportSessionState = "committed"
	SessionStateCancelled  SavingImportSessionState = "cancelled"
	SessionStateFailed     SavingImportSessionState = "failed"
)

// StagedSavingItem represents a single staged row awaiting review and confirmation.
type StagedSavingItem struct {
	Index             int          `json:"index"`
	NameProduct       string       `json:"name_product"`
	SKU               string       `json:"sku"`
	Quantity          float64      `json:"quantity"`
	Price             money.Amount `json:"price"`
	TotalValue        money.Amount `json:"total_value"`
	ProductID         *int64       `json:"product_id,omitempty"`
	MasterProductName string       `json:"master_product_name,omitempty"`
	MasterProductSKU  string       `json:"master_product_sku,omitempty"`
	MatchType         string       `json:"match_type"`
	Confidence        float64      `json:"confidence"`
	Included          bool         `json:"included"`
}

// SavingImportSession holds the complete state of a staged saving products import.
type SavingImportSession struct {
	ID            string                   `json:"id"`
	OrgID         int64                    `json:"org_id"`
	UserID        int64                    `json:"user_id"`
	Filename      string                   `json:"filename"`
	Status        SavingImportSessionState `json:"status"`
	Progress      int                      `json:"progress"` // 0 to 100
	ProgressPhase string                   `json:"progress_phase"`
	ProcessedRows int                      `json:"processed_rows"`
	TotalRows     int                      `json:"total_rows"`
	MatchedRows   int                      `json:"matched_rows"`
	UnlinkedRows  int                      `json:"unlinked_rows"`
	TotalQuantity float64                  `json:"total_quantity"`
	TotalValue    money.Amount             `json:"total_value"`
	ErrorMessage  string                   `json:"error_message,omitempty"`
	Items         []*StagedSavingItem      `json:"items"`
	CreatedAt     time.Time                `json:"created_at"`
	ExpiresAt     time.Time                `json:"expires_at"`
}

// SavingImportSessionStore is an in-memory thread-safe store for async staging sessions.
type SavingImportSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*SavingImportSession
}

var globalSavingImportSessionStore = &SavingImportSessionStore{
	sessions: make(map[string]*SavingImportSession),
}

func init() {
	// Periodic garbage collection of expired sessions
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
		ID:            id,
		OrgID:         orgID,
		UserID:        userID,
		Filename:      filename,
		Status:        SessionStateProcessing,
		Progress:      5,
		ProgressPhase: "قراءة وتحليل ملف الإكسيل",
		TotalRows:     totalRows,
		Items:         make([]*StagedSavingItem, 0, totalRows),
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(2 * time.Hour),
	}

	s.sessions[id] = session
	return session
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

// UpdateProgress updates the progress and phase message.
func (s *SavingImportSessionStore) UpdateProgress(id string, progress int, phase string, processed int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		sess.Progress = progress
		sess.ProgressPhase = phase
		sess.ProcessedRows = processed
	}
}

// CompleteProcessing finalizes processing and marks session as ready.
func (s *SavingImportSessionStore) CompleteProcessing(id string, items []*StagedSavingItem, matched, unlinked int, totalQty float64, totalVal money.Amount) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		sess.Status = SessionStateReady
		sess.Progress = 100
		sess.ProgressPhase = "اكتملت المعالجة — بانتظار مراجعة وتأكيد المستخدم"
		sess.Items = items
		sess.MatchedRows = matched
		sess.UnlinkedRows = unlinked
		sess.TotalQuantity = totalQty
		sess.TotalValue = totalVal
		sess.ProcessedRows = len(items)
	}
}

// FailSession marks a session as failed with an error message.
func (s *SavingImportSessionStore) FailSession(id string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		sess.Status = SessionStateFailed
		sess.Progress = 0
		sess.ProgressPhase = "فشلت المعالجة"
		sess.ErrorMessage = errMsg
	}
}

// CancelSession marks a session as cancelled and discards items.
func (s *SavingImportSessionStore) CancelSession(id string, orgID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok && sess.OrgID == orgID {
		sess.Status = SessionStateCancelled
		sess.Items = nil
		delete(s.sessions, id)
		return true
	}
	return false
}

// CommitSession writes staged items to the database via catalog service.
func (s *SavingImportSessionStore) CommitSession(ctx context.Context, id string, orgID, userID int64, catSvc *catalog.Service) (int, int, error) {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	if !ok || sess.OrgID != orgID || sess.Status != SessionStateReady {
		s.mu.Unlock()
		return 0, 0, fmt.Errorf("جلسة الاستيراد غير صالحة أو منتهية")
	}

	itemsToCommit := make([]*catalog.SavingProduct, 0, len(sess.Items))
	for _, it := range sess.Items {
		if !it.Included {
			continue
		}
		itemsToCommit = append(itemsToCommit, &catalog.SavingProduct{
			OrganizationID: orgID,
			UserID:         &userID,
			ProductID:      it.ProductID,
			NameProduct:    it.NameProduct,
			SKU:            it.SKU,
			Quantity:       it.Quantity,
			Price:          it.Price,
		})
	}
	s.mu.Unlock()

	if len(itemsToCommit) == 0 {
		return 0, 0, fmt.Errorf("لم يتم اختيار أي أصناف للحفظ")
	}

	added, updated, err := catSvc.BatchUpsertSavingProducts(ctx, orgID, &userID, itemsToCommit)
	if err != nil {
		return 0, 0, err
	}

	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok {
		sess.Status = SessionStateCommitted
		sess.Items = nil
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	return added, updated, nil
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
