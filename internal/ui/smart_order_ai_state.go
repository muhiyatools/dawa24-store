package ui

import (
	"context"
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
func (h *UIHandler) smartOrderAIState(ctx context.Context, orgID int64) (bool, string) {
	if h.aiClient == nil {
		return false, "المطابقة الذكية غير مفعّلة على هذه المنصة."
	}
	if !h.aiClient.Enabled() {
		return false, "بوابة الذكاء الاصطناعي متوقفة حاليًا. ستعمل المطابقة الحتمية وحدها."
	}
	if h.orgSvc == nil || orgID <= 0 {
		return false, "المطابقة الذكية متاحة لأعضاء المؤسسات فقط."
	}

	// An organisation with no virtual key has never been provisioned on the
	// Gateway, so every request it makes would be rejected.
	org, err := h.orgSvc.GetOrganization(ctx, orgID)
	if err != nil || org == nil {
		return false, "تعذّر التحقق من اشتراك مؤسستك في خدمات الذكاء الاصطناعي."
	}
	if org.AIVirtualKey == "" {
		return false, "لم يتم تفعيل الذكاء الاصطناعي لمؤسستك بعد. تواصل مع الإدارة لتفعيله."
	}

	return true, ""
}
