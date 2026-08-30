package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminFinancePage renders the unified financial management hub.
func (h *UIHandler) AdminFinancePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "deposits"
	}
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	methodFilter := strings.TrimSpace(r.URL.Query().Get("method"))
	walletIDStr := strings.TrimSpace(r.URL.Query().Get("wallet_id"))
	walletID, _ := strconv.ParseInt(walletIDStr, 10, 64)

	var (
		invoices             []*billing.AdminInvoiceView
		payments             []*billing.AdminPaymentView
		wallets              []*billing.AdminWalletView
		transactions         []*billing.AdminWalletTransactionView
		deposits             []*billing.AdminWalletDepositView
		totalInvoices        int
		totalPayments        int
		totalWallets         int
		totalTransactions    int
		totalDeposits        int
		pendingDepositsCount int
	)

	if h.billSvc != nil {
		deposits, totalDeposits, _ = h.billSvc.AdminListDetailedDeposits(ctx, billing.DepositFilter{
			Search:        searchQuery,
			Status:        statusFilter,
			PaymentMethod: methodFilter,
			WalletID:      walletID,
			Limit:         100,
		})
		_, pendingDepositsCount, _ = h.billSvc.AdminListDetailedDeposits(ctx, billing.DepositFilter{
			Status: "pending",
			Limit:  1,
		})
		invoices, totalInvoices, _ = h.billSvc.AdminListDetailedInvoices(ctx, billing.InvoiceFilter{
			Search: searchQuery,
			Status: statusFilter,
			Limit:  100,
		})
		payments, totalPayments, _ = h.billSvc.AdminListDetailedPayments(ctx, billing.PaymentFilter{
			Search: searchQuery,
			Status: statusFilter,
			Method: methodFilter,
			Limit:  100,
		})
		wallets, totalWallets, _ = h.billSvc.AdminListDetailedWallets(ctx, billing.WalletFilter{
			Search: searchQuery,
			Type:   typeFilter,
			Limit:  100,
		})
		transactions, totalTransactions, _ = h.billSvc.AdminListDetailedTransactions(ctx, billing.TransactionFilter{
			WalletID: walletID,
			Search:   searchQuery,
			Type:     typeFilter,
			Limit:    100,
		})
	}

	var totalRevenueMinor int64
	var totalPaidMinor int64
	for _, p := range payments {
		if p.Status == "paid" || p.Status == "completed" || p.Status == "success" {
			totalPaidMinor += p.Amount.Minor()
		}
	}
	for _, inv := range invoices {
		if inv.Status != "cancelled" {
			totalRevenueMinor += inv.TotalAmount.Minor()
		}
	}
	if totalRevenueMinor == 0 && totalPaidMinor > 0 {
		totalRevenueMinor = totalPaidMinor
	}

	totalPaid := money.FromMinor(totalPaidMinor)
	totalRevenue := money.FromMinor(totalRevenueMinor)
	commission := money.FromMinor(totalRevenueMinor * 5 / 100)

	var totalHeldMinor int64
	for _, w := range wallets {
		totalHeldMinor += w.Balance.Minor()
	}
	totalHeld := money.FromMinor(totalHeldMinor)

	data := pages.AdminFinanceData{
		ActiveTab:            tab,
		Invoices:             invoices,
		Payments:             payments,
		Wallets:              wallets,
		Transactions:         transactions,
		Deposits:             deposits,
		TotalInvoices:        totalInvoices,
		TotalPayments:        totalPayments,
		TotalWallets:         totalWallets,
		TotalTransactions:    totalTransactions,
		TotalDeposits:        totalDeposits,
		PendingDepositsCount: pendingDepositsCount,
		TotalRevenue:         totalRevenue,
		TotalCommission:      commission,
		TotalPaid:            totalPaid,
		TotalHeld:            totalHeld,
		Query:                searchQuery,
		StatusFilter:         statusFilter,
		TypeFilter:           typeFilter,
		MethodFilter:         methodFilter,
		SelectedWalletID:     walletID,
	}

	h.renderPage(ctx, w, "render admin finance page", pages.AdminFinance(data, lang, dir))
}

// AdminDepositApproveSubmit approves a pending deposit request and credits the user's wallet.
func (h *UIHandler) AdminDepositApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance?tab=deposits", http.StatusSeeOther)
		return
	}

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", "معرف طلب الإيداع غير صالح.")
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", "خدمة المدفوعات والمحفظة غير متاحة.")
		return
	}

	dep, tx, err := h.billSvc.AdminApproveDeposit(ctx, depositID, actor.UserID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to approve deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "success", fmt.Sprintf("تم اعتماد طلب الإيداع بنجاح وإضافة %s ج.م إلى محفظة المستخدم (معاملة #TX-%d).", dep.Amount.String(), tx.ID))
}

