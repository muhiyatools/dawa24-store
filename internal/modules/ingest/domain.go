// Package ingest manages vendor catalog bulk file uploads, column detection,
// staging rows, and intelligent multi-stage product matching.
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

// ImportMode specifies the reconciliation strategy during commit.
type ImportMode string

const (
	ModeAddNewOnly         ImportMode = "add_new_only"
	ModeUpdateExistingOnly ImportMode = "update_existing_only"
	ModeClearAndAdd        ImportMode = "clear_and_add"
	ModeUpdateAndAdd       ImportMode = "update_and_add"
)

// ConfidenceLevel classifies matching reliability.
type ConfidenceLevel string

const (
	ConfidenceHigh      ConfidenceLevel = "high"      // >= 85% or deterministic ID/Exact
	ConfidenceReview    ConfidenceLevel = "review"    // 60% - 84%
	ConfidenceLow       ConfidenceLevel = "low"       // < 60%
	ConfidenceUnmatched ConfidenceLevel = "unmatched" // No match
)

// CandidateMatch stores potential alternative master products with scores.
type CandidateMatch struct {
	ProductID      int64   `json:"product_id"`
	ProductName    string  `json:"product_name"`
	ScientificName string  `json:"scientific_name,omitempty"`
	DosageForm     string  `json:"dosage_form,omitempty"`
	Concentration  string  `json:"concentration,omitempty"`
	Manufacturer   string  `json:"manufacturer,omitempty"`
	PublicPrice    string  `json:"public_price,omitempty"`
	Similarity     float64 `json:"similarity"`
	Reason         string  `json:"reason"`
}

