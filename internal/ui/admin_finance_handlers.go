package ui

import (
	"context"
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
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
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

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

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
		depLimit, depOffset := 1, 0
		if tab == "deposits" {
			depLimit, depOffset = limit, offset
		}
		deposits, totalDeposits, _ = h.billSvc.AdminListDetailedDeposits(ctx, billing.DepositFilter{
			Search:        searchQuery,
			Status:        statusFilter,
			PaymentMethod: methodFilter,
			WalletID:      walletID,
			Limit:         depLimit,
			Offset:        depOffset,
		})
		_, pendingDepositsCount, _ = h.billSvc.AdminListDetailedDeposits(ctx, billing.DepositFilter{
			Status: "pending",
			Limit:  1,
		})

		invLimit, invOffset := 1, 0
		if tab == "invoices" {
			invLimit, invOffset = limit, offset
		}
		invoices, totalInvoices, _ = h.billSvc.AdminListDetailedInvoices(ctx, billing.InvoiceFilter{
			Search: searchQuery,
			Status: statusFilter,
			Limit:  invLimit,
			Offset: invOffset,
		})

		payLimit, payOffset := 1, 0
		if tab == "payments" {
			payLimit, payOffset = limit, offset
		}
		payments, totalPayments, _ = h.billSvc.AdminListDetailedPayments(ctx, billing.PaymentFilter{
			Search: searchQuery,
			Status: statusFilter,
			Method: methodFilter,
			Limit:  payLimit,
			Offset: payOffset,
		})

		walLimit, walOffset := 1, 0
		if tab == "wallets" {
			walLimit, walOffset = limit, offset
		}
		wallets, totalWallets, _ = h.billSvc.AdminListDetailedWallets(ctx, billing.WalletFilter{
			Search: searchQuery,
			Type:   typeFilter,
			Limit:  walLimit,
			Offset: walOffset,
		})

		txLimit, txOffset := 1, 0
		if tab == "transactions" {
			txLimit, txOffset = limit, offset
		}
		transactions, totalTransactions, _ = h.billSvc.AdminListDetailedTransactions(ctx, billing.TransactionFilter{
			WalletID: walletID,
			Search:   searchQuery,
			Type:     typeFilter,
			Limit:    txLimit,
			Offset:   txOffset,
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
		Page:                 page,
		PerPage:              limit,
	}

	h.renderPage(ctx, w, "render admin finance page", pages.AdminFinance(data, lang, dir))
}

// AdminDepositApproveSubmit approves a pending deposit request and credits the user's wallet.
func (h *UIHandler) AdminDepositApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance?tab=deposits", http.StatusSeeOther)
		return
	}

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", i18n.T(lang, "admin.finance.invalid_deposit_id"))
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	dep, tx, err := h.billSvc.AdminApproveDeposit(ctx, depositID, actor.UserID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to approve deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", h.safeMessage(err, lang))
		return
	}

	if dep != nil {
		var orgID int64
		if dep.OrganizationID != nil {
			orgID = *dep.OrganizationID
		}
		go h.notifyWalletDeposit(context.Background(), dep.UserID, orgID, dep.Amount, "approved")
	}

	h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "success", fmt.Sprintf(i18n.T(lang, "admin.finance.deposit_approved_success_format"), dep.Amount.String(), tx.ID))
}

// AdminDepositRejectSubmit rejects a pending deposit request with an explicit reason.
func (h *UIHandler) AdminDepositRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance?tab=deposits", http.StatusSeeOther)
		return
	}

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", i18n.T(lang, "admin.finance.invalid_deposit_id"))
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("rejection_reason"))
	if reason == "" {
		reason = i18n.T(lang, "admin.finance.default_deposit_rejection_reason")
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	dep, err := h.billSvc.AdminRejectDeposit(ctx, depositID, actor.UserID, reason)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to reject deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "error", h.safeMessage(err, lang))
		return
	}

	if dep != nil {
		var orgID int64
		if dep.OrganizationID != nil {
			orgID = *dep.OrganizationID
		}
		go h.notifyWalletDepositRejected(context.Background(), dep.UserID, orgID, dep.Amount, reason)
	}

	h.redirectWithNotice(w, r, "/admin/finance?tab=deposits", "success", i18n.T(lang, "admin.finance.deposit_rejected_success"))
}

// AdminWalletAdjustSubmit handles manual balance adjustment/credit/debit for a wallet.
func (h *UIHandler) AdminWalletAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor := authctx.FromContext(ctx)
	actorID := actor.UserID

	walletID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || walletID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance?tab=wallets", "error", i18n.T(lang, "admin.finance.invalid_wallet_id"))
		return
	}

	actionType := strings.TrimSpace(r.FormValue("action_type")) // "deposit", "withdrawal", "adjustment"
	amountStr := strings.TrimSpace(r.FormValue("amount"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	amt, parseErr := money.Parse(amountStr)
	if parseErr != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/admin/finance?tab=wallets", "error", i18n.T(lang, "admin.finance.invalid_amount"))
		return
	}
	if reason == "" {
		reason = i18n.T(lang, "admin.finance.default_adjustment_reason")
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
			h.redirectWithNotice(w, r, "/admin/finance?tab=wallets", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/finance?tab=transactions&wallet_id=%d", walletID), "success", i18n.T(lang, "admin.finance.wallet_adjusted_success"))
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

