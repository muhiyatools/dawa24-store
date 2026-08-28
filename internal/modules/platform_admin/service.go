package platformadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Service manages system configurations and geographical data.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a platform admin service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// GetSetting retrieves a configuration setting.
func (s *Service) GetSetting(ctx context.Context, key string) (*SystemSetting, error) {
	return s.repo.GetSetting(ctx, key)
}

// SetSetting writes a configuration setting.
func (s *Service) SetSetting(ctx context.Context, setting *SystemSetting) error {
	if err := setting.Validate(); err != nil {
		return err
	}
	if err := s.repo.SetSetting(ctx, setting); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "system setting updated", "key", setting.Key)
	return nil
}

// ListPublicSettings returns client-facing settings.
func (s *Service) ListPublicSettings(ctx context.Context) ([]*SystemSetting, error) {
	return s.repo.ListPublicSettings(ctx)
}

// ListCountries returns supported countries.
func (s *Service) ListCountries(ctx context.Context) ([]*Country, error) {
	return s.repo.ListCountries(ctx)
}

// ListGovernorates returns active governorates for a country.
func (s *Service) ListGovernorates(ctx context.Context, countryID int64) ([]*Governorate, error) {
	return s.repo.ListGovernorates(ctx, countryID)
}

// ListAllGovernorates returns all governorates for admin management.
func (s *Service) ListAllGovernorates(ctx context.Context, countryID int64) ([]*Governorate, error) {
	return s.repo.ListAllGovernorates(ctx, countryID)
}

// GetGovernorate returns a single governorate by ID.
func (s *Service) GetGovernorate(ctx context.Context, id int64) (*Governorate, error) {
	return s.repo.GetGovernorate(ctx, id)
}

// CreateGovernorate adds a new governorate.
func (s *Service) CreateGovernorate(ctx context.Context, g *Governorate) error {
	return s.repo.CreateGovernorate(ctx, g)
}

// UpdateGovernorate updates an existing governorate.
func (s *Service) UpdateGovernorate(ctx context.Context, g *Governorate) error {
	return s.repo.UpdateGovernorate(ctx, g)
}

// ToggleGovernorateStatus toggles the active status of a governorate.
func (s *Service) ToggleGovernorateStatus(ctx context.Context, id int64) error {
	return s.repo.ToggleGovernorateStatus(ctx, id)
}

// ListCities returns active cities for a country.
func (s *Service) ListCities(ctx context.Context, countryID int64) ([]*City, error) {
	return s.repo.ListCities(ctx, countryID)
}

// ListAllCities returns all cities (both active and inactive) for admin management.
func (s *Service) ListAllCities(ctx context.Context, countryID int64) ([]*City, error) {
	return s.repo.ListAllCities(ctx, countryID)
}

// ListCitiesByGovernorate returns all cities belonging to a specific governorate.
func (s *Service) ListCitiesByGovernorate(ctx context.Context, governorateID int64) ([]*City, error) {
	return s.repo.ListCitiesByGovernorate(ctx, governorateID)
}

// GetCity returns a single city by ID.
func (s *Service) GetCity(ctx context.Context, id int64) (*City, error) {
	return s.repo.GetCity(ctx, id)
}

// ToggleCityStatus toggles the active state of a city.
func (s *Service) ToggleCityStatus(ctx context.Context, id int64) error {
	return s.repo.ToggleCityStatus(ctx, id)
}

// CreateCity adds a new city with spatial coordinates.
func (s *Service) CreateCity(ctx context.Context, c *City) error {
	return s.repo.CreateCity(ctx, c)
}

// UpdateCity updates an existing city with new data.
func (s *Service) UpdateCity(ctx context.Context, c *City) error {
	return s.repo.UpdateCity(ctx, c)
}

// ListCurrencies returns supported currencies.
func (s *Service) ListCurrencies(ctx context.Context) ([]*Currency, error) {
	return s.repo.ListCurrencies(ctx)
}

// ListLanguages returns supported UI languages.
func (s *Service) ListLanguages(ctx context.Context) ([]*Language, error) {
	return s.repo.ListLanguages(ctx)
}

// SubmitContactMessage records a contact inquiry.
func (s *Service) SubmitContactMessage(ctx context.Context, m *ContactMessage) error {
	m.Status = "unread"
	return s.repo.CreateContactMessage(ctx, m)
}

// ListContactMessages returns contact inquiries.
func (s *Service) ListContactMessages(ctx context.Context, status string, limit, offset int) ([]*ContactMessage, error) {
	return s.repo.ListContactMessages(ctx, status, limit, offset)
}

// UpdateContactMessageStatus updates the read/in-progress status of a contact message.
func (s *Service) UpdateContactMessageStatus(ctx context.Context, id int64, status string) error {
	return s.repo.UpdateContactMessageStatus(ctx, id, status)
}

