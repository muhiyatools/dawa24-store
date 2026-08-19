package ingest

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

const DefaultBatchSize = 500

// ProgressCallback is invoked periodically during streaming row ingestion.
type ProgressCallback func(ctx context.Context, processedRows int, totalEstimated int)

// ProcessSpreadsheetStream streams rows from an io.Reader (CSV or XLSX) in memory-bounded batches of 500.
func (s *Service) ProcessSpreadsheetStream(
	ctx context.Context,
	sessionID int64,
	r io.Reader,
	filename string,
	nameColumn string,
	onProgress ProgressCallback,
) (int, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return 0, database.ErrNoTenant
	}

	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".xlsx") || strings.HasSuffix(lower, ".xls") {
		return s.processXLSXStream(ctx, orgID, sessionID, r, nameColumn, onProgress)
	}

	return s.processCSVStream(ctx, orgID, sessionID, r, nameColumn, onProgress)
}

func (s *Service) processCSVStream(
	ctx context.Context,
	orgID int64,
	sessionID int64,
	r io.Reader,
	nameColumn string,
	onProgress ProgressCallback,
) (int, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("read csv headers: %w", err)
	}

	var batch []*ImportRow
	totalProcessed := 0
	rowNumber := 1

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip corrupted line
		}

		raw := make(map[string]any, len(headers))
		for i, h := range headers {
			if i < len(record) {
				raw[h] = record[i]
			}
		}

		var norm string
		if nameColumn != "" {
			if val, ok := raw[nameColumn]; ok && val != nil {
				norm = arabic.Normalize(fmt.Sprintf("%v", val))
			}
		}

		batch = append(batch, &ImportRow{
			SessionID:      sessionID,
			OrganizationID: orgID,
			RowNumber:      rowNumber,
			RawData:        raw,
			NormalizedName: norm,
			Status:         "pending",
		})
		rowNumber++

		if len(batch) >= DefaultBatchSize {
			if err := s.repo.InsertImportRows(ctx, batch); err != nil {
				return totalProcessed, fmt.Errorf("insert batch: %w", err)
			}
			totalProcessed += len(batch)
			batch = batch[:0]
			if onProgress != nil {
				onProgress(ctx, totalProcessed, totalProcessed)
			}
		}
	}

	if len(batch) > 0 {
		if err := s.repo.InsertImportRows(ctx, batch); err != nil {
			return totalProcessed, fmt.Errorf("insert final batch: %w", err)
		}
		totalProcessed += len(batch)
		if onProgress != nil {
			onProgress(ctx, totalProcessed, totalProcessed)
		}
	}

	s.log.InfoContext(ctx, "streamed csv processing complete", "session_id", sessionID, "total_rows", totalProcessed)
	return totalProcessed, nil
}

func (s *Service) processXLSXStream(
	ctx context.Context,
	orgID int64,
	sessionID int64,
	r io.Reader,
	nameColumn string,
	onProgress ProgressCallback,
) (int, error) {
	// Read into bytes for excelize opening
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read xlsx data: %w", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("open xlsx stream: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return 0, fmt.Errorf("xlsx workbook has no sheets")
	}

	rows, err := f.Rows(sheets[0])
	if err != nil {
		return 0, fmt.Errorf("open sheet rows iterator: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, fmt.Errorf("empty sheet")
	}
	headers, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("read xlsx headers: %w", err)
	}

	var batch []*ImportRow
	totalProcessed := 0
	rowNumber := 1

	for rows.Next() {
		columns, err := rows.Columns()
		if err != nil {
			continue
		}

		raw := make(map[string]any, len(headers))
		for i, h := range headers {
			if i < len(columns) {
				raw[h] = columns[i]
			}
		}

		var norm string
		if nameColumn != "" {
			if val, ok := raw[nameColumn]; ok && val != nil {
				norm = arabic.Normalize(fmt.Sprintf("%v", val))
			}
		}

		batch = append(batch, &ImportRow{
			SessionID:      sessionID,
			OrganizationID: orgID,
			RowNumber:      rowNumber,
			RawData:        raw,
			NormalizedName: norm,
			Status:         "pending",
		})
		rowNumber++

		if len(batch) >= DefaultBatchSize {
			if err := s.repo.InsertImportRows(ctx, batch); err != nil {
				return totalProcessed, fmt.Errorf("insert batch: %w", err)
			}
			totalProcessed += len(batch)
			batch = batch[:0]
			if onProgress != nil {
				onProgress(ctx, totalProcessed, totalProcessed)
			}
		}
	}

	if len(batch) > 0 {
		if err := s.repo.InsertImportRows(ctx, batch); err != nil {
			return totalProcessed, fmt.Errorf("insert final batch: %w", err)
		}
		totalProcessed += len(batch)
		if onProgress != nil {
			onProgress(ctx, totalProcessed, totalProcessed)
		}
	}

	s.log.InfoContext(ctx, "streamed xlsx processing complete", "session_id", sessionID, "total_rows", totalProcessed)
	return totalProcessed, nil
}
