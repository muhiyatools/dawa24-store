package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// Whether the vendor import may offer its AI switch.
//
// Same discipline as the smart order's: never render an enabled toggle the
// platform cannot honour. A vendor who ticks a box, waits, and gets purely
// deterministic results has been told nothing about why — and every reason
// below is one an operator can go and fix.
//
// None of the checks calls the Gateway. Rendering a settings page must not wait
// on a network round trip to the service that may itself be the thing that is
// down.
func (h *UIHandler) vendorImportAIState(ctx context.Context) (bool, string) {
	if h.ingSvc == nil || !h.ingSvc.AIAvailable() {
		return false, "المطابقة الذكية غير مفعّلة على هذه المنصة."
	}
	if h.aiClient == nil || !h.aiClient.Enabled() {
		return false, "بوابة الذكاء الاصطناعي متوقفة حالياً. ستعمل المطابقة الحتمية وحدها."
	}
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		return false, "المطابقة الذكية متاحة لأعضاء المؤسسات فقط."
	}
	if h.orgSvc == nil {
		return false, "تعذّر التحقق من اشتراك مؤسستك في خدمات الذكاء الاصطناعي."
	}
	org, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID)
	if err != nil || org == nil {
		return false, "تعذّر التحقق من اشتراك مؤسستك في خدمات الذكاء الاصطناعي."
	}
	if org.AIVirtualKey == "" {
		return false, "لم يتم تفعيل الذكاء الاصطناعي لمؤسستك بعد. تواصل مع الإدارة لتفعيله."
	}
	return true, ""
}
