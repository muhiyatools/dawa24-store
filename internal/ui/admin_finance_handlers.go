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
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) loadAdminFinanceData(r *http.Request, tab string) (pages.AdminFinanceData, string, string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if tab == "" {
		tab = "wallets"
	}

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	methodFilter := strings.TrimSpace(r.URL.Query().Get("method"))
	walletIDStr := strings.TrimSpace(r.URL.Query().Get("wallet_id"))
	walletID, _ := strconv.ParseInt(walletIDStr, 10, 64)
	orgIDStr := strings.TrimSpace(r.URL.Query().Get("org_id"))
	orgID, _ := strconv.ParseInt(orgIDStr, 10, 64)

	var allOrgs []*org.Organization
	if h.orgSvc != nil {
		allOrgs, _ = h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 1000, 0)
	}

	// Auto-resolve organization ID if wallet_id is supplied without org_id
	if walletID > 0 && h.billSvc != nil {
		if wlt, err := h.billSvc.GetWalletByID(ctx, walletID); err == nil && wlt != nil {
			var wltOrgID int64
			if wlt.OrganizationID != nil && *wlt.OrganizationID > 0 {
				wltOrgID = *wlt.OrganizationID
			} else if wlt.UserID > 0 {
				for _, o := range allOrgs {
					if o.OwnerID == wlt.UserID {
						wltOrgID = o.ID
						break
					}
				}
			}

			if orgID == 0 && wltOrgID > 0 {
				orgID = wltOrgID
			} else if orgID > 0 && wltOrgID > 0 && orgID != wltOrgID {
				// The admin manually selected another organization filter from the dropdown; clear walletID so it doesn't conflict
				walletID = 0
			}
		}
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var stats *billing.AdminFinanceStats
	if h.billSvc != nil {
		stats, _ = h.billSvc.AdminGetFinanceStats(ctx)
	}
	if stats == nil {
		stats = &billing.AdminFinanceStats{}
	}

	var (
		invoices          []*billing.AdminInvoiceView
		payments          []*billing.AdminPaymentView
		wallets           []*billing.AdminWalletView
		transactions      []*billing.AdminWalletTransactionView
		deposits          []*billing.AdminWalletDepositView
		withdrawals       []*billing.AdminWalletWithdrawalView
		totalInvoices     = stats.TotalInvoices
		totalPayments     = stats.TotalPayments
		totalWallets      = stats.TotalWallets
		totalTransactions = stats.TotalTransactions
		totalDeposits     = stats.TotalDeposits
		totalWithdrawals  = stats.TotalWithdrawals
	)

	if h.billSvc != nil {
		switch tab {
		case "wallets":
			wallets, totalWallets, _ = h.billSvc.AdminListDetailedWallets(ctx, billing.WalletFilter{
				Search:         searchQuery,
				Type:           typeFilter,
				OrganizationID: orgID,
				Limit:          limit,
				Offset:         offset,
			})
		case "transactions":
			transactions, totalTransactions, _ = h.billSvc.AdminListDetailedTransactions(ctx, billing.TransactionFilter{
				WalletID:       walletID,
				OrganizationID: orgID,
				Search:         searchQuery,
				Type:           typeFilter,
				Limit:          limit,
				Offset:         offset,
			})
		case "deposits":
			deposits, totalDeposits, _ = h.billSvc.AdminListDetailedDeposits(ctx, billing.DepositFilter{
				Search:         searchQuery,
				Status:         statusFilter,
				PaymentMethod:  methodFilter,
				WalletID:       walletID,
				OrganizationID: orgID,
				Limit:          limit,
				Offset:         offset,
			})
		case "withdrawals":
			withdrawals, totalWithdrawals, _ = h.billSvc.AdminListDetailedWithdrawals(ctx, billing.WithdrawalFilter{
				Search:         searchQuery,
				Status:         statusFilter,
				WalletID:       walletID,
				OrganizationID: orgID,
				Limit:          limit,
				Offset:         offset,
			})
		case "invoices":
			var invOrgID *int64
			if orgID > 0 {
				invOrgID = &orgID
			}
			invoices, totalInvoices, _ = h.billSvc.AdminListDetailedInvoices(ctx, billing.InvoiceFilter{
				Search:         searchQuery,
				Status:         statusFilter,
				OrganizationID: invOrgID,
				Limit:          limit,
				Offset:         offset,
			})
		case "payments":
			var payOrgID *int64
			if orgID > 0 {
				payOrgID = &orgID
			}
			payments, totalPayments, _ = h.billSvc.AdminListDetailedPayments(ctx, billing.PaymentFilter{
				Search:         searchQuery,
				Status:         statusFilter,
				Method:         methodFilter,
				OrganizationID: payOrgID,
				Limit:          limit,
				Offset:         offset,
			})
		}
	}

	data := pages.AdminFinanceData{
		ActiveTab:               tab,
		Invoices:                invoices,
		Payments:                payments,
		Wallets:                 wallets,
		Transactions:            transactions,
		Deposits:                deposits,
		Withdrawals:             withdrawals,
		TotalInvoices:           totalInvoices,
		TotalPayments:           totalPayments,
		TotalWallets:            totalWallets,
		TotalTransactions:       totalTransactions,
		TotalDeposits:           totalDeposits,
		PendingDepositsCount:    stats.PendingDeposits,
		TotalWithdrawals:        totalWithdrawals,
		PendingWithdrawalsCount: stats.PendingWithdrawals,
		TotalRevenue:            stats.TotalRevenue,
		TotalPaid:               stats.TotalPaid,
		TotalHeld:               stats.TotalHeld,
		Query:                   searchQuery,
		StatusFilter:            statusFilter,
		TypeFilter:              typeFilter,
		MethodFilter:            methodFilter,
		SelectedWalletID:        walletID,
		Organizations:           allOrgs,
		SelectedOrgID:           orgID,
		Page:                    page,
		PerPage:                 limit,
	}

	return data, lang, dir
}

