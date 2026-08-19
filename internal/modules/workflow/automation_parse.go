package workflow

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ParseTabularData parses CSV or delimited tabular data into structured ParsedProductLines.
func ParseTabularData(r io.Reader) ([]ParsedProductLine, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read tabular data: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("file must contain header row and at least one data row")
	}

	headers := records[0]
	mapping := compare.DetectColumns(headers)

	var lines []ParsedProductLine
	for idx, row := range records[1:] {
		name := extractField(row, mapping, compare.FieldProductName)
		if name == "" {
			name = extractField(row, mapping, compare.FieldSKU)
		}
		if name == "" {
			continue // skip empty rows
		}

		qty := 1
		qtyStr := extractField(row, mapping, compare.FieldQuantity)
		if q, err := strconv.Atoi(strings.TrimSpace(qtyStr)); err == nil && q > 0 {
			qty = q
		}

		var targetPrice *money.Amount
		priceStr := extractField(row, mapping, compare.FieldAlertPrice)
		if priceStr == "" {
			priceStr = extractField(row, mapping, compare.FieldPrice)
		}
		if priceStr != "" {
			if p, err := money.Parse(priceStr); err == nil && p.IsPositive() {
				targetPrice = &p
			}
		}

		var targetDisc float64
		discStr := extractField(row, mapping, compare.FieldAlertDiscount)
		if discStr == "" {
			discStr = extractField(row, mapping, compare.FieldDiscount)
		}
		if discStr != "" {
			cleanDisc := strings.TrimSuffix(strings.TrimSpace(discStr), "%")
			if d, err := strconv.ParseFloat(cleanDisc, 64); err == nil && d > 0 {
				targetDisc = d
			}
		}

		sku := extractField(row, mapping, compare.FieldSKU)
		barcode := extractField(row, mapping, compare.FieldBarcode)
		notes := extractField(row, mapping, compare.FieldDescription)

		lines = append(lines, ParsedProductLine{
			RowIndex:       idx + 1,
			ProductName:    name,
			ProductSKU:     sku,
			ProductBarcode: barcode,
			Quantity:       qty,
			TargetPrice:    targetPrice,
			TargetDiscount: targetDisc,
			Notes:          notes,
		})
	}

	return lines, nil
}

// ParseCSVBytes parses CSV content directly from raw bytes.
func ParseCSVBytes(data []byte) ([]ParsedProductLine, error) {
	return ParseTabularData(bytes.NewReader(data))
}

func extractField(row []string, mapping map[int]compare.TargetField, field compare.TargetField) string {
	if mapping == nil {
		return ""
	}
	for colIdx, f := range mapping {
		if f == field && colIdx >= 0 && colIdx < len(row) {
			return strings.TrimSpace(row[colIdx])
		}
	}
	return ""
}
