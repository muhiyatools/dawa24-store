// Package ingest manages vendor catalog bulk file uploads, column detection,
// staging rows, and fuzzy Arabic product matching.
package ingest

import (
	"strings"
	"time"
)

// SessionStatus tracks lifecycle states of an import task.
type SessionStatus string

const (
	StatusPending    SessionStatus = "pending"
	StatusProcessing SessionStatus = "processing"
	StatusCompleted  SessionStatus = "completed"
	StatusFailed     SessionStatus = "failed"
)

// FileUpload records an uploaded catalog file in S3/MinIO (D5 fix: key pointer, no BLOBs).
type FileUpload struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	Filename       string    `json:"filename"`
	StorageKey     string    `json:"storage_key"`
	FileSizeBytes  int64     `json:"file_size_bytes"`
	MimeType       string    `json:"mime_type"`
	CreatedAt      time.Time `json:"created_at"`
}

// ImportSession represents an ongoing or completed bulk file import.
type ImportSession struct {
	ID                 int64             `json:"id"`
	PublicID           string            `json:"public_id"`
	OrganizationID     int64             `json:"organization_id"`
	FileUploadID       int64             `json:"file_upload_id"`
	Status             SessionStatus     `json:"status"`
	ColumnMapping      map[string]string `json:"column_mapping"`
	MinSimilarityScore float64           `json:"min_similarity_score"`
	TotalRows          int               `json:"total_rows"`
	ProcessedRows      int               `json:"processed_rows"`
	MatchedRows        int               `json:"matched_rows"`
	ErrorMessage       string            `json:"error_message,omitempty"`
	StartedAt          *time.Time        `json:"started_at,omitempty"`
	CompletedAt        *time.Time        `json:"completed_at,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

// ImportRow represents a single staged row extracted from an import file.
type ImportRow struct {
	ID               int64          `json:"id"`
	SessionID        int64          `json:"session_id"`
	OrganizationID   int64          `json:"organization_id"`
	RowNumber        int            `json:"row_number"`
	RawData          map[string]any `json:"raw_data"`
	NormalizedName   string         `json:"normalized_name,omitempty"`
	MatchedProductID *int64         `json:"matched_product_id,omitempty"`
	SimilarityScore  *float64       `json:"similarity_score,omitempty"`
	Status           string         `json:"status"`
	CreatedAt        time.Time      `json:"created_at"`
}

// Standard target fields expected by the catalog ingest pipeline.
const (
	FieldProductName = "product_name"
	FieldPrice       = "price"
	FieldQuantity    = "quantity"
	FieldDiscount    = "discount"
	FieldBarcode     = "barcode"
	FieldSKU         = "sku"
)

var knownColumnSynonyms = map[string][]string{
	FieldProductName: {
		"اسم الصنف", "اسم المنتج", "الصنف", "الاسم", "اسم الدواء", "المنتج",
		"name", "product_name", "item_name", "item_description", "description",
	},
	FieldPrice: {
		"السعر", "سعر الجمهور", "سعر البيع", "سعر الوحدة", "السعر للجمهور",
		"price", "unit_price", "public_price", "selling_price",
	},
	FieldQuantity: {
		"الكمية", "الرصيد", "المخزون", "العدد", "كمية الصنف",
		"quantity", "qty", "stock", "balance", "count",
	},
	FieldDiscount: {
		"الخصم", "نسبة الخصم", "الخصم التجاري", "خصم",
		"discount", "discount_percentage", "disc", "discount_percent",
	},
	FieldBarcode: {
		"الباركود", "باركود", "كود دولي", "الرقم الدولي",
		"barcode", "upc", "ean", "international_code",
	},
	FieldSKU: {
		"الكود", "كود الصنف", "رقم الصنف", "كود الشركة",
		"sku", "code", "item_code", "product_code",
	},
}

// DetectColumns uses deterministic keyword matching to map raw spreadsheet headers.
func DetectColumns(headers []string) map[string]string {
	mapping := make(map[string]string)
	usedTargets := make(map[string]bool)

	for _, header := range headers {
		clean := strings.ToLower(strings.TrimSpace(header))
		clean = strings.ReplaceAll(clean, "_", " ")
		clean = strings.ReplaceAll(clean, "-", " ")

		for targetField, synonyms := range knownColumnSynonyms {
			if usedTargets[targetField] {
				continue
			}
			for _, syn := range synonyms {
				if clean == syn || strings.Contains(clean, syn) {
					mapping[header] = targetField
					usedTargets[targetField] = true
					break
				}
			}
		}
	}
	return mapping
}
