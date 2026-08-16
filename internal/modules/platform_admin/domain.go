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
	IsActive  bool      `json:"is_active"`
}

// Validate ensures setting keys are non-empty.
func (s *SystemSetting) Validate() error {
	if s.Key == "" {
		return apperr.Validation("setting.key_required", "Setting key is required.", nil)
	}
	return nil
}
