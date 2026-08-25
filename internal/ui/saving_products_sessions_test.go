package ui

import (
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestSavingImportSessionLifecycle(t *testing.T) {
	store := &SavingImportSessionStore{
		sessions: make(map[string]*SavingImportSession),
	}

	orgID := int64(101)
	userID := int64(202)

	// 1. Create session
	sess := store.NewSession(orgID, userID, "test_file.xlsx", 50)
	if sess == nil || sess.ID == "" {
		t.Fatalf("expected valid session, got nil")
	}
	if sess.Status != SessionStateProcessing {
		t.Fatalf("expected status processing, got %s", sess.Status)
	}

	// 2. Query session
	fetched, ok := store.GetSession(sess.ID, orgID)
	if !ok || fetched == nil {
		t.Fatalf("expected to get session from store")
	}

	// 3. Update Progress
	store.UpdateProgress(sess.ID, 45, "جاري المطابقة الذكية", 25)
	fetched, _ = store.GetSession(sess.ID, orgID)
	if fetched.Progress != 45 || fetched.ProcessedRows != 25 {
		t.Fatalf("expected progress 45 and 25 rows, got %d, %d", fetched.Progress, fetched.ProcessedRows)
	}

	// 4. Complete Processing
	pID := int64(999)
	items := []*StagedSavingItem{
		{
			Index:             1,
			NameProduct:       "بنادول اكسترا",
			SKU:               "622123456789",
			Quantity:          10,
			Price:             money.FromMinor(5000),
			TotalValue:        money.FromMinor(50000),
			ProductID:         &pID,
			MasterProductName: "Panadol Extra 500mg",
			MatchType:         "exact_sku",
			Confidence:        1.0,
			Included:          true,
		},
		{
			Index:       2,
			NameProduct: "صنف تجريبي غير مسجل",
			SKU:         "UNLINKED-01",
			Quantity:    5,
			Price:       money.FromMinor(2000),
			TotalValue:  money.FromMinor(10000),
			ProductID:   nil,
			MatchType:   "unlinked",
			Confidence:  0.0,
			Included:    true,
		},
	}

	store.CompleteProcessing(sess.ID, items, 1, 1, 15, money.FromMinor(60000))
	fetched, _ = store.GetSession(sess.ID, orgID)
	if fetched.Status != SessionStateReady {
		t.Fatalf("expected status ready, got %s", fetched.Status)
	}
	if fetched.MatchedRows != 1 || fetched.UnlinkedRows != 1 {
		t.Fatalf("expected 1 matched, 1 unlinked, got %d, %d", fetched.MatchedRows, fetched.UnlinkedRows)
	}
	if len(fetched.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(fetched.Items))
	}

	// 5. Cancel Session
	cancelled := store.CancelSession(sess.ID, orgID)
	if !cancelled {
		t.Fatalf("expected cancel to succeed")
	}

	_, ok = store.GetSession(sess.ID, orgID)
	if ok {
		t.Fatalf("expected session to be deleted after cancel")
	}
}

func TestSavingImportSessionExpiryCleanup(t *testing.T) {
	store := &SavingImportSessionStore{
		sessions: make(map[string]*SavingImportSession),
	}

	sess := store.NewSession(1, 1, "old.xlsx", 10)
	// Force session to be expired
	sess.ExpiresAt = time.Now().Add(-1 * time.Hour)

	store.cleanupExpired()

	_, ok := store.GetSession(sess.ID, 1)
	if ok {
		t.Fatalf("expected expired session to be cleaned up")
	}
}
