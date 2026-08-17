package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// WalletData is the /wallet view model: balance, ledger and payment methods.
type WalletData struct {
	Balance        money.Amount
	Currency       string
	Transactions   []*billing.WalletTransaction
	PaymentMethods []*billing.UserPaymentMethod
}

// InvoicesData is the /invoices view model.
type InvoicesData struct {
	Invoices []*billing.Invoice
}

// TxLabel maps a wallet transaction type onto an Arabic label.
func TxLabel(t billing.TransactionType) string {
	switch t {
	case billing.TxDeposit:
		return "إيداع"
	case billing.TxWithdrawal:
		return "سحب"
	case billing.TxPurchase:
		return "شراء"
	case billing.TxRefund:
		return "استرداد"
	case billing.TxBonus:
		return "مكافأة"
	case billing.TxPenalty:
		return "غرامة"
	case billing.TxTransferIn:
		return "تحويل وارد"
	case billing.TxTransferOut:
		return "تحويل صادر"
	case billing.TxAdjustment:
		return "تسوية"
	default:
		return string(t)
	}
}

// InvoiceLabel maps an invoice status onto an Arabic label.
func InvoiceLabel(s billing.InvoiceStatus) string {
	switch s {
	case billing.InvoiceDraft:
		return "مسودة"
	case billing.InvoiceIssued:
		return "صادرة"
	case billing.InvoicePaid:
		return "مدفوعة"
	case billing.InvoiceOverdue:
		return "متأخرة"
	case billing.InvoiceCancelled:
		return "ملغاة"
	default:
		return string(s)
	}
}

// InvoiceBadgeClass maps an invoice status onto a badge colour class.
func InvoiceBadgeClass(s billing.InvoiceStatus) string {
	switch s {
	case billing.InvoicePaid:
		return "badge-emerald"
	case billing.InvoiceIssued:
		return "badge-amber"
	case billing.InvoiceOverdue:
		return "badge-rose"
	case billing.InvoiceCancelled:
		return "badge-rose"
	default:
		return "badge-amber"
	}
}

// msgStatusLabel maps a contact-message status onto an Arabic label.
func msgStatusLabel(s string) string {
	switch s {
	case "resolved":
		return "تم الرد"
	case "read":
		return "مقروءة"
	default:
		return "جديدة"
	}
}

// msgStatusBadge maps a contact-message status onto a badge colour class.
func msgStatusBadge(s string) string {
	switch s {
	case "resolved":
		return "badge-emerald"
	case "read":
		return "badge-amber"
	default:
		return "badge-rose"
	}
}
