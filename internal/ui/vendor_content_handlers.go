package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorPoliciesPage renders the vendor's return, payment, and shipping policies editor.
func (h *UIHandler) VendorPoliciesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/policies", http.StatusSeeOther)
		return
	}

	policyMap := make(map[string]string)
	if h.orgSvc != nil {
		if policies, err := h.orgSvc.ListPolicies(ctx, actor.OrganizationID); err == nil {
			for _, p := range policies {
				if p != nil {
					policyMap[p.PolicyType] = p.Content
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorPoliciesPage(policyMap, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor policies", "error", err)
	}
}

// VendorPoliciesSubmit saves the vendor's updated policy text.
func (h *UIHandler) VendorPoliciesSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/policies", http.StatusSeeOther)
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/policies", "error", "خدمة إدارة المنشآت غير متوفرة حالياً.")
		return
	}

	_ = r.ParseForm()
	shipping := r.PostFormValue("shipping_policy")
	returns := r.PostFormValue("returns_policy")
	terms := r.PostFormValue("terms_policy")

	policies := []*org.Policy{
		{Title: "سياسة الشحن والتسليم", Content: shipping, PolicyType: "shipping", IsActive: true},
		{Title: "سياسة المرتجعات والتوالف", Content: returns, PolicyType: "returns", IsActive: true},
		{Title: "شروط السداد والدفع الآجل", Content: terms, PolicyType: "terms", IsActive: true},
	}

	if err := h.orgSvc.SavePolicies(ctx, actor.OrganizationID, policies); err != nil {
		h.log.ErrorContext(ctx, "save vendor policies", "error", err)
		h.redirectWithNotice(w, r, "/vendor/policies", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/policies", "success", "تم حفظ سياسات المنشأة بنجاح.")
}

// VendorSocialMediaPage renders social media accounts editor for the supplier profile.
func (h *UIHandler) VendorSocialMediaPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/social-media", http.StatusSeeOther)
		return
	}

	linksMap := make(map[string]string)
	if h.orgSvc != nil {
		if links, err := h.orgSvc.ListSocialMedia(ctx, actor.OrganizationID); err == nil {
			for _, l := range links {
				if l != nil {
					linksMap[l.Platform] = l.URL
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorSocialMediaPage(linksMap, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor social media", "error", err)
	}
}

// VendorSocialMediaSubmit saves the vendor's social media accounts.
func (h *UIHandler) VendorSocialMediaSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/social-media", http.StatusSeeOther)
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/social-media", "error", "خدمة إدارة المنشآت غير متوفرة حالياً.")
		return
	}

	_ = r.ParseForm()
	whatsapp := r.PostFormValue("whatsapp")
	facebook := r.PostFormValue("facebook")
	linkedin := r.PostFormValue("linkedin")

	links := []*org.SocialMedia{
		{Platform: "whatsapp", URL: whatsapp},
		{Platform: "facebook", URL: facebook},
		{Platform: "linkedin", URL: linkedin},
	}

	if err := h.orgSvc.SaveSocialMedia(ctx, actor.OrganizationID, links); err != nil {
		h.log.ErrorContext(ctx, "save vendor social media", "error", err)
		h.redirectWithNotice(w, r, "/vendor/social-media", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/social-media", "success", "تم حفظ قنوات التواصل الاجتماعي بنجاح.")
}
