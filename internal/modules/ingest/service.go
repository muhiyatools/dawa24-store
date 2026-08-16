package ingest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

// ProductCandidate represents a potential matching product in the master catalogue.
type ProductCandidate struct {
	ID   int64
	Name string
}

// AIMatcher defines an optional AI-augmented capability to match product queries.
type AIMatcher interface {
	MatchCandidate(ctx context.Context, query string, candidateNames []string) (bestCandidate string, score float64)
}

// Service manages vendor bulk catalog file processing and Arabic product matching.
type Service struct {
	repo      Repository
	aiMatcher AIMatcher
	log       *slog.Logger
}

// NewService creates a new ingest service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// SetAIMatcher configures an optional AI candidate matcher.
func (s *Service) SetAIMatcher(matcher AIMatcher) {
	s.aiMatcher = matcher
}

// RegisterUpload creates a file upload metadata pointer.
func (s *Service) RegisterUpload(ctx context.Context, f *FileUpload) (*FileUpload, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	f.OrganizationID = orgID

	if f.Filename == "" || f.StorageKey == "" {
		return nil, apperr.Validation("upload.invalid", "Filename and storage key are required.", nil)
	}

	if err := s.repo.CreateFileUpload(ctx, f); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "file upload registered", "upload_id", f.ID, "storage_key", f.StorageKey)
	return f, nil
}

// StartSession initiates an ingest session with detected column mappings.
func (s *Service) StartSession(
	ctx context.Context,
	uploadID int64,
	headers []string,
	minScore float64,
) (*ImportSession, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	if minScore <= 0 {
		minScore = 0.85 // Standard legacy default
	}

	mapping := DetectColumns(headers)

	session := &ImportSession{
		OrganizationID:     orgID,
		FileUploadID:       uploadID,
		Status:             StatusPending,
		ColumnMapping:      mapping,
		MinSimilarityScore: minScore,
	}

	if err := s.repo.CreateImportSession(ctx, session); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "import session created", "session_id", session.ID, "detected_columns", len(mapping))
	return session, nil
}

// StageRows inserts raw spreadsheet rows into the staging table with normalized names.
func (s *Service) StageRows(
	ctx context.Context,
	sessionID int64,
	nameColumn string,
	rawRows []map[string]any,
) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}

	var importRows []*ImportRow
	for idx, raw := range rawRows {
		var norm string
		if rawVal, exists := raw[nameColumn]; exists && rawVal != nil {
			norm = arabic.Normalize(fmt.Sprintf("%v", rawVal))
		}

		importRows = append(importRows, &ImportRow{
			SessionID:      sessionID,
			OrganizationID: orgID,
			RowNumber:      idx + 1,
			RawData:        raw,
			NormalizedName: norm,
			Status:         "pending",
		})
	}

	return s.repo.InsertImportRows(ctx, importRows)
}

// MatchRowDeterministic matches a staged row against catalog candidates using pure Arabic similarity.
func (s *Service) MatchRowDeterministic(
	ctx context.Context,
	row *ImportRow,
	candidates []ProductCandidate,
	minScore float64,
) (bool, *int64, float64, error) {
	if row.NormalizedName == "" {
		return false, nil, 0, nil
	}

	var bestMatchID *int64
	var highestScore float64

	for _, cand := range candidates {
		candNorm := arabic.Normalize(cand.Name)
		score := arabic.Similarity(row.NormalizedName, candNorm)
		if score > highestScore {
			highestScore = score
			candID := cand.ID
			bestMatchID = &candID
		}
	}

	matched := highestScore >= minScore
	if !matched && s.aiMatcher != nil && len(candidates) > 0 {
		candNames := make([]string, len(candidates))
		for i, c := range candidates {
			candNames[i] = c.Name
		}
		bestCand, aiScore := s.aiMatcher.MatchCandidate(ctx, row.NormalizedName, candNames)
		if aiScore >= minScore && bestCand != "" {
			for _, cand := range candidates {
				if cand.Name == bestCand {
					candID := cand.ID
					bestMatchID = &candID
					highestScore = aiScore
					matched = true
					break
				}
			}
		}
	}

	status := "unmatched"
	if matched {
		status = "matched"
	}

	err := s.repo.UpdateImportRowMatch(ctx, row.ID, bestMatchID, highestScore, status)
	return matched, bestMatchID, highestScore, err
}

// GetSessionProgress retrieves the progress and statistics of an import session.
func (s *Service) GetSessionProgress(ctx context.Context, sessionID int64) (*ImportSession, error) {
	return s.repo.GetImportSessionByID(ctx, sessionID)
}

// ListSessions returns import sessions for an organization.
func (s *Service) ListSessions(ctx context.Context, orgID int64, limit, offset int) ([]*ImportSession, error) {
	return s.repo.ListImportSessions(ctx, orgID, limit, offset)
}

// UpdateColumnMapping updates the detected column map.
func (s *Service) UpdateColumnMapping(ctx context.Context, sessionID int64, mapping map[string]string) error {
	return s.repo.UpdateColumnMapping(ctx, sessionID, mapping)
}

// CommitSession finalizes the import session and marks it completed.
func (s *Service) CommitSession(ctx context.Context, sessionID int64) error {
	return s.repo.UpdateSessionStatus(ctx, sessionID, StatusCompleted)
}

// CancelSession marks the session cancelled.
func (s *Service) CancelSession(ctx context.Context, sessionID int64) error {
	return s.repo.UpdateSessionStatus(ctx, sessionID, StatusFailed)
}

// ListImportRows returns staged rows for review.
func (s *Service) ListImportRows(ctx context.Context, sessionID int64, status string, limit, offset int) ([]*ImportRow, error) {
	return s.repo.ListImportRows(ctx, sessionID, status, limit, offset)
}

// OverrideRowMatch sets a manual product match for a staged row.
func (s *Service) OverrideRowMatch(ctx context.Context, rowID int64, matchedProductID int64) error {
	if matchedProductID <= 0 {
		return apperr.Validation("product_id.invalid", "Invalid product ID", nil)
	}
	return s.repo.UpdateImportRowMatch(ctx, rowID, &matchedProductID, 1.0, "matched")
}
