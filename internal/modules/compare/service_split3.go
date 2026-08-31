package compare

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// extractRowFromRecord extracts a CompareFileRow from a parsed record using the mapping config.
func (s *Service) extractRowFromRecord(record []string, headers []string, file *CompareFile, rowNumber int) *CompareFileRow {
	cfg := file.MappingConfig

	// Extract required fields using mapping config
	getValue := func(col *int) string {
		if col == nil {
			return ""
		}
		if *col >= 0 && *col < len(record) {
			return strings.TrimSpace(record[*col])
		}
		return ""
	}

	name := getValue(cfg.NameCol)
	if name == "" {
		return nil // Skip rows without product name
	}

	// Parse price
	priceStr := getValue(cfg.PriceCol)
	price := money.Zero
	if priceStr != "" {
		if val, err := strconv.ParseFloat(priceStr, 64); err == nil {
			price = money.FromMinor(int64(math.Round(val * 100)))
		} else {
			// Try to extract number from string
			if val, err := extractNumber(priceStr); err == nil {
				price = money.FromMinor(int64(math.Round(val * 100)))
			}
		}
	}
	// price and price_after_discount are numeric(12,2): ten integer digits.
	// A barcode read as a price would overflow and fail the batch.
	if price.Minor() < 0 || price.Minor() >= maxPriceMinor {
		price = money.Zero
	}

	// Parse discount
	discountStr := getValue(cfg.DiscountCol)
	discount := 0.0
	if discountStr != "" {
		if val, err := strconv.ParseFloat(discountStr, 64); err == nil {
			discount = val
		} else {
			if val, err := extractNumber(discountStr); err == nil {
				discount = val
			}
		}
	}
	// compare.file_rows.discount is numeric(5,2) and this value is a
	// percentage, so anything outside 0-100 is not a discount at all - it is
	// the auto-mapper having pointed at a price, a barcode or a pack size.
	// Storing it raw overflows the column, and because rows are inserted as one
	// batch a single such cell used to fail the whole file.
	if discount < 0 || discount > 100 {
		discount = 0
	}

	// Parse code/SKU
	code := getValue(cfg.CodeCol)

	// Calculate price after discount
	priceAfterDiscount := CalculatePriceAfterDiscount(price, discount)

	// Normalize name for matching
	normalizedName := arabic.Normalize(name)
	normalizedName = strings.ToLower(strings.TrimSpace(normalizedName))

	return &CompareFileRow{
		FileID:             file.ID,
		OrganizationID:     file.OrganizationID,
		RowNumber:          rowNumber,
		RawName:            name,
		NormalizedName:     normalizedName,
		SKU:                code,
		Price:              price,
		Discount:           discount,
		PriceAfterDiscount: priceAfterDiscount,
		MatchMethod:        MatchMethodNone,
	}
}

// maxPriceMinor is the exclusive ceiling for a price in minor units, set by
// compare.file_rows.price being numeric(12,2).
const maxPriceMinor int64 = 10_000_000_000

// extractNumber extracts the first number from a string, handling Arabic/Eastern numerals and commas.
func extractNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}

	// Convert Eastern Arabic numerals to standard digits
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_39_39"), "0")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_40_40"), "1")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_41_41"), "2")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_42_42"), "3")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_43_43"), "4")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_44_44"), "5")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_45_45"), "6")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_46_46"), "7")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_47_47"), "8")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_48_48"), "9")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_49_49"), ".")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_50_50"), "")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_51_51"), "")
	s = strings.ReplaceAll(s, "EGP", "")
	s = strings.ReplaceAll(s, "egp", "")
	s = strings.ReplaceAll(s, "LE", "")
	s = strings.ReplaceAll(s, "le", "")
	s = strings.TrimSpace(s)

	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", "")
	} else if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ",", ".")
	}

	var numStr strings.Builder
	hasDot := false
	foundDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			numStr.WriteRune(r)
			foundDigit = true
		} else if r == '.' && !hasDot {
			numStr.WriteRune(r)
			hasDot = true
		} else if foundDigit && r != ' ' && r != '\t' {
			break
		}
	}
	if !foundDigit {
		return 0, fmt.Errorf("no number found")
	}
	return strconv.ParseFloat(numStr.String(), 64)
}

// ListMarketDiscounts retrieves market-wide approved discounts with full search and filtering.
func (s *Service) ListMarketDiscounts(ctx context.Context, filter MarketDiscountsFilter) (*MarketDiscountsResult, error) {
	return s.repo.ListMarketDiscounts(ctx, filter)
}

// GetFileRowsPaginated retrieves paginated rows for a specific warehouse file.
func (s *Service) GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*CompareFileRow, int64, error) {
	return s.repo.GetFileRowsPaginated(ctx, fileID, page, limit)
}

// DeleteFileRow deletes a single item from a temporary warehouse and decrements row_count.
func (s *Service) DeleteFileRow(ctx context.Context, rowID int64) error {
	return s.repo.DeleteFileRow(ctx, rowID)
}

// DeleteFileRowOwnedBy deletes a single row only when its parent file belongs to
// ownerUserID. Used by the "my uploaded warehouses" screen.
func (s *Service) DeleteFileRowOwnedBy(ctx context.Context, rowID, ownerUserID int64) error {
	return s.repo.DeleteFileRowOwnedBy(ctx, rowID, ownerUserID)
}

// DeleteFileRows deletes all rows for a compare/warehouse file.
func (s *Service) DeleteFileRows(ctx context.Context, fileID int64) error {
	return s.repo.DeleteFileRows(ctx, fileID)
}

// CreateFile creates a new compare/warehouse file record.
func (s *Service) CreateFile(ctx context.Context, f *CompareFile) error {
	return s.repo.CreateFile(ctx, f)
}

// UpdateFile updates compare/warehouse file metadata.
func (s *Service) UpdateFile(ctx context.Context, f *CompareFile) error {
	return s.repo.UpdateFile(ctx, f)
}

// InsertFileRows inserts rows for a compare/warehouse file.
func (s *Service) InsertFileRows(ctx context.Context, rows []*CompareFileRow) error {
	return s.repo.InsertFileRows(ctx, rows)
}

// PurgeExpiredFiles runs the retention cleanup pass for expired compare files.
func (s *Service) PurgeExpiredFiles(ctx context.Context, defaultRetentionDays int) (int64, error) {
	return s.repo.PurgeExpiredCompareFiles(ctx, defaultRetentionDays)
}
