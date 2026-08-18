// Package platformadmin handles system-wide configurations, geographical master data,
// and platform administration settings.
package platformadmin

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// SystemSetting represents a global system key-value configuration.
type SystemSetting struct {
	Key         string         `json:"key"`
	Value       map[string]any `json:"value"`
	Description string         `json:"description,omitempty"`
	IsPublic    bool           `json:"is_public"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Country represents a supported operating country.
type Country struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      i18n.Text `json:"name"`
	PhoneCode string    `json:"phone_code"`
	Currency  string    `json:"currency"`
	IsActive  bool      `json:"is_active"`
}

// City represents an operating city within a country.
type City struct {
	ID        int64     `json:"id"`
	CountryID int64     `json:"country_id"`
	Name      i18n.Text `json:"name"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	IsActive  bool      `json:"is_active"`
}


// Validate ensures setting keys are non-empty.
func (s *SystemSetting) Validate() error {
	if s.Key == "" {
		return apperr.Validation("setting.key_required", "Setting key is required.", nil)
	}
	return nil
}

// Currency represents a supported monetary currency.
type Currency struct {
	ID              int64     `json:"id"`
	Code            string    `json:"code"`
	Name            i18n.Text `json:"name"`
	Symbol          string    `json:"symbol"`
	ExchangeRateEGP float64   `json:"exchange_rate_egp"`
	IsActive        bool      `json:"is_active"`
	IsDefault       bool      `json:"is_default"`
}

// Language represents a supported UI locale language.
type Language struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Dir       string `json:"dir"` // rtl or ltr
	IsActive  bool   `json:"is_active"`
	IsDefault bool   `json:"is_default"`
}

// ContactMessage represents a public contact inquiry form submission.
type ContactMessage struct {
	ID        int64     `json:"id"`
	PublicID  string    `json:"public_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone,omitempty"`
	Subject   string    `json:"subject"`
	Message   string    `json:"message"`
	Status    string    `json:"status"` // unread, read, resolved
	CreatedAt time.Time `json:"created_at"`
}

// Document represents an organization uploaded official verification document.
type Document struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID *int64    `json:"organization_id,omitempty"`
	Title          string    `json:"title"`
	DocumentType   string    `json:"document_type"`
	StorageKey     string    `json:"storage_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// ContentBlock is an editable bilingual CMS block (legacy what_in_contents).
type ContentBlock struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Title     i18n.Text `json:"title"`
	Body      i18n.Text `json:"body"`
	Position  string    `json:"position"`
	SortOrder int       `json:"sort_order"`
	IsActive  bool      `json:"is_active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Visitor is one analytics row: a unique visitor on a unique day.
type Visitor struct {
	ID         int64     `json:"id"`
	VisitorKey string    `json:"visitor_key"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	Browser    string    `json:"browser"`
	Device     string    `json:"device"`
	OS         string    `json:"os"`
	Country    string    `json:"country"`
	City       string    `json:"city"`
	VisitedAt  time.Time `json:"visited_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// VisitorAnalytics is the aggregate view for the admin analytics page.
type VisitorAnalytics struct {
	Total     int
	Today     int
	ByDevice  map[string]int
	ByOS      map[string]int
	ByBrowser map[string]int
	Recent    []*Visitor
}

// Translation is one bilingual UI string override (platform_admin.translations).
type Translation struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Group     string    `json:"translation_group"`
	Text      i18n.Text `json:"text"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditEntry is one row of the platform audit trail, for /admin/audit.
type AuditEntry struct {
	ID             int64          `json:"id"`
	OrganizationID *int64         `json:"organization_id,omitempty"`
	ActorUserID    *int64         `json:"actor_user_id,omitempty"`
	Action         string         `json:"action"`
	EntityType     string         `json:"entity_type"`
	EntityID       string         `json:"entity_id"`
	Before         map[string]any `json:"before,omitempty"`
	After          map[string]any `json:"after,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// Policy represents a versioned platform policy document (Terms, Privacy, Refund, etc.).
type Policy struct {
	ID          int64      `json:"id"`
	PolicyKey   string     `json:"policy_key"`
	Version     string     `json:"version"`
	Title       i18n.Text  `json:"title"`
	Content     i18n.Text  `json:"content"`
	Summary     i18n.Text  `json:"summary"`
	IsPublished bool       `json:"is_published"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedBy   *int64     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

