package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Whether the AI toggle can honestly be offered.
//
// Rendering it enabled against a Gateway that cannot serve it produces a run
// that silently does no AI at all — the buyer ticks a box, waits, and gets
// deterministic results with no explanation. Every reason below is one the
// operator can act on, so the message says which.
//
// The checks are ordered cheapest first and none of them calls the Gateway: a
// page render must not wait on a network round trip to a service that may be
// the thing that is down.
func (h *UIHandler) smartOrderAIState(ctx context.Context, orgID int64, langOptional ...string) (bool, string) {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	if h.aiClient == nil {
		return false, i18n.T(lang, "smartorder.ai_not_enabled")
	}
	if !h.aiClient.Enabled() {
		return false, i18n.T(lang, "smartorder.ai_gateway_down")
	}
	if h.orgSvc == nil || orgID <= 0 {
		return false, i18n.T(lang, "smartorder.ai_org_members_only")
	}

	// An organisation with no virtual key has never been provisioned on the
	// Gateway, so every request it makes would be rejected.
	org, err := h.orgSvc.GetOrganization(ctx, orgID)
	if err != nil || org == nil {
		return false, i18n.T(lang, "smartorder.ai_subscription_check_failed")
	}
	if org.AIVirtualKey == "" {
		return false, i18n.T(lang, "smartorder.ai_key_missing")
	}

	return true, ""
}