// DeleteContactMessage removes a contact message.
func (s *Service) DeleteContactMessage(ctx context.Context, id int64) error {
	return s.repo.DeleteContactMessage(ctx, id)
}

// ListContentBlocks returns all CMS content blocks.
func (s *Service) ListContentBlocks(ctx context.Context) ([]*ContentBlock, error) {
	return s.repo.ListContentBlocks(ctx)
}

// GetContentBlockByKey returns one CMS block by key.
func (s *Service) GetContentBlockByKey(ctx context.Context, key string) (*ContentBlock, error) {
	return s.repo.GetContentBlockByKey(ctx, key)
}

// UpsertContentBlock creates or updates a CMS block.
func (s *Service) UpsertContentBlock(ctx context.Context, b *ContentBlock) error {
	if b.Key == "" {
		return apperr.Validation("content.key_required", "A content block key is required.", nil)
	}
	return s.repo.UpsertContentBlock(ctx, b)
}

// ToggleContentBlockStatus toggles is_active for a content block.
func (s *Service) ToggleContentBlockStatus(ctx context.Context, id int64) error {
	return s.repo.ToggleContentBlockStatus(ctx, id)
}

// DeleteContentBlock removes a content block by ID.
func (s *Service) DeleteContentBlock(ctx context.Context, id int64) error {
	return s.repo.DeleteContentBlock(ctx, id)
}

// RecordVisitor records one visitor-session-day, deduplicated by key+day.
func (s *Service) RecordVisitor(ctx context.Context, v *Visitor) error {
	if v.VisitorKey == "" {
		return nil
	}
	return s.repo.RecordVisitor(ctx, v)
}

// VisitorAnalytics returns the aggregate traffic view for the admin page.
func (s *Service) VisitorAnalytics(ctx context.Context, limit int) (*VisitorAnalytics, error) {
	return s.repo.VisitorAnalytics(ctx, limit)
}

// ListAuditLog returns the platform audit trail, newest first.
func (s *Service) ListAuditLog(ctx context.Context, limit, offset int) ([]*AuditEntry, error) {
	return s.repo.ListAuditLog(ctx, limit, offset)
}

// QueueStats returns River job counts grouped by state.
func (s *Service) QueueStats(ctx context.Context) (map[string]int, error) {
	return s.repo.QueueStats(ctx)
}

// ListPolicyVersions returns all historical and current versions of a policy.
func (s *Service) ListPolicyVersions(ctx context.Context, policyKey string) ([]*Policy, error) {
	return s.repo.ListPolicyVersions(ctx, policyKey)
}

// GetPolicyVersion retrieves a policy by key and version string.
func (s *Service) GetPolicyVersion(ctx context.Context, policyKey, version string) (*Policy, error) {
	return s.repo.GetPolicyVersion(ctx, policyKey, version)
}

// GetActivePolicy retrieves the current live published policy for public rendering.
func (s *Service) GetActivePolicy(ctx context.Context, policyKey string) (*Policy, error) {
	return s.repo.GetActivePolicy(ctx, policyKey)
}

// CreatePolicyVersion saves a new policy draft version.
func (s *Service) CreatePolicyVersion(ctx context.Context, p *Policy) error {
	if p.PolicyKey == "" {
		return apperr.Validation("policy.key_required", "Policy key is required.", nil)
	}
	if p.Version == "" {
		p.Version = "1.0"
	}
	if err := s.repo.CreatePolicyVersion(ctx, p); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "policy version created", "key", p.PolicyKey, "version", p.Version)
	return nil
}

// PublishPolicyVersion makes a specific version the live active document.
func (s *Service) PublishPolicyVersion(ctx context.Context, id int64) error {
	if err := s.repo.PublishPolicyVersion(ctx, id); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "policy version published", "id", id)
	return nil
}

// GetAISettings loads AI configuration from database settings.
func (s *Service) GetAISettings(ctx context.Context) (*AISettings, error) {
	setting, err := s.repo.GetSetting(ctx, "ai_configuration")
	if err != nil || setting == nil || setting.Value == nil {
		return &AISettings{
			Temperature:  0.7,
			MaxTokens:    2048,
			SystemPrompt: "أنت المساعد الذكي لمنصة دواء 24، متخصص في مساعدة الصيدليات والموردين في العمليات الدوائية وإدارة المخزون والتوريد.",
			IsActive:     true,
		}, nil
	}
	v := setting.Value
	ai := &AISettings{
		APIKey:       getString(v, "api_key", ""),
		EndpointURL:  getString(v, "endpoint_url", "https://api.muhiya.com"),
		Temperature:  getFloat(v, "temperature", 0.7),
		MaxTokens:    getInt(v, "max_tokens", 2048),
		SystemPrompt: getString(v, "system_prompt", ""),
		IsActive:     getBool(v, "is_active", true),
	}
	return ai, nil
}

