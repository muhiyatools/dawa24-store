package billing

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// CooldownSettings configures plan change cooldowns.
type CooldownSettings struct {
	CooldownDays int `json:"cooldown_days"` // Days a paid plan must wait before changing plan (default 25)
	MinDays      int `json:"min_days"`      // Absolute minimum wait after any purchase (default 2)
}

// SubscriptionCooldownInfo summarizes the active cooldown status for an organization or user.
type SubscriptionCooldownInfo struct {
	IsOnCooldown      bool      `json:"is_on_cooldown"`
	EarliestAllowedAt time.Time `json:"earliest_allowed_at"`
	CurrentPlanSlug   string    `json:"current_plan_slug"`
	CurrentPlanName   string    `json:"current_plan_name"`
	CooldownDays      int       `json:"cooldown_days"`
}

// GetCooldownSettings reads the configured cooldown thresholds from system settings,
// falling back to defaults (25 days for paid plan cooldown, 2 days minimum wait).
func (s *Service) GetCooldownSettings(ctx context.Context) CooldownSettings {
	settings := CooldownSettings{
		CooldownDays: 25,
		MinDays:      2,
	}
	if s == nil || s.repo == nil {
		return settings
	}
	if val, err := s.repo.GetSetting(ctx, "subscription_change_cooldown_days"); err == nil && val != "" {
		if d := parseSettingInt(val); d > 0 {
			settings.CooldownDays = d
		}
	}
	if val, err := s.repo.GetSetting(ctx, "subscription_change_min_days"); err == nil && val != "" {
		if d := parseSettingInt(val); d > 0 {
			settings.MinDays = d
		}
	}
	return settings
}

func parseSettingInt(val string) int {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	// Try direct int first
	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	// Try JSON map
	var m map[string]any
	if err := json.Unmarshal([]byte(val), &m); err == nil {
		if v, ok := m["days"].(float64); ok && v > 0 {
			return int(v)
		}
		if v, ok := m["value"].(float64); ok && v > 0 {
			return int(v)
		}
		if v, ok := m["days"].(string); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		}
		if v, ok := m["value"].(string); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		}
	}
	return 0
}

// CheckSubscriptionChangeCooldown determines if an org or user is currently prohibited
// from changing their subscription plan due to the active cooldown window.
func (s *Service) CheckSubscriptionChangeCooldown(
	ctx context.Context,
	userID int64,
	orgID *int64,
	targetPlanSlug string,
) (SubscriptionCooldownInfo, error) {
	info := SubscriptionCooldownInfo{}
	if s == nil || s.repo == nil {
		return info, nil
	}

	var currentSub *Subscription
	if orgID != nil && *orgID > 0 {
		currentSub, _ = s.repo.GetActiveSubscriptionByOrg(ctx, *orgID)
	} else if userID > 0 {
		currentSub, _ = s.repo.GetActiveSubscription(ctx, userID)
	}

	if currentSub == nil {
		return info, nil
	}

	currentPlan, err := s.repo.GetPlanByID(ctx, currentSub.PlanID)
	if err != nil || currentPlan == nil {
		return info, nil
	}

	info.CurrentPlanSlug = currentPlan.Slug
	info.CurrentPlanName = currentPlan.Name.Get("ar")
	if info.CurrentPlanName == "" {
		info.CurrentPlanName = currentPlan.Name.Get("en")
	}

	// Rule 1: Free / default plan is exempt. Upgrading from free is ALWAYS allowed immediately.
	if currentPlan.IsDefault || currentPlan.PriceMonth.IsZero() {
		return info, nil
	}

	// Current plan is a paid plan.
	settings := s.GetCooldownSettings(ctx)
	effectiveDays := settings.CooldownDays
	if effectiveDays < settings.MinDays {
		effectiveDays = settings.MinDays
	}
	info.CooldownDays = effectiveDays

	earliestAllowed := currentSub.StartsAt.Add(time.Duration(effectiveDays) * 24 * time.Hour)
	info.EarliestAllowedAt = earliestAllowed

	now := time.Now().UTC()
	if now.Before(earliestAllowed) {
		info.IsOnCooldown = true
	}

	return info, nil
}
