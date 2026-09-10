package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// buildAdminInvoiceView transforms a raw billing.Invoice into an enriched view for payment display.
func (h *UIHandler) buildInvoiceViewForPayment(ctx context.Context, inv *billing.Invoice) *billing.AdminInvoiceView {
	view := &billing.AdminInvoiceView{
		ID:              inv.ID,
		PublicID:        inv.PublicID,
		OrganizationID:  inv.OrganizationID,
		CustomerOrgID:   inv.CustomerOrgID,
		OrderID:         inv.OrderID,
		InvoiceNumber:   inv.InvoiceNumber,
		IssueDate:       inv.IssueDate,
		DueDate:         inv.DueDate,
		Subtotal:        inv.Subtotal,
		TaxAmount:       inv.TaxAmount,
		DiscountAmount:  inv.DiscountAmount,
		TotalAmount:     inv.TotalAmount,
		PaidAmount:      inv.PaidAmount,
		RemainingAmount: inv.RemainingAmount,
		Status:          inv.Status,
		PaymentMethod:   inv.PaymentMethod,
		Notes:           inv.Notes,
		CreatedAt:       inv.CreatedAt,
	}

	if h.orgSvc != nil {
		if vOrg, err := h.orgSvc.GetOrganization(ctx, inv.OrganizationID); err == nil && vOrg != nil {
			view.VendorName = vOrg.LegalName
			if view.VendorName == "" {
				view.VendorName = vOrg.TradeName.Get("ar")
			}
		}
		if inv.CustomerOrgID != nil && *inv.CustomerOrgID > 0 {
			if cOrg, err := h.orgSvc.GetOrganization(ctx, *inv.CustomerOrgID); err == nil && cOrg != nil {
				view.CustomerName = cOrg.LegalName
				if view.CustomerName == "" {
					view.CustomerName = cOrg.TradeName.Get("ar")
				}
			}
		}
	}

	if inv.OrderID != nil && *inv.OrderID > 0 && h.commSvc != nil {
		if ord, err := h.commSvc.GetOrder(ctx, *inv.OrderID); err == nil && ord != nil {
			view.OrderNumber = ord.OrderNumber
		}
	}

	return view
}

// InvoicePaymentPage renders the payment recording screen for a vendor invoice.
func (h *UIHandler) InvoicePaymentPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/invoices", http.StatusSeeOther)
		return
	}

	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/orders", "error", "غير مصرح لحساب الصيدلية بإدارة وتسجيل مدفوعات الفواتير")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/invoices", "error", "معرف الفاتورة غير صحيح")
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/invoices", "error", "خدمة الفواتير غير متاحة حالياً")
		return
	}

	inv, err := h.billSvc.GetInvoiceByID(ctx, id)
	if err != nil || inv == nil {
		h.redirectWithNotice(w, r, "/invoices", "error", "لم يتم العثور على الفاتورة المطلوبة")
		return
	}

	if !actor.IsStaff && actor.OrganizationID > 0 && inv.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/invoices", "error", "ليس لديك صلاحية لتسجيل دفعات على فواتير لا تخص منشأتك")
		return
	}

	view := h.buildInvoiceViewForPayment(ctx, inv)

	var payments []*billing.AdminPaymentView
	if pList, _, pErr := h.billSvc.AdminListDetailedPayments(ctx, billing.PaymentFilter{InvoiceID: &id}); pErr == nil {
		payments = pList
	}

	data := pages.InvoicePaymentPageData{
		Invoice:   view,
		Payments:  payments,
		IsAdmin:   false,
		BackURL:   "/invoices",
		ActionURL: fmt.Sprintf("/invoices/%d/payment", id),
	}

	h.renderPage(ctx, w, "render invoice payment page", pages.InvoicePaymentPage(data, lang, dir))
}