// SaveAISettings writes AI configuration to database settings.
func (s *Service) SaveAISettings(ctx context.Context, ai *AISettings) error {
	val := map[string]any{
		"api_key":       ai.APIKey,
		"endpoint_url":  ai.EndpointURL,
		"temperature":   ai.Temperature,
		"max_tokens":    ai.MaxTokens,
		"system_prompt": ai.SystemPrompt,
		"is_active":     ai.IsActive,
	}
	return s.repo.SetSetting(ctx, &SystemSetting{
		Key:         "ai_configuration",
		Value:       val,
		Description: "Platform AI Configuration",
		IsPublic:    false,
	})
}

// GetGatewaySettings loads API Gateway settings from database.
func (s *Service) GetGatewaySettings(ctx context.Context) (*GatewaySettings, error) {
	setting, err := s.repo.GetSetting(ctx, "gateway_configuration")
	if err != nil || setting == nil || setting.Value == nil {
		return &GatewaySettings{
			EndpointURL:    "https://api.dawa24.com/v1",
			Environment:    "production",
			TimeoutSeconds: 30,
			IsActive:       true,
		}, nil
	}
	v := setting.Value
	gw := &GatewaySettings{
		EndpointURL:    getString(v, "endpoint_url", "https://api.dawa24.com/v1"),
		Environment:    getString(v, "environment", "production"),
		TimeoutSeconds: getInt(v, "timeout_seconds", 30),
		APIKey:         getString(v, "api_key", ""),
		IsActive:       getBool(v, "is_active", true),
		VirtualKey:     getString(v, "virtual_key", ""),
		AIUserID:       getString(v, "ai_user_id", ""),
		FastModel:      getString(v, "fast_model", ""),
		QualityModel:   getString(v, "quality_model", ""),
		AIPlanID:       getString(v, "ai_plan_id", ""),
	}
	if gw.FastModel == "nemotron-3.5-lightning" {
		gw.FastModel = "qwen3.7-flash"
	}
	if gw.QualityModel == "nemotron-3.5-lightning" {
		gw.QualityModel = "qwen3.7-flash"
	}
	return gw, nil
}

// SaveGatewaySettings writes API Gateway settings to database.
func (s *Service) SaveGatewaySettings(ctx context.Context, gw *GatewaySettings) error {
	val := map[string]any{
		"endpoint_url":    gw.EndpointURL,
		"environment":     gw.Environment,
		"timeout_seconds": gw.TimeoutSeconds,
		"api_key":         gw.APIKey,
		"is_active":       gw.IsActive,
		"virtual_key":     gw.VirtualKey,
		"ai_user_id":      gw.AIUserID,
		"fast_model":      gw.FastModel,
		"quality_model":   gw.QualityModel,
		"ai_plan_id":      gw.AIPlanID,
	}
	return s.repo.SetSetting(ctx, &SystemSetting{
		Key:         "gateway_configuration",
		Value:       val,
		Description: "Platform API Gateway Endpoints Configuration",
		IsPublic:    false,
	})
}

// GetSiteSettings loads public website branding, contact info, and social links.
func (s *Service) GetSiteSettings(ctx context.Context) (*SiteSettings, error) {
	setting, err := s.repo.GetSetting(ctx, "site_public_settings")
	if err != nil || setting == nil || setting.Value == nil {
		return &SiteSettings{
			SiteName:        "دواء 24",
			SiteDescription: "المنصة الرائدة لتوريد وتوزيع الأدوية والمستلزمات الطبية",
			LogoURL:         "/static/img/logo.png",
			FaviconURL:      "/static/img/logo.png",
			ContactEmail:    "info@dawa24.com",
			SupportEmail:    "support@dawa24.com",
			Phone:           "01065397000",
			WhatsApp:        "201065397000",
			Address:         "القاهرة، جمهورية مصر العربية",
			SocialLinks: map[string]string{
				"facebook":  "https://facebook.com/dawa24",
				"twitter":   "https://twitter.com/dawa24",
				"instagram": "https://instagram.com/dawa24",
				"linkedin":  "https://linkedin.com/company/dawa24",
				"youtube":   "https://youtube.com/@dawa24",
				"tiktok":    "https://tiktok.com/@dawa24",
				"snapchat":  "https://snapchat.com/add/dawa24",
				"whatsapp":  "https://wa.me/201065397000",
				"telegram":  "https://t.me/dawa24",
			},
		}, nil
	}
	v := setting.Value
	socialMap := map[string]string{}
	if rawSocial, ok := v["social_links"].(map[string]any); ok {
		for k, val := range rawSocial {
			socialMap[k] = fmt.Sprintf("%v", val)
		}
	} else if rawSocialStr, ok := v["social_links"].(map[string]string); ok {
		socialMap = rawSocialStr
	}

	ss := &SiteSettings{
		SiteName:        getString(v, "site_name", "دواء 24"),
		SiteDescription: getString(v, "site_description", "المنصة الرائدة لتوريد الأدوية"),
		LogoURL:         getString(v, "logo_url", "/static/img/logo.png"),
		FaviconURL:      getString(v, "favicon_url", "/static/img/logo.png"),
		ContactEmail:    getString(v, "contact_email", "info@dawa24.com"),
		SupportEmail:    getString(v, "support_email", "support@dawa24.com"),
		Phone:           getString(v, "phone", "01065397000"),
		WhatsApp:        getString(v, "whatsapp", "201065397000"),
		Address:         getString(v, "address", "القاهرة، جمهورية مصر العربية"),
		SocialLinks:     socialMap,
	}
	return ss, nil
}

