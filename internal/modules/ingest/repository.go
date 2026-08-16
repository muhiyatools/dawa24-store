package ingest

import (
	"context"
)

// Repository defines storage operations for bulk catalog ingestion.
type Repository interface {
	CreateFileUpload(ctx context.Context, f *FileUpload) error
	GetFileUploadByID(ctx context.Context, id int64) (*FileUpload, error)

	CreateImportSession(ctx context.Context, s *ImportSession) error
	GetImportSessionByID(ctx context.Context, id int64) (*ImportSession, error)
	UpdateImportSessionProgress(ctx context.Context, id int64, processed, matched int, status SessionStatus, errMsg string) error

	InsertImportRows(ctx context.Context, rows []*ImportRow) error
	ListImportRows(ctx context.Context, sessionID int64, limit, offset int) ([]*ImportRow, error)
	UpdateImportRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, score float64, status string) error
}
