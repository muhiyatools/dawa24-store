package platformadmin

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockPlatformAdminRepo struct {
	settings   map[string]*SystemSetting
	countries  []*Country
	cities     map[int64][]*City
	currencies []*Currency
	languages  []*Language
	messages   []*ContactMessage
}

func newMockPlatformAdminRepo() *mockPlatformAdminRepo {
	return &mockPlatformAdminRepo{
		settings: map[string]*SystemSetting{},
		cities:   map[int64][]*City{},
	}
}

func (m *mockPlatformAdminRepo) GetSetting(_ context.Context, key string) (*SystemSetting, error) {
	s, ok := m.settings[key]
	if !ok {
		return nil, apperr.NotFound("setting")
	}
	return s, nil
}

func (m *mockPlatformAdminRepo) SetSetting(_ context.Context, s *SystemSetting) error {
	m.settings[s.Key] = s
	return nil
}

func (m *mockPlatformAdminRepo) ListPublicSettings(_ context.Context) ([]*SystemSetting, error) {
	var list []*SystemSetting
	for _, s := range m.settings {
		if s.IsPublic {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockPlatformAdminRepo) ListCountries(_ context.Context) ([]*Country, error) {
	return m.countries, nil
}

func (m *mockPlatformAdminRepo) ListCities(_ context.Context, countryID int64) ([]*City, error) {
	return m.cities[countryID], nil
}

func (m *mockPlatformAdminRepo) ListCurrencies(_ context.Context) ([]*Currency, error) {
	return m.currencies, nil
}

func (m *mockPlatformAdminRepo) ListLanguages(_ context.Context) ([]*Language, error) {
	return m.languages, nil
}

func (m *mockPlatformAdminRepo) CreateContactMessage(_ context.Context, msg *ContactMessage) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockPlatformAdminRepo) ListContactMessages(_ context.Context, status string, limit, offset int) ([]*ContactMessage, error) {
	return m.messages, nil
}

func TestPlatformAdminSettingsAndGeo(t *testing.T) {
	ctx := context.Background()
	repo := newMockPlatformAdminRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Set Setting
	err := svc.SetSetting(ctx, &SystemSetting{
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

	publicSettings, err := svc.ListPublicSettings(ctx)
	if err != nil || len(publicSettings) != 1 {
		t.Fatalf("ListPublicSettings failed: %v", err)
	}

	// 2. Geography Data
	repo.countries = []*Country{
		{ID: 1, Code: "EG", Name: i18n.New("مصر", "Egypt"), PhoneCode: "+20", Currency: "EGP", IsActive: true},
	}
	repo.cities[1] = []*City{
		{ID: 10, CountryID: 1, Name: i18n.New("القاهرة", "Cairo"), IsActive: true},
	}
	repo.currencies = []*Currency{
		{Code: "EGP", Name: i18n.New("جنيه مصري", "Egyptian Pound"), Symbol: "EGP", IsActive: true},
	}
	repo.languages = []*Language{
		{Code: "ar", Name: "العربية", Dir: "rtl", IsActive: true},
	}

	countries, err := svc.ListCountries(ctx)
	if err != nil || len(countries) != 1 {
		t.Fatalf("ListCountries failed: %v", err)
	}

	cities, err := svc.ListCities(ctx, 1)
	if err != nil || len(cities) != 1 {
		t.Fatalf("ListCities failed: %v", err)
	}

	currencies, err := svc.ListCurrencies(ctx)
	if err != nil || len(currencies) != 1 {
		t.Fatalf("ListCurrencies failed: %v", err)
	}

	languages, err := svc.ListLanguages(ctx)
	if err != nil || len(languages) != 1 {
		t.Fatalf("ListLanguages failed: %v", err)
	}

	// 3. Contact Messages
	err = svc.SubmitContactMessage(ctx, &ContactMessage{
		Name:    "Amr",
		Email:   "amr@example.com",
		Message: "Inquiry regarding API access",
	})
	if err != nil {
		t.Fatalf("SubmitContactMessage failed: %v", err)
	}

	msgs, err := svc.ListContactMessages(ctx, "unread", 10, 0)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("ListContactMessages failed: %v", err)
	}
}