// FileUpload records an uploaded catalog file in S3/MinIO.
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
	ID                    int64             `json:"id"`
	PublicID              string            `json:"public_id"`
	OrganizationID        int64             `json:"organization_id"`
	WarehouseID           *int64            `json:"warehouse_id,omitempty"`
	ImportMode            ImportMode        `json:"import_mode"`
	EnableAIMatching      bool              `json:"enable_ai_matching"`
	EnableSavingsMatching bool              `json:"enable_savings_matching"`
	FileUploadID          int64             `json:"file_upload_id"`
	Status                SessionStatus     `json:"status"`
	ColumnMapping         map[string]string `json:"column_mapping"`
	MinSimilarityScore    float64           `json:"min_similarity_score"`
	TotalRows             int               `json:"total_rows"`
	ProcessedRows         int               `json:"processed_rows"`
	MatchedRows           int               `json:"matched_rows"`
	ReviewRows            int               `json:"review_rows"`
	UnmatchedRows         int               `json:"unmatched_rows"`
	ErrorMessage          string            `json:"error_message,omitempty"`
	StartedAt             *time.Time        `json:"started_at,omitempty"`
	CompletedAt           *time.Time        `json:"completed_at,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// ImportRow represents a single staged row extracted from an import file.
type ImportRow struct {
	ID               int64            `json:"id"`
	SessionID        int64            `json:"session_id"`
	OrganizationID   int64            `json:"organization_id"`
	RowNumber        int              `json:"row_number"`
	RawData          map[string]any   `json:"raw_data"`
	NormalizedName   string           `json:"normalized_name,omitempty"`
	MatchedProductID *int64           `json:"matched_product_id,omitempty"`
	MatchedProdName  string           `json:"matched_product_name,omitempty"`
	MatchedProdSKU   string           `json:"matched_product_sku,omitempty"`
	SimilarityScore  *float64         `json:"similarity_score,omitempty"`
	ConfidenceLevel  ConfidenceLevel  `json:"confidence_level"`
	MatchReason      string           `json:"match_reason,omitempty"`
	CandidateMatches []CandidateMatch `json:"candidate_matches,omitempty"`
	IsApproved       bool             `json:"is_approved"`
	ImportAction     string           `json:"import_action"`
	ErrorDetails     string           `json:"error_details,omitempty"`
	Status           string           `json:"status"`
	CreatedAt        time.Time        `json:"created_at"`
}

// Standard target fields expected by the catalog ingest pipeline.
const (
	FieldProductName   = "product_name"
	FieldPrice         = "price"
	FieldCostPrice     = "cost_price"
	FieldQuantity      = "quantity"
	FieldDiscount      = "discount"
	FieldBarcode       = "barcode"
	FieldSKU           = "sku"
	FieldBatchNumber   = "batch_number"
	FieldExpiryDate    = "expiry_date"
	FieldUnit          = "unit"
	FieldMinThreshold  = "min_threshold"
	FieldMinOrderQty   = "min_order_qty"
	FieldManufacturer  = "manufacturer"
	FieldDosageForm    = "dosage_form"
	FieldConcentration = "concentration"
)

var knownColumnSynonyms = map[string][]string{
	FieldProductName: {
		"اسم الصنف", "اسم المنتج", "الصنف", "الاسم", "اسم الدواء", "المنتج", "اسم_الصنف", "اسم_المنتج",
		"name", "product_name", "item_name", "item_description", "description", "product", "title",
	},
	FieldPrice: {
		"السعر", "سعر الجمهور", "سعر البيع", "سعر الوحدة", "السعر للجمهور", "سعر المستهلك", "سعر_الجمهور",
		"price", "unit_price", "public_price", "selling_price", "retail_price",
	},
	FieldCostPrice: {
		"سعر التكلفة", "سعر الشراء", "التكلفة", "سعر_التكلفة", "سعر_الشراء",
		"cost", "cost_price", "purchase_price", "buy_price", "buying_price",
	},
	FieldQuantity: {
		"الكمية", "الرصيد", "المخزون", "العدد", "كمية الصنف", "الرصيد المتاح", "كمية_المخزون", "الكميه",
		"quantity", "qty", "stock", "balance", "count", "inventory", "available_qty",
	},
	FieldDiscount: {
		"الخصم", "نسبة الخصم", "الخصم التجاري", "خصم", "قيمة الخصم", "نسبة_الخصم",
		"discount", "discount_percentage", "disc", "discount_percent", "discount_rate",
	},
	FieldBarcode: {
		"الباركود", "باركود", "كود دولي", "الرقم الدولي", "الباركود الدولي",
		"barcode", "upc", "ean", "international_code", "gtin",
	},
	FieldSKU: {
		"الكود", "كود الصنف", "رقم الصنف", "كود الشركة", "كود المنتج", "كود_الصنف",
		"sku", "code", "item_code", "product_code", "item_no", "item_id", "product_id",
	},
	FieldBatchNumber: {
		"رقم التشغيلة", "التشغيلة", "رقم الباتش", "الباتش", "الطبخة", "رقم_التشغيلة", "تشغيلة",
		"batch", "batch_number", "batch_no", "lot", "lot_number", "lot_no",
	},
	FieldExpiryDate: {
		"تاريخ الصلاحية", "الصلاحية", "تاريخ الانتهاء", "انتهاء الصلاحية", "تاريخ_الصلاحية", "تاريخ_الانتهاء",
		"expiry", "expiry_date", "exp_date", "expiration_date", "valid_until",
	},
	FieldUnit: {
		"الوحدة", "وحدة القياس", "العبوة", "نوع العبوة", "الوحده",
		"unit", "pack", "packaging", "uom", "unit_of_measure",
	},
	FieldMinThreshold: {
		"حد الأمان", "الحد الأدنى للمخزون", "نقطة إعادة الطلب", "حد_الامان",
		"min_threshold", "safety_stock", "reorder_point", "min_stock",
	},
	FieldMinOrderQty: {
		"الحد الأدنى للطلب", "أقل كمية للطلب", "الحد_الادنى_للطلب",
		"min_order_qty", "min_order", "moq", "minimum_order",
	},
	FieldManufacturer: {
		"الشركة المصنعة", "المصنع", "الشركة", "البراند", "الشركة_المصنعة", "المورد",
		"manufacturer", "company", "brand", "producer", "vendor",
	},
	FieldDosageForm: {
		"الشكل الدوائي", "شكل الدواء", "النوع", "الشكل_الدوائي",
		"dosage_form", "form", "type",
	},
	FieldConcentration: {
		"التركيز", "القوة", "عيار", "تركيز الدواء",
		"concentration", "strength", "dose",
	},
}

// DetectColumns uses deterministic keyword matching to map standard target fields to raw spreadsheet headers.
// Returns map[targetField]rawHeaderName (e.g. mapping["product_name"] = "اسم الصنف").
func DetectColumns(headers []string) map[string]string {
	mapping := make(map[string]string)
	usedHeaders := make(map[string]bool)

	for targetField, synonyms := range knownColumnSynonyms {
		for _, header := range headers {
			if usedHeaders[header] {
				continue
			}
			clean := strings.ToLower(strings.TrimSpace(header))
			clean = strings.ReplaceAll(clean, "_", " ")
			clean = strings.ReplaceAll(clean, "-", " ")

			matched := false
			for _, syn := range synonyms {
				synClean := strings.ToLower(strings.TrimSpace(syn))
				if clean == synClean || strings.Contains(clean, synClean) || strings.Contains(synClean, clean) {
					mapping[targetField] = header
					usedHeaders[header] = true
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
	}
	return mapping
}
