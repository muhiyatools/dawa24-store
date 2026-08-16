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
}
