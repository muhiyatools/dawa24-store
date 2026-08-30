package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SettingsIndex renders the comprehensive unified tab-based account settings hub.
func (h *UIHandler) SettingsIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}
	if actor.IsVendor() {
		http.Redirect(w, r, "/vendor/organization", http.StatusSeeOther)
		return
	}

	var user *identity.User
	var sessions []*identity.Session
	var sessionPlans []*identity.SessionPlan
	var paymentMethods []*billing.UserPaymentMethod
	var wallet *billing.Wallet
	var txs []*billing.WalletTransaction

	var platformPaymentMethods []*billing.PlatformPaymentMethod

	if h.idSvc != nil {
		if me, err := h.idSvc.GetMe(ctx, actor.UserID, nil); err != nil {
			h.log.DebugContext(ctx, "settings: get me optional", "error", err)
		} else if me != nil {
			user = me.User
		}
		if sess, err := h.idSvc.ListSessions(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "settings: list sessions", "error", err)
		} else {
			sessions = sess
		}
		if plans, err := h.idSvc.ListSessionPlans(ctx); err != nil {
			h.log.WarnContext(ctx, "settings: list session plans", "error", err)
		} else {
			sessionPlans = plans
		}
	}

	var depositRequests []*billing.WalletDeposit

	if h.billSvc != nil {
		if pms, err := h.billSvc.ListPaymentMethods(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "settings: list payment methods", "error", err)
		} else {
			paymentMethods = pms
		}
		if ppms, err := h.billSvc.ListPlatformPaymentMethods(ctx, true); err != nil {
			h.log.WarnContext(ctx, "settings: list platform payment methods", "error", err)
		} else {
			platformPaymentMethods = ppms
		}
		if deps, err := h.billSvc.ListUserDeposits(ctx, actor.UserID, 50, 0); err != nil {
			h.log.WarnContext(ctx, "settings: list user deposits", "error", err)
		} else {
			depositRequests = deps
		}
		if w, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err != nil {
			h.log.DebugContext(ctx, "settings: get wallet optional", "error", err)
		} else if w != nil {
			wallet = w
			if list, err := h.billSvc.ListWalletTransactions(ctx, w.ID, 50, 0); err != nil {
				h.log.WarnContext(ctx, "settings: list wallet transactions", "error", err)
			} else {
				txs = list
			}
		}
	}

	if user == nil {
		user = &identity.User{
			ID:    actor.UserID,
			Email: "user@dawa24.eg",
			Name:  i18n.Text{i18n.AR: "طبيب / صيدلي معتمد", i18n.EN: "Verified Pharmacist"},
			Role:  actor.Role,
		}
	}

	data := pages.UnifiedSettingsData{
		User:                   user,
		Wallet:                 wallet,
		Transactions:           txs,
		DepositRequests:        depositRequests,
		PaymentMethods:         paymentMethods,
		PlatformPaymentMethods: platformPaymentMethods,
		Sessions:               sessions,
		SessionPlans:           sessionPlans,
		ActiveTab:              "profile",
	}

	h.renderPage(ctx, w, "render unified settings page", pages.UnifiedSettingsPage(data, lang, dir))
}

// SettingsOrgUpdateSubmit saves organization profile fields.
func (h *UIHandler) SettingsOrgUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/organization", http.StatusSeeOther)
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	o, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID)
	if err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", h.safeMessage(err, langOf(r)))
		return
	}
	o.LegalName = r.PostFormValue("legal_name")
	o.TradeName = i18n.New(r.PostFormValue("trade_name_ar"), r.PostFormValue("trade_name_en"))
	o.TaxNumber = r.PostFormValue("tax_number")
	o.CommercialRegister = r.PostFormValue("commercial_register")

	if err := h.orgSvc.UpdateOrganization(ctx, o); err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/settings/organization", "success", "تم حفظ بيانات المؤسسة.")
}
