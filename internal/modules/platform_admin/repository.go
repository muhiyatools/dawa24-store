package platformadmin

import (
	"context"
)

// Repository defines storage operations for system settings and locations.
type Repository interface {
	GetSetting(ctx context.Context, key string) (*SystemSetting, error)
	SetSetting(ctx context.Context, s *SystemSetting) error
	ListPublicSettings(ctx context.Context) ([]*SystemSetting, error)

	ListCountries(ctx context.Context) ([]*Country, error)
	ListCities(ctx context.Context, countryID int64) ([]*City, error)

	ListCurrencies(ctx context.Context) ([]*Currency, error)
	ListLanguages(ctx context.Context) ([]*Language, error)
	CreateContactMessage(ctx context.Context, m *ContactMessage) error
	ListContactMessages(ctx context.Context, status string, limit, offset int) ([]*ContactMessage, error)

	ListContentBlocks(ctx context.Context) ([]*ContentBlock, error)
	GetContentBlockByKey(ctx context.Context, key string) (*ContentBlock, error)
	UpsertContentBlock(ctx context.Context, b *ContentBlock) error
	GetPublishedPolicy(ctx context.Context, slug string) (*PrivacyPolicy, error)

	RecordVisitor(ctx context.Context, v *Visitor) error
	VisitorAnalytics(ctx context.Context, limit int) (*VisitorAnalytics, error)
}
