package ingest_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockIngestRepo struct {
	uploads  map[int64]*ingest.FileUpload
	sessions map[int64]*ingest.ImportSession
	rows     map[int64]*ingest.ImportRow
	nextID   int64
}

func newMockIngestRepo() *mockIngestRepo {
	return &mockIngestRepo{
		uploads:  map[int64]*ingest.FileUpload{},
		sessions: map[int64]*ingest.ImportSession{},
		rows:     map[int64]*ingest.ImportRow{},
		nextID:   1,
	}
}

func (m *mockIngestRepo) CreateFileUpload(_ context.Context, f *ingest.FileUpload) error {
	f.ID = m.nextID
	m.nextID++
	m.uploads[f.ID] = f
	return nil
}

func (m *mockIngestRepo) GetFileUploadByID(_ context.Context, id int64) (*ingest.FileUpload, error) {
	f, ok := m.uploads[id]
	if !ok {
		return nil, apperr.NotFound("file_upload")
	}
	return f, nil
}

func (m *mockIngestRepo) CreateImportSession(_ context.Context, s *ingest.ImportSession) error {
	s.ID = m.nextID
	m.nextID++
	m.sessions[s.ID] = s
	return nil
}

func (m *mockIngestRepo) GetImportSessionByID(_ context.Context, id int64) (*ingest.ImportSession, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, apperr.NotFound("import_session")
	}
	return s, nil
}

func (m *mockIngestRepo) UpdateImportSessionProgress(_ context.Context, id int64, processed, matched int, status ingest.SessionStatus, errMsg string) error {
	s, ok := m.sessions[id]
	if !ok {
		return apperr.NotFound("import_session")
	}
	s.ProcessedRows = processed
	s.MatchedRows = matched
	s.Status = status
	s.ErrorMessage = errMsg
	return nil
}

func (m *mockIngestRepo) InsertImportRows(_ context.Context, rows []*ingest.ImportRow) error {
	for _, r := range rows {
		r.ID = m.nextID
		m.nextID++
		m.rows[r.ID] = r
	}
	return nil
}

func (m *mockIngestRepo) ListImportSessions(_ context.Context, orgID int64, limit, offset int) ([]*ingest.ImportSession, error) {
	return []*ingest.ImportSession{}, nil
}

func (m *mockIngestRepo) UpdateColumnMapping(_ context.Context, id int64, mapping map[string]string) error {
	if s, ok := m.sessions[id]; ok {
		s.ColumnMapping = mapping
	}
	return nil
}

func (m *mockIngestRepo) UpdateSessionStatus(_ context.Context, id int64, status ingest.SessionStatus) error {
	if s, ok := m.sessions[id]; ok {
		s.Status = status
	}
	return nil
}

func (m *mockIngestRepo) ListImportRows(_ context.Context, sessionID int64, status string, limit, offset int) ([]*ingest.ImportRow, error) {
	var list []*ingest.ImportRow
	for _, r := range m.rows {
		if r.SessionID == sessionID && (status == "" || r.Status == status) {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockIngestRepo) GetImportRowByID(_ context.Context, id int64) (*ingest.ImportRow, error) {
	if r, ok := m.rows[id]; ok {
		return r, nil
	}
	return nil, apperr.NotFound("import_row")
}

func (m *mockIngestRepo) UpdateImportRowMatch(_ context.Context, rowID int64, matchedProductID *int64, score float64, status string) error {
	r, ok := m.rows[rowID]
	if !ok {
		return apperr.NotFound("import_row")
	}
	r.MatchedProductID = matchedProductID
	r.SimilarityScore = &score
	r.Status = status
	return nil
}

func TestColumnDetection(t *testing.T) {
	rawHeaders := []string{
		"اسم الدواء",
		"سعر الجمهور",
		"الرصيد",
		"نسبة الخصم",
		"الباركود",
		"كود الصنف",
	}

	mapping := ingest.DetectColumns(rawHeaders)

	expected := map[string]string{
		"اسم الدواء":  ingest.FieldProductName,
		"سعر الجمهور": ingest.FieldPrice,
		"الرصيد":      ingest.FieldQuantity,
		"نسبة الخصم":  ingest.FieldDiscount,
		"الباركود":    ingest.FieldBarcode,
		"كود الصنف":   ingest.FieldSKU,
	}

	for header, expectedField := range expected {
		if mapping[header] != expectedField {
			t.Errorf("header %q mapped to %q; want %q", header, mapping[header], expectedField)
		}
	}
}

func TestDeterministicProductMatcher(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 10)
	repo := newMockIngestRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := ingest.NewService(repo, logger)

	candidates := []ingest.ProductCandidate{
		{ID: 1001, Name: "بنادول اكسترا اقراص"},
		{ID: 1002, Name: "كتافلام 50 مجم"},
		{ID: 1003, Name: "كونجستال اقراص"},
	}

	// Case 1: Close Arabic match
	row1 := &ingest.ImportRow{
		ID:             1,
		NormalizedName: "بانادول اكسترا اقراص",
	}
	repo.rows[row1.ID] = row1
	matched, matchID, score, err := svc.MatchRowDeterministic(ctx, row1, candidates, 0.85)
	if err != nil {
		t.Fatalf("MatchRowDeterministic failed: %v", err)
	}
	if !matched || matchID == nil || *matchID != 1001 {
		t.Errorf("expected row1 to match product 1001 with high score, got matched=%v, matchID=%v, score=%f", matched, matchID, score)
	}

	// Case 2: Unrelated product
	row2 := &ingest.ImportRow{
		ID:             2,
		NormalizedName: "فيتامين سي فوار",
	}
	repo.rows[row2.ID] = row2
	matched, _, score, err = svc.MatchRowDeterministic(ctx, row2, candidates, 0.85)
	if err != nil {
		t.Fatalf("MatchRowDeterministic failed: %v", err)
	}
	if matched {
		t.Errorf("expected row2 to be unmatched, got matched=true, score=%f", score)
	}
}