// AdminFinanceWalletsPage renders the standalone Wallets & Balances page.
func (h *UIHandler) AdminFinanceWalletsPage(w http.ResponseWriter, r *http.Request) {
	data, lang, dir := h.loadAdminFinanceData(r, "wallets")
	h.renderPage(r.Context(), w, "render admin finance wallets page", pages.AdminFinanceWalletsPage(data, lang, dir))
}

// AdminFinanceTransactionsPage renders the standalone Transactions Log page.
func (h *UIHandler) AdminFinanceTransactionsPage(w http.ResponseWriter, r *http.Request) {
	data, lang, dir := h.loadAdminFinanceData(r, "transactions")
	h.renderPage(r.Context(), w, "render admin finance transactions page", pages.AdminFinanceTransactionsPage(data, lang, dir))
}

// AdminFinanceDepositsPage renders the standalone Deposit Requests page.
func (h *UIHandler) AdminFinanceDepositsPage(w http.ResponseWriter, r *http.Request) {
	data, lang, dir := h.loadAdminFinanceData(r, "deposits")
	h.renderPage(r.Context(), w, "render admin finance deposits page", pages.AdminFinanceDepositsPage(data, lang, dir))
}

// AdminFinanceWithdrawalsPage renders the standalone Withdrawal Requests page.
func (h *UIHandler) AdminFinanceWithdrawalsPage(w http.ResponseWriter, r *http.Request) {
	data, lang, dir := h.loadAdminFinanceData(r, "withdrawals")
	h.renderPage(r.Context(), w, "render admin finance withdrawals page", pages.AdminFinanceWithdrawalsPage(data, lang, dir))
}

// AdminFinanceInvoicesPage renders the standalone Invoices page.
func (h *UIHandler) AdminFinanceInvoicesPage(w http.ResponseWriter, r *http.Request) {
	data, lang, dir := h.loadAdminFinanceData(r, "invoices")
	h.renderPage(r.Context(), w, "render admin finance invoices page", pages.AdminFinanceInvoicesPage(data, lang, dir))
}

// AdminFinancePaymentsPage renders the standalone Payments Log page.
func (h *UIHandler) AdminFinancePaymentsPage(w http.ResponseWriter, r *http.Request) {
	data, lang, dir := h.loadAdminFinanceData(r, "payments")
	h.renderPage(r.Context(), w, "render admin finance payments page", pages.AdminFinancePaymentsPage(data, lang, dir))
}

// AdminFinancePage serves the main finance hub (retained for backward compatibility).
func (h *UIHandler) AdminFinancePage(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	if tab == "earnings" {
		http.Redirect(w, r, "/admin/finance?tab=wallets", http.StatusMovedPermanently)
		return
	}
	if tab == "" {
		tab = "wallets"
	}
	data, lang, dir := h.loadAdminFinanceData(r, tab)
	h.renderPage(r.Context(), w, "render admin finance hub", pages.AdminFinance(data, lang, dir))
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
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.invalid_deposit_id"))
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	dep, tx, err := h.billSvc.AdminApproveDeposit(ctx, depositID, actor.UserID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to approve deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", h.safeMessage(err, lang))
		return
	}

	if dep != nil {
		var orgID int64
		if dep.OrganizationID != nil {
			orgID = *dep.OrganizationID
		}
		go h.notifyWalletDeposit(context.Background(), dep.UserID, orgID, dep.Amount, "approved")
	}

	h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "success", fmt.Sprintf(i18n.T(lang, "admin.finance.deposit_approved_success_format"), dep.Amount.String(), tx.ID))
}

// AdminDepositRejectSubmit rejects a pending deposit request with an explicit reason.
func (h *UIHandler) AdminDepositRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance/deposits?tab=deposits", http.StatusSeeOther)
		return
	}

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.invalid_deposit_id"))
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("rejection_reason"))
	if reason == "" {
		reason = i18n.T(lang, "admin.finance.default_deposit_rejection_reason")
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	dep, err := h.billSvc.AdminRejectDeposit(ctx, depositID, actor.UserID, reason)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to reject deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", h.safeMessage(err, lang))
		return
	}

	if dep != nil {
		var orgID int64
		if dep.OrganizationID != nil {
			orgID = *dep.OrganizationID
		}
		go h.notifyWalletDepositRejected(context.Background(), dep.UserID, orgID, dep.Amount, reason)
	}

	h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "success", i18n.T(lang, "admin.finance.deposit_rejected_success"))
}

// AdminWalletAdjustSubmit handles manual balance adjustment/credit/debit for a wallet.
func (h *UIHandler) AdminWalletAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor := authctx.FromContext(ctx)
	actorID := actor.UserID

	walletID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || walletID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/wallets?tab=wallets", "error", i18n.T(lang, "admin.finance.invalid_wallet_id"))
		return
	}

	actionType := strings.TrimSpace(r.FormValue("action_type")) // "deposit", "withdrawal", "adjustment"
	amountStr := strings.TrimSpace(r.FormValue("amount"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	amt, parseErr := money.Parse(amountStr)
	if parseErr != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/admin/finance/wallets?tab=wallets", "error", i18n.T(lang, "admin.finance.invalid_amount"))
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
			h.redirectWithNotice(w, r, "/admin/finance/wallets?tab=wallets", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/finance/transactions?tab=transactions&wallet_id=%d", walletID), "success", i18n.T(lang, "admin.finance.wallet_adjusted_success"))
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