// AdminDepositRejectSubmit rejects a pending deposit request with an explicit reason.
func (h *UIHandler) AdminDepositRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance?tab=deposits", http.StatusSeeOther)
		return
	}

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", "معرف طلب الإيداع غير صالح.")
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("rejection_reason"))
	if reason == "" {
		reason = "تم رفض طلب الإيداع لعدم تطابق بيانات أو إشعار التحويل البنكي."
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", "خدمة المدفوعات والمحفظة غير متاحة.")
		return
	}

	if _, err := h.billSvc.AdminRejectDeposit(ctx, depositID, actor.UserID, reason); err != nil {
		h.log.ErrorContext(ctx, "failed to reject deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "success", "تم رفض طلب الإيداع وإشعار صاحب الحساب بحيثيات الرفض.")
}

// AdminWalletAdjustSubmit handles manual balance adjustment/credit/debit for a wallet.
func (h *UIHandler) AdminWalletAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	actorID := actor.UserID

	walletID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || walletID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance?tab=wallets", "error", "معرف المحفظة غير صحيح.")
		return
	}

	actionType := strings.TrimSpace(r.FormValue("action_type")) // "deposit", "withdrawal", "adjustment"
	amountStr := strings.TrimSpace(r.FormValue("amount"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	amt, parseErr := money.Parse(amountStr)
	if parseErr != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/admin/finance?tab=wallets", "error", "يرجى تحديد مبلغ صالح أكبر من الصفر.")
		return
	}
	if reason == "" {
		reason = "تعديل إداري مباشر للرصيد"
	}

	var txType billing.TransactionType

	switch actionType {
	case "deposit":
		txType = billing.TxDeposit
	case "withdrawal":
		txType = billing.TxWithdrawal
		amt = money.FromMinor(-amt.Minor())
	default:
		txType = billing.TxAdjustment
		if r.FormValue("is_deduct") == "true" {
			amt = money.FromMinor(-amt.Minor())
		}
	}

	if h.billSvc != nil {
		if err := h.billSvc.AdminPerformWalletAdjustment(ctx, walletID, amt, txType, reason, actorID); err != nil {
			h.redirectWithNotice(w, r, "/admin/finance?tab=wallets", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/finance?tab=transactions&wallet_id=%d", walletID), "success", "تم قيد وتحديث رصيد المحفظة بنجاح وتسجيل المعاملة في السجل.")
}

// AdminOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) AdminOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	orderID, _ := strconv.ParseInt(idStr, 10, 64)

	var order *commerce.Order
	if h.commSvc != nil && orderID > 0 {
		order, _ = h.commSvc.GetOrder(database.AsSystem(ctx), orderID)
	}

	if order == nil {
		http.Redirect(w, r, "/admin/orders?tab=offers", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render admin offer order detail", pages.AdminOfferOrderDetailPage(order, lang, dir))
}

// AdminPlansInfoPage renders subscription plan tiers directory.
func (h *UIHandler) AdminPlansInfoPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var plans []*billing.Plan
	if h.billSvc != nil {
		plans, _ = h.billSvc.ListPlans(ctx)
	}

	h.renderPage(ctx, w, "render admin plans info", pages.AdminPlansInfoPage(plans, lang, dir))
}

// AdminPlanTypesPage renders subscription plan types CRUD.
func (h *UIHandler) AdminPlanTypesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []pages.ReferenceItem
	if h.billSvc != nil {
		plans, _ := h.billSvc.ListPlans(ctx)
		for _, p := range plans {
			items = append(items, pages.ReferenceItem{
				ID:          p.ID,
				Name:        p.Name.Get(i18n.Lang(lang)),
				Description: p.Slug,
				Status:      "active",
				Extra:       p.Description.Get(i18n.Lang(lang)),
			})
		}
	}

	h.renderPage(ctx, w, "render admin plan types", pages.AdminReferenceCRUDPage("أنواع وتصنيفات خطط الاشتراك", "plan-types", "نوع خطة", items, "plans", lang, dir))
}

// AdminPlanFeaturesPage renders feature matrix for subscription plans.
func (h *UIHandler) AdminPlanFeaturesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []pages.ReferenceItem
	if h.billSvc != nil {
		plans, _ := h.billSvc.ListPlans(ctx)
		for _, p := range plans {
			for k, v := range p.Features {
				items = append(items, pages.ReferenceItem{
					ID:          p.ID,
					Name:        k,
					Description: p.Name.Get(i18n.Lang(lang)),
					Status:      "active",
					Extra:       v,
				})
			}
		}
	}

	h.renderPage(ctx, w, "render admin plan features", pages.AdminReferenceCRUDPage("ميزات ومحددات باقات الاشتراك", "plan-features", "ميزة", items, "plans", lang, dir))
}

// AdminPlansSubscriptionsPage renders active subscriptions and subscriber histories.
func (h *UIHandler) AdminPlansSubscriptionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var subs []*billing.Subscription
	if h.billSvc != nil {
		subs, _ = h.billSvc.AdminListSubscriptions(ctx, 100, 0)
	}

	h.renderPage(ctx, w, "render admin plan subscriptions", pages.AdminPlansSubscriptionsPage(subs, lang, dir))
}