// SaveSiteSettings writes public site settings and branding to database.
func (s *Service) SaveSiteSettings(ctx context.Context, ss *SiteSettings) error {
	val := map[string]any{
		"site_name":        ss.SiteName,
		"site_description": ss.SiteDescription,
		"logo_url":         ss.LogoURL,
		"favicon_url":      ss.FaviconURL,
		"contact_email":    ss.ContactEmail,
		"support_email":    ss.SupportEmail,
		"phone":            ss.Phone,
		"whatsapp":         ss.WhatsApp,
		"address":          ss.Address,
		"social_links":     ss.SocialLinks,
	}
	return s.repo.SetSetting(ctx, &SystemSetting{
		Key:         "site_public_settings",
		Value:       val,
		Description: "Public website branding, contact info, and social media",
		IsPublic:    true,
	})
}

func getString(m map[string]any, k, def string) string {
	if val, ok := m[k]; ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func getFloat(m map[string]any, k string, def float64) float64 {
	if val, ok := m[k]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return def
}

func getInt(m map[string]any, k string, def int) int {
	if val, ok := m[k]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case int64:
			return int(v)
		}
	}
	return def
}

func getBool(m map[string]any, k string, def bool) bool {
	if val, ok := m[k]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return def
}

// ExecuteSQL runs an arbitrary SQL query against PostgreSQL with duration and logging.
func (s *Service) ExecuteSQL(ctx context.Context, actorID *int64, actorName, query string) (*SQLQueryResult, error) {
	return s.repo.ExecuteSQL(ctx, actorID, actorName, query)
}

// ListSQLLogs returns previous executed queries from the SQL Console.
func (s *Service) ListSQLLogs(ctx context.Context, limit, offset int) ([]*SQLLog, error) {
	return s.repo.ListSQLLogs(ctx, limit, offset)
}

// LogError records a diagnostic error or exception.
func (s *Service) LogError(ctx context.Context, entry *ErrorLog) error {
	return s.repo.LogError(ctx, entry)
}

// ListErrorLogs searches and retrieves diagnostic error logs.
func (s *Service) ListErrorLogs(ctx context.Context, filter ErrorLogFilter) ([]*ErrorLog, int, error) {
	return s.repo.ListErrorLogs(ctx, filter)
}

// GetErrorLogByID returns an individual diagnostic error.
func (s *Service) GetErrorLogByID(ctx context.Context, id int64) (*ErrorLog, error) {
	return s.repo.GetErrorLogByID(ctx, id)
}

// UpdateErrorLogStatus updates error status (NEW, INVESTIGATING, RESOLVED, IGNORED).
func (s *Service) UpdateErrorLogStatus(ctx context.Context, id int64, status string) error {
	return s.repo.UpdateErrorLogStatus(ctx, id, status)
}

// GetErrorDiagnosticsMetrics returns aggregate error metrics for developer dashboard.
func (s *Service) GetErrorDiagnosticsMetrics(ctx context.Context) (total, critical24h, unresolved, affectedUsers int, err error) {
	return s.repo.GetErrorDiagnosticsMetrics(ctx)
}

// FetchGatewayModels dynamically connects to the AI Gateway endpoint to list available LLM models.
func (s *Service) FetchGatewayModels(ctx context.Context, endpointURL, apiKey string) ([]string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(endpointURL), "/")
	if endpoint == "" {
		endpoint = "https://api.muhiya.com"
	}

	reqURL := endpoint + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.log.WarnContext(ctx, "failed to query ai gateway models endpoint", "url", reqURL, "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []string `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range body.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	if len(models) == 0 && len(body.Models) > 0 {
		models = body.Models
	}

	return models, nil
}

// ListAuditLogByOrg returns audit trail entries filtered to an organization.
func (s *Service) ListAuditLogByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*AuditEntry, error) {
	return s.repo.ListAuditLogByOrg(ctx, orgID, limit, offset)
}
