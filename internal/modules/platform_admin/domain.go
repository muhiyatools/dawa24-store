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
	Total           int
	Today           int
	ByCountry       map[string]int
	ByCity          map[string]int
	ByDevice        map[string]int
	ByOS            map[string]int
	ByBrowser       map[string]int
	TotalPharmacies int
	TotalSuppliers  int
	TotalOrders     int
	TotalProducts   int
	TotalGMV        string
	Recent          []*Visitor
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
	ID               int64          `json:"id"`
	OrganizationID   *int64         `json:"organization_id,omitempty"`
	OrganizationName string         `json:"organization_name,omitempty"`
	ActorUserID      *int64         `json:"actor_user_id,omitempty"`
	ActorName        string         `json:"actor_name,omitempty"`
	ActorEmail       string         `json:"actor_email,omitempty"`
	Module           string         `json:"module,omitempty"` // Section / Module (القسم)
	Action           string         `json:"action"`           // Action (الإجراء)
	ActionLabelAr    string         `json:"action_label_ar,omitempty"`
	Title            string         `json:"title,omitempty"`       // Title (العنوان)
	Description      string         `json:"description,omitempty"` // Description (الوصف)
	Severity         string         `json:"severity,omitempty"`    // info, warning, critical (الأهمية)
	IPAddress        string         `json:"ip_address,omitempty"`  // IP address (عنوان IP)
	Route            string         `json:"route,omitempty"`       // Route / URL path (المسار)
	UserAgent        string         `json:"user_agent,omitempty"`
	EntityType       string         `json:"entity_type"`
	EntityTypeAr     string         `json:"entity_type_ar,omitempty"`
	EntityID         string         `json:"entity_id"`
	Before           map[string]any `json:"before,omitempty"`
	After            map[string]any `json:"after,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

// AISettings defines configuration for artificial intelligence LLM providers.
type AISettings struct {
	Provider     string  `json:"provider"`      // gemini, openai, anthropic, deepseek, custom
	Model        string  `json:"model"`         // e.g. gemini-1.5-pro, gpt-4o
	APIKey       string  `json:"api_key"`       // API secret key
	EndpointURL  string  `json:"endpoint_url"`  // Custom or Gateway URL
	Temperature  float64 `json:"temperature"`   // 0.0 - 1.0
	MaxTokens    int     `json:"max_tokens"`    // default 2048
	SystemPrompt string  `json:"system_prompt"` // Default platform assistant prompt
	IsActive     bool    `json:"is_active"`
}

// GatewaySettings defines configuration for platform API gateways and external integrations.
type GatewaySettings struct {
	EndpointURL    string `json:"endpoint_url"`    // Gateway Base URL
	Environment    string `json:"environment"`     // production / sandbox
	TimeoutSeconds int    `json:"timeout_seconds"` // Request timeout
	APIKey         string `json:"api_key"`
	IsActive       bool   `json:"is_active"`
}

// SiteSettings defines public website branding, contact info, and social media.
type SiteSettings struct {
	SiteName        string            `json:"site_name"`
	SiteDescription string            `json:"site_description"`
	LogoURL         string            `json:"logo_url"`
	FaviconURL      string            `json:"favicon_url"`
	ContactEmail    string            `json:"contact_email"`
	SupportEmail    string            `json:"support_email"`
	Phone           string            `json:"phone"`
	WhatsApp        string            `json:"whatsapp"`
	Address         string            `json:"address"`
	SocialLinks     map[string]string `json:"social_links"` // facebook, twitter, instagram, linkedin, youtube, tiktok, snapchat, whatsapp, telegram
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

// SQLLog records an executed SQL query from the Developer SQL Console.
type SQLLog struct {
	ID           int64     `json:"id"`
	Query        string    `json:"query"`
	ExecutedBy   *int64    `json:"executed_by,omitempty"`
	ActorName    string    `json:"actor_name,omitempty"`
	DurationMS   int64     `json:"duration_ms"`
	RowsAffected int64     `json:"rows_affected"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SQLQueryResult represents the tabular output of a SQL query execution.
type SQLQueryResult struct {
	Columns      []string `json:"columns"`
	Rows         [][]any  `json:"rows"`
	DurationMS   int64    `json:"duration_ms"`
	RowsAffected int64    `json:"rows_affected"`
	Error        string   `json:"error,omitempty"`
	Truncated    bool     `json:"truncated,omitempty"`
	Message      string   `json:"message,omitempty"`
}

// ErrorLog represents a comprehensive diagnostic error / exception record.
type ErrorLog struct {
	ID               int64          `json:"id"`
	UserID           *int64         `json:"user_id,omitempty"`
	UserName         string         `json:"user_name,omitempty"`
	UserEmail        string         `json:"user_email,omitempty"`
	OrganizationName string         `json:"organization_name,omitempty"`
	ErrorLevel       string         `json:"error_level"` // CRITICAL, ERROR, WARNING, EXCEPTION
	ErrorMessage     string         `json:"error_message"`
	ExceptionClass   string         `json:"exception_class,omitempty"`
	StackTrace       string         `json:"stack_trace,omitempty"`
	FilePath         string         `json:"file_path,omitempty"`
	LineNumber       int            `json:"line_number,omitempty"`
	HTTPMethod       string         `json:"http_method,omitempty"`
	URLPath          string         `json:"url_path,omitempty"`
	IPAddress        string         `json:"ip_address,omitempty"`
	UserAgent        string         `json:"user_agent,omitempty"`
	RequestPayload   map[string]any `json:"request_payload,omitempty"`
	Status           string         `json:"status"` // NEW, INVESTIGATING, RESOLVED, IGNORED
	CreatedAt        time.Time      `json:"created_at"`
}

// ErrorLogFilter defines search and filter options for system diagnostic error logs.
type ErrorLogFilter struct {
	Level  string
	Status string
	Search string
	UserID *int64
	Limit  int
	Offset int
}
