package workflow

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AutomationRequestStatus represents the lifecycle of an automation request.
type AutomationRequestStatus string

const (
	AutomationStatusPending    AutomationRequestStatus = "pending"
	AutomationStatusProcessing AutomationRequestStatus = "processing"
	AutomationStatusCompleted  AutomationRequestStatus = "completed"
	AutomationStatusApproved   AutomationRequestStatus = "approved"
	AutomationStatusFailed     AutomationRequestStatus = "failed"
)

// AutomationRequest models a customer's automated bulk purchase optimization job (Plan V5 Task 3.3).
type AutomationRequest struct {
	ID                int64                   `json:"id"`
	PublicID          string                  `json:"public_id"`
	UserID            int64                   `json:"user_id"`
	OrganizationID    *int64                  `json:"organization_id,omitempty"`
	RequestNumber     string                  `json:"request_number"`
	OriginalFilename  string                  `json:"original_filename,omitempty"`
	FilePath          string                  `json:"file_path,omitempty"`
	Status            AutomationRequestStatus `json:"status"`
	TotalProducts     int                     `json:"total_products"`
	MatchedProducts   int                     `json:"matched_products"`
	ApprovedProducts  int                     `json:"approved_products"`
	TotalValue        money.Amount            `json:"total_value"`
	Priorities        Priorities              `json:"priorities"`
	BudgetConstraint  *money.Amount           `json:"budget_constraint,omitempty"`
	FileData          []ParsedProductLine     `json:"file_data,omitempty"`
	VendorMatches     []MatchedProductEntry   `json:"vendor_matches,omitempty"`
	ComparisonResults map[string]any          `json:"comparison_results,omitempty"`
	Notes             string                  `json:"notes,omitempty"`
	ProcessedAt       *time.Time              `json:"processed_at,omitempty"`
	ApprovedAt        *time.Time              `json:"approved_at,omitempty"`
	ApprovedBy        *int64                  `json:"approved_by,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}

// ParsedProductLine represents a single product row extracted from the uploaded document.
type ParsedProductLine struct {
	RowIndex       int           `json:"row_index"`
	ProductName    string        `json:"product_name"`
	ProductSKU     string        `json:"product_sku,omitempty"`
	ProductBarcode string        `json:"product_barcode,omitempty"`
	Quantity       int           `json:"quantity"`
	TargetPrice    *money.Amount `json:"target_price,omitempty"`
	TargetDiscount float64       `json:"target_discount,omitempty"`
	Notes          string        `json:"notes,omitempty"`
}

// MatchedVendorOffer represents a vendor candidate offering a requested item.
type MatchedVendorOffer struct {
	ProductID        int64        `json:"product_id"`
	OrganizationID   int64        `json:"organization_id"`
	OrganizationName string       `json:"organization_name"`
	BranchID         *int64       `json:"branch_id,omitempty"`
	BranchName       string       `json:"branch_name,omitempty"`
	ProductName      string       `json:"product_name"`
	ProductSKU       string       `json:"product_sku,omitempty"`
	Price            money.Amount `json:"price"`
	Discount         float64      `json:"discount"`
	FinalPrice       money.Amount `json:"final_price"`
	StockQuantity    int          `json:"stock_quantity"`
	DistanceKm       *float64     `json:"distance_km,omitempty"`
	SimilarityScore  float64      `json:"similarity_score"`
	PriorityScore    float64      `json:"priority_score"`
}

// MatchedProductEntry groups all vendor matches for one requested product row.
type MatchedProductEntry struct {
	RequestedLine   ParsedProductLine    `json:"requested_line"`
	ExactMatches    []MatchedVendorOffer `json:"exact_matches"`
	SimilarMatches  []MatchedVendorOffer `json:"similar_matches"`
	BestOffer       *MatchedVendorOffer  `json:"best_offer,omitempty"`
	MatchConfidence float64              `json:"match_confidence"`
}

// OptimizationOption represents a proposed vendor allocation strategy (Option A/B/C).
type OptimizationOption struct {
	Key               string                `json:"key"` // "lowest_cost", "fastest_delivery", "minimal_vendors"
	Title             string                `json:"title"`
	Description       string                `json:"description"`
	TotalItems        int                   `json:"total_items"`
	TotalCost         money.Amount          `json:"total_cost"`
	TotalSavings      money.Amount          `json:"total_savings"`
	VendorCount       int                   `json:"vendor_count"`
	EstimatedDays     int                   `json:"estimated_days"`
	VendorAllocations []VendorShipmentDraft `json:"vendor_allocations"`
}

// VendorShipmentDraft groups lines assigned to a single supplier.
type VendorShipmentDraft struct {
	OrganizationID   int64               `json:"organization_id"`
	OrganizationName string              `json:"organization_name"`
	BranchID         *int64              `json:"branch_id,omitempty"`
	Lines            []ShipmentLineDraft `json:"lines"`
	Subtotal         money.Amount        `json:"subtotal"`
}

// ShipmentLineDraft represents a purchased line in a draft shipment.
type ShipmentLineDraft struct {
	ProductID   int64        `json:"product_id"`
	ProductName string       `json:"product_name"`
	Quantity    int          `json:"quantity"`
	UnitPrice   money.Amount `json:"unit_price"`
	Discount    float64      `json:"discount"`
	TotalPrice  money.Amount `json:"total_price"`
}

// AutomationAlert captures price or stock warnings for a line item.
type AutomationAlert struct {
	RowIndex    int    `json:"row_index"`
	ProductName string `json:"product_name"`
	AlertType   string `json:"alert_type"` // "price_exceeded", "discount_low", "stock_shortfall"
	Message     string `json:"message"`
}

// GenerateAutomationRequestNumber formats AR-YYYYMMDD-XXX matching Laravel's pattern (Rule R7).
func GenerateAutomationRequestNumber(t time.Time) string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000))
	return fmt.Sprintf("AR-%s-%03d", t.Format("20060102"), n.Int64())
}
