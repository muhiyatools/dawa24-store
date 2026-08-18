package platformadmin

import (
	"context"
	"log/slog"

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

// ListCities returns cities for a country.
func (s *Service) ListCities(ctx context.Context, countryID int64) ([]*City, error) {
	return s.repo.ListCities(ctx, countryID)
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

// ListTranslations returns all UI translations.
func (s *Service) ListTranslations(ctx context.Context) ([]*Translation, error) {
	return s.repo.ListTranslations(ctx)
}

// UpsertTranslation creates or updates a translation override.
func (s *Service) UpsertTranslation(ctx context.Context, t *Translation) error {
	if t.Key == "" {
		return apperr.Validation("translation.key_required", "A translation key is required.", nil)
	}
	return s.repo.UpsertTranslation(ctx, t)
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

// PublishPolicyVersion marks a version as active and unpublishes other versions.
func (s *Service) PublishPolicyVersion(ctx context.Context, id int64) error {
	if err := s.repo.PublishPolicyVersion(ctx, id); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "policy version published", "id", id)
	return nil
}

