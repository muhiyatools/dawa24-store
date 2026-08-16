package integration_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type blackholeMockIngestRepo struct {
	rows map[int64]*ingest.ImportRow
}

func (m *blackholeMockIngestRepo) CreateFileUpload(_ context.Context, f *ingest.FileUpload) error {
	return nil
}
func (m *blackholeMockIngestRepo) GetFileUploadByID(_ context.Context, id int64) (*ingest.FileUpload, error) {
	return nil, apperr.NotFound("file_upload")
}
func (m *blackholeMockIngestRepo) CreateImportSession(_ context.Context, s *ingest.ImportSession) error {
	return nil
}
func (m *blackholeMockIngestRepo) GetImportSessionByID(_ context.Context, id int64) (*ingest.ImportSession, error) {
	return nil, apperr.NotFound("import_session")
}
func (m *blackholeMockIngestRepo) ListImportSessions(_ context.Context, orgID int64, limit, offset int) ([]*ingest.ImportSession, error) {
	return nil, nil
}
func (m *blackholeMockIngestRepo) UpdateImportSessionProgress(_ context.Context, id int64, processed, matched int, status ingest.SessionStatus, errMsg string) error {
	return nil
}
func (m *blackholeMockIngestRepo) UpdateColumnMapping(_ context.Context, id int64, mapping map[string]string) error {
	return nil
}
func (m *blackholeMockIngestRepo) UpdateSessionStatus(_ context.Context, id int64, status ingest.SessionStatus) error {
	return nil
}
func (m *blackholeMockIngestRepo) InsertImportRows(_ context.Context, rows []*ingest.ImportRow) error {
	return nil
}
func (m *blackholeMockIngestRepo) ListImportRows(_ context.Context, sessionID int64, status string, limit, offset int) ([]*ingest.ImportRow, error) {
	return nil, nil
}
func (m *blackholeMockIngestRepo) GetImportRowByID(_ context.Context, id int64) (*ingest.ImportRow, error) {
	return nil, apperr.NotFound("import_row")
}
func (m *blackholeMockIngestRepo) UpdateImportRowMatch(_ context.Context, rowID int64, matchedProductID *int64, score float64, status string) error {
	if r, ok := m.rows[rowID]; ok {
		r.MatchedProductID = matchedProductID
		r.SimilarityScore = &score
		r.Status = status
	}
	return nil
}

// TestGatewayBlackHoleVerifiesDeterministicFallback ensures that when the AI Gateway
// is pointed at an unreachable black-hole IP (RFC 5737 192.0.2.1), the system does not hang
// and completes ingestion matching with deterministic Arabic similarity fallback.
func TestGatewayBlackHoleVerifiesDeterministicFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Point gateway client to RFC 5737 unroutable TEST-NET-1 IP
	gwCfg := config.Gateway{
		BaseURL:    "http://192.0.2.1:8181",
		VirtualKey: "vk-test-blackhole-key",
		ClientApp:  "dawa24-store-test",
		Timeout:    100 * time.Millisecond,
		Enabled:    true,
	}
	gwClient := gateway.New(gwCfg, logger)

	aiSvc := aicapabilities.NewService(gwClient, logger)

	row := &ingest.ImportRow{
		ID:             101,
		SessionID:      1,
		OrganizationID: 42,
		RowNumber:      1,
		NormalizedName: "بنادول اكسترا 500 مجم",
		Status:         "pending",
	}

	repo := &blackholeMockIngestRepo{
		rows: map[int64]*ingest.ImportRow{101: row},
	}
	ingSvc := ingest.NewService(repo, logger)
	ingSvc.SetAIMatcher(aiSvc)

	candidates := []ingest.ProductCandidate{
		{ID: 1001, Name: "بنادول اكسترا 500 مجم اقراص"},
		{ID: 1002, Name: "كونجستال اقراص"},
		{ID: 1003, Name: "اوغمنتين 1 جم"},
	}

	matched, matchID, score, err := ingSvc.MatchRowDeterministic(ctx, row, candidates, 0.75)
	if err != nil {
		t.Fatalf("unexpected error during black-hole fallback match: %v", err)
	}

	if !matched {
		t.Fatal("expected product to match via deterministic fallback")
	}

	if matchID == nil || *matchID != 1001 {
		t.Fatalf("expected matched candidate ID 1001, got %v", matchID)
	}

	if score < 0.75 {
		t.Fatalf("expected similarity score >= 0.75, got %f", score)
	}
}
