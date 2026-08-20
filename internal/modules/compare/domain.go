package compare

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Plan represents a compare-engine subscription tier.
type Plan struct {
	ID            int64          `json:"id"`
	PublicID      string         `json:"public_id"`
	Name          i18n.Text      `json:"name"`
	Slug          string         `json:"slug"`
	Description   i18n.Text      `json:"description"`
	PriceMonthly  money.Amount   `json:"price_monthly"`
	PriceYearly   money.Amount   `json:"price_yearly"`
	PriceLifetime money.Amount   `json:"price_lifetime"`
	Currency      string         `json:"currency"`
	TrialDays     int            `json:"trial_days"`
	IsActive      bool           `json:"is_active"`
	IsPublic      bool           `json:"is_public"`
	IsRecommended bool           `json:"is_recommended"`
	SortOrder     int            `json:"sort_order"`
	Features      []*PlanFeature `json:"features,omitempty"`
	CreatedBy     *int64         `json:"created_by,omitempty"`
	UpdatedBy     *int64         `json:"updated_by,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     *time.Time     `json:"deleted_at,omitempty"`
}

// PlanFeature represents a capability or quota assigned to a plan.
type PlanFeature struct {
	ID          int64     `json:"id"`
	PlanID      int64     `json:"plan_id"`
	Key         string    `json:"key"`
	Name        i18n.Text `json:"name"`
	Description i18n.Text `json:"description"`
	Value       string    `json:"value"`
	ValueType   string    `json:"value_type"`
	IsActive    bool      `json:"is_active"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PlanRequestStatus represents the review state of a plan request.
type PlanRequestStatus string

const (
	RequestPending   PlanRequestStatus = "pending"
	RequestApproved  PlanRequestStatus = "approved"
	RequestRejected  PlanRequestStatus = "rejected"
	RequestCancelled PlanRequestStatus = "cancelled"
)

// PlanRequest represents a tenant's request for plan enrollment or upgrade.
type PlanRequest struct {
	ID              int64             `json:"id"`
	PublicID        string            `json:"public_id"`
	PlanID          int64             `json:"plan_id"`
	OrganizationID  int64             `json:"organization_id"`
	UserID          int64             `json:"user_id"`
	Status          PlanRequestStatus `json:"status"`
	Notes           string            `json:"notes,omitempty"`
	RejectionReason string            `json:"rejection_reason,omitempty"`
	ReviewedBy      *int64            `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time        `json:"reviewed_at,omitempty"`
	Plan            *Plan             `json:"plan,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	DeletedAt       *time.Time        `json:"deleted_at,omitempty"`
}

// SubscriptionStatus represents the life cycle state of an active subscription.
type SubscriptionStatus string

const (
	SubPending   SubscriptionStatus = "pending"
	SubActive    SubscriptionStatus = "active"
	SubExpired   SubscriptionStatus = "expired"
	SubCancelled SubscriptionStatus = "cancelled"
	SubSuspended SubscriptionStatus = "suspended"
)

// Subscription represents an organization's or user's compare-engine subscription.
type Subscription struct {
	ID             int64              `json:"id"`
	PublicID       string             `json:"public_id"`
	PlanID         int64              `json:"plan_id"`
	OrganizationID *int64             `json:"organization_id,omitempty"`
	UserID         int64              `json:"user_id"`
	BillingPeriod  string             `json:"billing_period"`
	PaymentMethod  string             `json:"payment_method"`
	StartsAt       time.Time          `json:"starts_at"`
	EndsAt         *time.Time         `json:"ends_at,omitempty"`
	Status         SubscriptionStatus `json:"status"`
	Seats          int                `json:"seats"`
	Plan           *Plan              `json:"plan,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	DeletedAt      *time.Time         `json:"deleted_at,omitempty"`
}

// IsValid checks whether the subscription is active and not expired.
func (s *Subscription) IsValid() bool {
	if s == nil || s.Status != SubActive {
		return false
	}
	if s.EndsAt != nil && s.EndsAt.Before(time.Now().UTC()) {
		return false
	}
	return true
}

// SubscriptionUser assigns an individual user to a multi-seat organization subscription.
type SubscriptionUser struct {
	ID             int64     `json:"id"`
	SubscriptionID int64     `json:"subscription_id"`
	UserID         int64     `json:"user_id"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UserSession represents an active client session for concurrent device limit enforcement.
type UserSession struct {
	ID                 int64      `json:"id"`
	PublicID           string     `json:"public_id"`
	SubscriptionUserID *int64     `json:"subscription_user_id,omitempty"`
	UserID             int64      `json:"user_id"`
	SessionID          string     `json:"session_id"`
	DeviceUUID         string     `json:"device_uuid,omitempty"`
	IsActive           bool       `json:"is_active"`
	DeviceName         string     `json:"device_name,omitempty"`
	DeviceType         string     `json:"device_type,omitempty"`
	Platform           string     `json:"platform,omitempty"`
	PlatformVersion    string     `json:"platform_version,omitempty"`
	Browser            string     `json:"browser,omitempty"`
	BrowserVersion     string     `json:"browser_version,omitempty"`
	IPAddress          string     `json:"ip_address,omitempty"`
	UserAgent          string     `json:"user_agent,omitempty"`
	Country            string     `json:"country,omitempty"`
	City               string     `json:"city,omitempty"`
	LoggedInAt         time.Time  `json:"logged_in_at"`
	LastActivityAt     time.Time  `json:"last_activity_at"`
	LoggedOutAt        *time.Time `json:"logged_out_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Entitlement answers "what may this user do in the compare tool right now?" (Plan V5 Phase 2 §2.1.4).
type Entitlement struct {
	Active            bool       `json:"active"`
	PlanSlug          string     `json:"plan_slug"`
	MaxActiveFiles    int        `json:"max_active_files"`
	MaxSessions       int        `json:"max_sessions"`
	AIMatchingEnabled bool       `json:"ai_matching_enabled"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

// CompareFileStatus represents the processing state of an uploaded spreadsheet.
type CompareFileStatus string

const (
	FileUploaded CompareFileStatus = "uploaded"
	FileMapping  CompareFileStatus = "mapping"
	FileReady    CompareFileStatus = "ready"
	FileFailed   CompareFileStatus = "failed"
	FileArchived CompareFileStatus = "archived"
)

// MappingConfig defines column index mappings for spreadsheet parser.
type MappingConfig struct {
	NameCol     *int `json:"name_col,omitempty"`
	PriceCol    *int `json:"price_col,omitempty"`
	DiscountCol *int `json:"discount_col,omitempty"`
	CodeCol     *int `json:"code_col,omitempty"`
}

// CompareFile represents an uploaded supplier price & discount spreadsheet.
type CompareFile struct {
	ID               int64             `json:"id"`
	PublicID         string            `json:"public_id"`
	OrganizationID   *int64            `json:"organization_id,omitempty"`
	UserID           int64             `json:"user_id"`
	SupplierName     string            `json:"supplier_name"`
	OriginalFilename string            `json:"original_filename"`
	StorageKey       string            `json:"storage_key"`
	MIMEType         string            `json:"mime_type"`
	SizeBytes        int64             `json:"size_bytes"`
	RowCount         int               `json:"row_count"`
	Status           CompareFileStatus `json:"status"`
	MappingConfig    MappingConfig     `json:"mapping_config"`
	ArchivedAt       *time.Time        `json:"archived_at,omitempty"`
	ArchiveReason    string            `json:"archive_reason,omitempty"`
	ErrorMessage     string            `json:"error_message,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	DeletedAt        *time.Time        `json:"deleted_at,omitempty"`
}

// MatchMethod tracks how a row was linked to the canonical master catalog.
type MatchMethod string

const (
	MatchMethodSavedMapping MatchMethod = "saved_mapping"
	MatchMethodManual       MatchMethod = "manual"
	MatchMethodDirectID     MatchMethod = "direct_id"
	MatchMethodSKU          MatchMethod = "sku"
	MatchMethodBarcode      MatchMethod = "barcode"
	MatchMethodExactName    MatchMethod = "exact_name"
	MatchMethodFuzzy        MatchMethod = "fuzzy"
	MatchMethodPartial      MatchMethod = "partial"
	MatchMethodAI           MatchMethod = "ai"
	MatchMethodUnmatched    MatchMethod = "unmatched"
)

// CompareFileRow represents an individual drug/product item extracted from a spreadsheet.
type CompareFileRow struct {
	ID                 int64          `json:"id"`
	FileID             int64          `json:"file_id"`
	OrganizationID     *int64         `json:"organization_id,omitempty"`
	RowNumber          int            `json:"row_number"`
	RawName            string         `json:"raw_name"`
	NormalizedName     string         `json:"normalized_name"`
	SKU                string         `json:"sku,omitempty"`
	Price              money.Amount   `json:"price"`
	Discount           float64        `json:"discount"`
	PriceAfterDiscount money.Amount   `json:"price_after_discount"`
	MatchedProductID   *int64         `json:"matched_product_id,omitempty"`
	MatchConfidence    float64        `json:"match_confidence"`
	MatchMethod        MatchMethod    `json:"match_method"`
	Meta               map[string]any `json:"meta,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}