// InvoicePaymentSubmit records a payment against a vendor invoice.
func (h *UIHandler) InvoicePaymentSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/invoices", http.StatusSeeOther)
		return
	}

	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/orders", "error", "غير مصرح لحساب الصيدلية بتسجيل مدفوعات الفواتير")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/invoices", "error", "معرف الفاتورة غير صحيح")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/invoices/%d/payment", id), "error", "تعذر قراءة بيانات النموذج")
		return
	}

	inv, err := h.billSvc.GetInvoiceByID(ctx, id)
	if err != nil || inv == nil {
		h.redirectWithNotice(w, r, "/invoices", "error", "لم يتم العثور على الفاتورة")
		return
	}

	if !actor.IsStaff && actor.OrganizationID > 0 && inv.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/invoices", "error", "غير مصرح لك بتسجيل دفعات على هذه الفاتورة")
		return
	}

	amountStr := strings.TrimSpace(r.PostFormValue("amount"))
	amount, err := money.Parse(amountStr)
	if err != nil || amount.Minor() <= 0 {
		h.redirectWithNotice(w, r, fmt.Sprintf("/invoices/%d/payment", id), "error", "يرجى إدخال مبلغ صحيح للدفعة أكبر من صفر")
		return
	}

	if amount.Minor() > inv.RemainingAmount.Minor() {
		h.redirectWithNotice(w, r, fmt.Sprintf("/invoices/%d/payment", id), "error", fmt.Sprintf("مبلغ الدفعة (%s) يتجاوز المتبقي على الفاتورة (%s)", amount.String(), inv.RemainingAmount.String()))
		return
	}

	method := strings.TrimSpace(r.PostFormValue("method"))
	refNum := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	var paidAt *time.Time
	if paidAtStr := strings.TrimSpace(r.PostFormValue("paid_at")); paidAtStr != "" {
		if t, err := time.Parse("2006-01-02", paidAtStr); err == nil {
			paidAt = &t
		}
	}

	req := billing.RecordInvoicePaymentRequest{
		InvoiceID:       inv.ID,
		OrganizationID:  inv.OrganizationID,
		UserID:          actor.UserID,
		Amount:          amount,
		Method:          method,
		ReferenceNumber: refNum,
		Notes:           notes,
		PaidAt:          paidAt,
	}

	if _, payErr := h.billSvc.RecordInvoicePayment(ctx, req); payErr != nil {
		h.log.ErrorContext(ctx, "record invoice payment failed", "invoice_id", id, "error", payErr)
		h.redirectWithNotice(w, r, fmt.Sprintf("/invoices/%d/payment", id), "error", h.safeMessage(payErr, lang))
		return
	}

	h.redirectWithNotice(w, r, "/invoices", "success", fmt.Sprintf("تم قيد دفعة بمبلغ %s ج.م بنجاح للفاتورة رقم %s", amount.String(), inv.InvoiceNumber))
}

// AdminInvoicePaymentPage renders the payment recording screen for admin finance.
func (h *UIHandler) AdminInvoicePaymentPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsStaff {
		http.Redirect(w, r, "/admin/login?redirect=/admin/finance/invoices", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/invoices", "error", "معرف الفاتورة غير صحيح")
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/invoices", "error", "خدمة الفواتير غير متاحة حالياً")
		return
	}

	inv, err := h.billSvc.GetInvoiceByID(ctx, id)
	if err != nil || inv == nil {
		h.redirectWithNotice(w, r, "/admin/finance/invoices", "error", "لم يتم العثور على الفاتورة المطلوبة")
		return
	}

	view := h.buildInvoiceViewForPayment(ctx, inv)

	var payments []*billing.AdminPaymentView
	if pList, _, pErr := h.billSvc.AdminListDetailedPayments(ctx, billing.PaymentFilter{InvoiceID: &id}); pErr == nil {
		payments = pList
	}

	data := pages.InvoicePaymentPageData{
		Invoice:   view,
		Payments:  payments,
		IsAdmin:   true,
		BackURL:   "/admin/finance/invoices",
		ActionURL: fmt.Sprintf("/admin/finance/invoices/%d/payment", id),
	}

	h.renderPage(ctx, w, "render admin invoice payment page", pages.InvoicePaymentPage(data, lang, dir))
}

// AdminInvoicePaymentSubmit records a payment against an invoice via admin finance.
func (h *UIHandler) AdminInvoicePaymentSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsStaff {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/invoices", "error", "معرف الفاتورة غير صحيح")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/finance/invoices/%d/payment", id), "error", "تعذر قراءة بيانات النموذج")
		return
	}

	inv, err := h.billSvc.GetInvoiceByID(ctx, id)
	if err != nil || inv == nil {
		h.redirectWithNotice(w, r, "/admin/finance/invoices", "error", "لم يتم العثور على الفاتورة")
		return
	}

	amountStr := strings.TrimSpace(r.PostFormValue("amount"))
	amount, err := money.Parse(amountStr)
	if err != nil || amount.Minor() <= 0 {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/finance/invoices/%d/payment", id), "error", "يرجى إدخال مبلغ صحيح للدفعة أكبر من صفر")
		return
	}

	if amount.Minor() > inv.RemainingAmount.Minor() {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/finance/invoices/%d/payment", id), "error", fmt.Sprintf("مبلغ الدفعة (%s) يتجاوز المتبقي على الفاتورة (%s)", amount.String(), inv.RemainingAmount.String()))
		return
	}

	method := strings.TrimSpace(r.PostFormValue("method"))
	refNum := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	var paidAt *time.Time
	if paidAtStr := strings.TrimSpace(r.PostFormValue("paid_at")); paidAtStr != "" {
		if t, err := time.Parse("2006-01-02", paidAtStr); err == nil {
			paidAt = &t
		}
	}

	req := billing.RecordInvoicePaymentRequest{
		InvoiceID:       inv.ID,
		OrganizationID:  inv.OrganizationID,
		UserID:          actor.UserID,
		Amount:          amount,
		Method:          method,
		ReferenceNumber: refNum,
		Notes:           notes,
		PaidAt:          paidAt,
	}

	if _, payErr := h.billSvc.RecordInvoicePayment(ctx, req); payErr != nil {
		h.log.ErrorContext(ctx, "admin record invoice payment failed", "invoice_id", id, "error", payErr)
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/finance/invoices/%d/payment", id), "error", h.safeMessage(payErr, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/finance/invoices", "success", fmt.Sprintf("تم قيد دفعة إدارية بمبلغ %s ج.م بنجاح للفاتورة رقم %s", amount.String(), inv.InvoiceNumber))
}
