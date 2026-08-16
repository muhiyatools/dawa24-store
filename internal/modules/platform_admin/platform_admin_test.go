package platformadmin_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockPlatformAdminRepo struct {
	settings  map[string]*platformadmin.SystemSetting
	countries []*platformadmin.Country
	cities    map[int64][]*platformadmin.City
}

func newMockPlatformAdminRepo() *mockPlatformAdminRepo {
	return &mockPlatformAdminRepo{
		settings: map[string]*platformadmin.SystemSetting{},
		cities:   map[int64][]*platformadmin.City{},
	}
}

func (m *mockPlatformAdminRepo) GetSetting(_ context.Context, key string) (*platformadmin.SystemSetting, error) {
	s, ok := m.settings[key]
	if !ok {
		return nil, apperr.NotFound("setting")
	}
	return s, nil
}

func (m *mockPlatformAdminRepo) SetSetting(_ context.Context, s *platformadmin.SystemSetting) error {
	m.settings[s.Key] = s
	return nil
}

func (m *mockPlatformAdminRepo) ListPublicSettings(_ context.Context) ([]*platformadmin.SystemSetting, error) {
	var list []*platformadmin.SystemSetting
	for _, s := range m.settings {
		if s.IsPublic {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockPlatformAdminRepo) ListCountries(_ context.Context) ([]*platformadmin.Country, error) {
	return m.countries, nil
}

func (m *mockPlatformAdminRepo) ListCities(_ context.Context, countryID int64) ([]*platformadmin.City, error) {
	return m.cities[countryID], nil
}

func TestPlatformAdminSettingsAndGeo(t *testing.T) {
	ctx := context.Background()
	repo := newMockPlatformAdminRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := platformadmin.NewService(repo, logger)

	// 1. Set Setting
	err := svc.SetSetting(ctx, &platformadmin.SystemSetting{
		Key:      "site_title",
		Value:    map[string]any{"ar": "دوا 24", "en": "Dawa 24"},
		IsPublic: true,
	})
	if err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	setting, err := svc.GetSetting(ctx, "site_title")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if setting.Key != "site_title" || !setting.IsPublic {
		t.Errorf("unexpected setting: %+v", setting)
	}

	// 2. Geography Data
	repo.countries = []*platformadmin.Country{
		{ID: 1, Code: "EG", Name: i18n.New("مصر", "Egypt"), PhoneCode: "+20", Currency: "EGP", IsActive: true},
	}
	repo.cities[1] = []*platformadmin.City{
		{ID: 10, CountryID: 1, Name: i18n.New("القاهرة", "Cairo"), IsActive: true},
		{ID: 11, CountryID: 1, Name: i18n.New("الجيزة", "Giza"), IsActive: true},
	}

	countries, _ := svc.ListCountries(ctx)
	if len(countries) != 1 || countries[0].Code != "EG" {
		t.Errorf("unexpected countries: %+v", countries)
	}

	cities, _ := svc.ListCities(ctx, 1)
	if len(cities) != 2 {
		t.Errorf("unexpected cities: %+v", cities)
	}
}
