package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// WalletPage redirects to the unified settings wallet tab.
func (h *UIHandler) WalletPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings?tab=wallet", http.StatusMovedPermanently)
}

// WalletDepositSubmit handles submitting a funds deposit request, placing it in pending status for admin review.
func (h *UIHandler) WalletDepositSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings?tab=wallet", http.StatusSeeOther)
		return
	}

	_ = r.ParseMultipartForm(MaxUploadBytes)
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "يرجى إدخال مبلغ إيداع صالح وموجب.")
		return
	}

	method := strings.TrimSpace(r.PostFormValue("payment_method"))
	ref := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if method == "" {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "يرجى اختيار وسيلة الدفع أو التحويل.")
		return
	}
	if ref == "" {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "يرجى إدخال رقم الإشعار أو مرجع التحويل.")
		return
	}

	var attachmentURL string
	if file, _, err := r.FormFile("receipt"); err == nil && file != nil {
		_ = file.Close()
		if savedPath, err := saveUploadedFile(r, "receipt", "receipts"); err == nil {
			attachmentURL = savedPath
		}
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "خدمة المحفظة والفواتير غير متوفرة.")
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if _, err := h.billSvc.RequestDeposit(ctx, actor.UserID, orgPtr, "EGP", amt, method, ref, attachmentURL, notes); err != nil {
		h.log.ErrorContext(ctx, "failed to submit deposit request", "error", err)
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings?tab=wallet", "success", "تم تسجيل طلب شحن الرصيد بنجاح، والعملية قيد مراجعة وتدقيق الإدارة المالية.")
}

// WalletDepositEditSubmit handles updating a pending deposit request before admin review.
func (h *UIHandler) WalletDepositEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings?tab=wallet", http.StatusSeeOther)
		return
	}

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "معرف عملية الإيداع غير صالح.")
		return
	}

	_ = r.ParseMultipartForm(MaxUploadBytes)
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "يرجى إدخال مبلغ إيداع صالح.")
		return
	}

	method := strings.TrimSpace(r.PostFormValue("payment_method"))
	ref := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	var attachmentURL string
	if file, _, err := r.FormFile("receipt"); err == nil && file != nil {
		_ = file.Close()
		if savedPath, err := saveUploadedFile(r, "receipt", "receipts"); err == nil {
			attachmentURL = savedPath
		}
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "خدمة المحفظة والفواتير غير متوفرة.")
		return
	}

	if _, err := h.billSvc.EditPendingDeposit(ctx, actor.UserID, depositID, amt, method, ref, attachmentURL, notes); err != nil {
		h.log.ErrorContext(ctx, "failed to update pending deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings?tab=wallet", "success", "تم تحديث بيانات طلب شحن الرصيد بنجاح.")
}

// WalletWithdrawSubmit handles submitting a funds withdrawal request and debiting the wallet.
func (h *UIHandler) WalletWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings?tab=wallet", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "يرجى إدخال مبلغ سحب صالح.")
		return
	}

	dest := r.PostFormValue("destination_id")
	reason := r.PostFormValue("reason")
	desc := fmt.Sprintf("طلب سحب رصيد إلى: %s", dest)
	if reason != "" {
		desc += fmt.Sprintf(" (السبب: %s)", reason)
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", "خدمة المحفظة والفواتير غير متوفرة.")
		return
	}

	if _, err := h.billSvc.Withdraw(ctx, actor.UserID, "EGP", amt, "user_withdrawal", nil, desc); err != nil {
		h.log.ErrorContext(ctx, "failed wallet withdrawal", "error", err)
		h.redirectWithNotice(w, r, "/settings?tab=wallet", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings?tab=wallet", "success", "تم خصم وتسجيل طلب السحب بنجاح.")
}

// InvoicesPage renders the vendor's invoice list with status badges and actions.
func (h *UIHandler) InvoicesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/invoices", http.StatusSeeOther)
		return
	}

	// If pharmacy customer visits /invoices, redirect them to /orders since invoices are printed from order details
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/orders", "info", "يمكنك طباعة الفواتير الضريبية مباشرة من صفحة تفاصيل الطلب.")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	var detailedInvoices []*billing.AdminInvoiceView
	if h.billSvc != nil {
		invoices, _, err := h.billSvc.AdminListDetailedInvoices(ctx, billing.InvoiceFilter{
			Search: q,
			Status: status,
			Limit:  100,
		})
		if err != nil {
			h.log.WarnContext(ctx, "account: list detailed invoices", "error", err)
		} else {
			// Filter by vendor organization unless platform staff
			for _, inv := range invoices {
				if actor.IsStaff || inv.OrganizationID == actor.OrganizationID {
					detailedInvoices = append(detailedInvoices, inv)
				}
			}
		}
	}

	data := pages.InvoicesData{
		Invoices:     detailedInvoices,
		Search:       q,
		StatusFilter: status,
		IsVendor:     true,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.InvoicesPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render invoices page", "error", err)
	}
}

// InvoicePrintPage renders the Egyptian-standard printable invoice for a specific invoice ID.
func (h *UIHandler) InvoicePrintPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	var invoice *billing.Invoice
	if h.billSvc != nil {
		inv, errGet := h.billSvc.GetInvoice(ctx, id)
		if errGet == nil && inv != nil {
			invoice = inv
		}
	}

	if invoice == nil {
		http.NotFound(w, r)
		return
	}

	printableData, errBuild := h.buildPrintableInvoiceData(ctx, invoice, nil)
	if errBuild != nil {
		h.log.ErrorContext(ctx, "failed to build printable invoice data", "error", errBuild)
		http.Error(w, "تعذر تجهيز بيانات الفاتورة للطباعة", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.InvoicePrintablePage(*printableData, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render invoice print page", "error", err)
	}
}

// OrderInvoicePrintPage renders the Egyptian-standard printable invoice for a specific order ID.
func (h *UIHandler) OrderInvoicePrintPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	var order *commerce.Order
	if h.commSvc != nil {
		ord, errGet := h.commSvc.GetOrder(ctx, id)
		if errGet == nil && ord != nil {
			order = ord
		}
	}

	if order == nil {
		http.NotFound(w, r)
		return
	}

	if order.Status == commerce.StatusPending {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", "لا يمكن طباعة الفاتورة قبل قبول وتأكيد المورد للطلب.")
		return
	}

	printableData, errBuild := h.buildPrintableInvoiceData(ctx, nil, order)
	if errBuild != nil {
		h.log.ErrorContext(ctx, "failed to build printable invoice data for order", "error", errBuild)
		http.Error(w, "تعذر تجهيز بيانات الفاتورة للطباعة", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.InvoicePrintablePage(*printableData, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render order invoice print page", "error", err)
	}
}

func (h *UIHandler) buildPrintableInvoiceData(ctx context.Context, invoice *billing.Invoice, order *commerce.Order) (*billing.PrintableInvoiceData, error) {
	if invoice == nil && order == nil {
		return nil, fmt.Errorf("both invoice and order are nil")
	}

	if invoice == nil && order != nil && h.billSvc != nil {
		inv, err := h.billSvc.GetInvoiceByOrderID(ctx, order.ID)
		if err == nil && inv != nil {
			invoice = inv
		} else {
			vendorOrgID := int64(1)
			if len(order.Shipments) > 0 && order.Shipments[0].OrganizationID > 0 {
				vendorOrgID = order.Shipments[0].OrganizationID
			} else if order.VendorBranchID != nil {
				vendorOrgID = *order.VendorBranchID
			}
			newInv := &billing.Invoice{
				OrganizationID: vendorOrgID,
				CustomerOrgID:  order.OrganizationID,
				OrderID:        &order.ID,
				InvoiceNumber:  fmt.Sprintf("INV-%d-%05d", time.Now().Year(), order.ID),
				IssueDate:      order.CreatedAt,
				DueDate:        order.CreatedAt.AddDate(0, 0, 30),
				Subtotal:       order.Subtotal,
				TaxAmount:      order.TaxAmount,
				DiscountAmount: order.TotalDiscount,
				TotalAmount:    order.TotalAmount,
				Status:         billing.InvoicePaid,
				PaymentMethod:  string(order.PaymentMethod),
				Notes:          order.Notes,
			}
			if order.PaymentStatus != commerce.PaymentPaid {
				newInv.Status = billing.InvoiceIssued
			}
			for _, l := range order.Lines {
				unitPrice := l.UnitPrice
				newInv.Lines = append(newInv.Lines, billing.InvoiceLine{
					ProductID:   l.ProductID,
					Description: l.ProductName.Get("ar"),
					Quantity:    l.Quantity,
					UnitPrice:   unitPrice,
					TotalPrice:  l.TotalPrice,
				})
			}
			savedInv, errCreate := h.billSvc.CreateInvoice(ctx, newInv)
			if errCreate == nil && savedInv != nil {
				invoice = savedInv
			} else {
				invoice = newInv
			}
		}
	}

	if order == nil && invoice != nil && invoice.OrderID != nil && h.commSvc != nil {
		ord, _ := h.commSvc.GetOrder(ctx, *invoice.OrderID)
		if ord != nil {
			order = ord
		}
	}

	// Fetch Vendor Info
	vendorInfo := billing.PrintableOrgInfo{
		DisplayName:        "المورد المعتمد - دواء 24",
		LegalName:          "شركة توزيع الأدوية والمستلزمات الطبية",
		TaxNumber:          "100-245-890",
		CommercialRegister: "108920",
		Phone:              "0100002424",
		Address:            "المنطقة الصناعية - مخازن الأدوية",
		City:               "القاهرة",
	}
	if invoice != nil && invoice.OrganizationID > 0 && h.orgSvc != nil {
		vOrg, _ := h.orgSvc.GetOrganization(ctx, invoice.OrganizationID)
		if vOrg != nil {
			vendorInfo.OrganizationID = vOrg.ID
			if vOrg.LegalName != "" {
				vendorInfo.LegalName = vOrg.LegalName
				vendorInfo.DisplayName = vOrg.LegalName
			}
			if vOrg.TradeName.Get("ar") != "" {
				vendorInfo.DisplayName = vOrg.TradeName.Get("ar")
			}
			if vOrg.TaxNumber != "" {
				vendorInfo.TaxNumber = vOrg.TaxNumber
			}
			if vOrg.CommercialRegister != "" {
				vendorInfo.CommercialRegister = vOrg.CommercialRegister
			}
		}
	}

	// Fetch Customer Info
	custInfo := billing.PrintableOrgInfo{
		DisplayName:        "صيدلية معتمدة",
		LegalName:          "صيدلية معتمدة",
		TaxNumber:          "400-123-789",
		CommercialRegister: "98201",
		PharmacistLicense:  "PH-2026/884",
		Phone:              "0110002424",
		Address:            "جمهورية مصر العربية",
		City:               "مصر",
	}
	var custOrgID *int64
	if invoice != nil && invoice.CustomerOrgID != nil {
		custOrgID = invoice.CustomerOrgID
	} else if order != nil && order.OrganizationID != nil {
		custOrgID = order.OrganizationID
	}
	if custOrgID != nil && *custOrgID > 0 && h.orgSvc != nil {
		cOrg, _ := h.orgSvc.GetOrganization(ctx, *custOrgID)
		if cOrg != nil {
			custInfo.OrganizationID = cOrg.ID
			if cOrg.LegalName != "" {
				custInfo.LegalName = cOrg.LegalName
				custInfo.DisplayName = cOrg.LegalName
			}
			if cOrg.TradeName.Get("ar") != "" {
				custInfo.DisplayName = cOrg.TradeName.Get("ar")
			}
			if cOrg.TaxNumber != "" {
				custInfo.TaxNumber = cOrg.TaxNumber
			}
			if cOrg.CommercialRegister != "" {
				custInfo.CommercialRegister = cOrg.CommercialRegister
			}
			if cOrg.PharmacistLicense != "" {
				custInfo.PharmacistLicense = cOrg.PharmacistLicense
			}
		}
	}

	if order != nil {
		if len(order.Shipments) > 0 {
			sh := order.Shipments[0]
			if sh.CustomerBranchName.Get("ar") != "" {
				custInfo.DisplayName = sh.CustomerBranchName.Get("ar")
			}
			if sh.CustomerBranchAddress != "" {
				custInfo.Address = sh.CustomerBranchAddress
			}
			if sh.CustomerBranchPhone != "" {
				custInfo.Phone = sh.CustomerBranchPhone
			}
		}
	}

	// Build Lines
	var printableLines []*billing.PrintableInvoiceLine
	if invoice != nil && len(invoice.Lines) > 0 {
		for idx, l := range invoice.Lines {
			discPct := 0.0
			unitPrice := l.UnitPrice
			netPrice := l.UnitPrice
			if l.UnitPrice.IsPositive() && l.Quantity > 0 {
				computedLinePrice := money.FromMinor(l.UnitPrice.Minor() * int64(l.Quantity))
				if computedLinePrice.Minor() > l.TotalPrice.Minor() {
					diff := computedLinePrice.Minor() - l.TotalPrice.Minor()
					discPct = float64(diff) / float64(computedLinePrice.Minor()) * 100.0
					netPrice = money.FromMinor(l.TotalPrice.Minor() / int64(l.Quantity))
				}
			}
			printableLines = append(printableLines, &billing.PrintableInvoiceLine{
				Index:           idx + 1,
				ProductID:       l.ProductID,
				ItemName:        l.Description,
				Quantity:        l.Quantity,
				UnitPrice:       unitPrice,
				DiscountPercent: discPct,
				NetUnitPrice:    netPrice,
				TotalPrice:      l.TotalPrice,
				IsExempt:        true,
			})
		}
	} else if order != nil && len(order.Lines) > 0 {
		for idx, l := range order.Lines {
			discPct := 0.0
			unitPrice := l.UnitPrice
			netPrice := l.UnitPrice
			if l.UnitPrice.IsPositive() && l.Quantity > 0 {
				computedLinePrice := money.FromMinor(l.UnitPrice.Minor() * int64(l.Quantity))
				if computedLinePrice.Minor() > l.TotalPrice.Minor() {
					diff := computedLinePrice.Minor() - l.TotalPrice.Minor()
					discPct = float64(diff) / float64(computedLinePrice.Minor()) * 100.0
					netPrice = money.FromMinor(l.TotalPrice.Minor() / int64(l.Quantity))
				}
			}
			printableLines = append(printableLines, &billing.PrintableInvoiceLine{
				Index:           idx + 1,
				ProductID:       l.ProductID,
				ItemName:        l.ProductName.Get("ar"),
				Quantity:        l.Quantity,
				UnitPrice:       unitPrice,
				DiscountPercent: discPct,
				NetUnitPrice:    netPrice,
				TotalPrice:      l.TotalPrice,
				IsExempt:        true,
			})
		}
	}

	invNumber := "INV-2026-00001"
	var issueDate, dueDate time.Time
	var subtotal, totalDiscount, totalTax, totalAmount money.Amount
	invStatus := billing.InvoicePaid
	paymentMethod := "credit"
	paymentStatus := "paid"
	notes := ""
	var orderNum string

	if invoice != nil {
		invNumber = invoice.InvoiceNumber
		issueDate = invoice.IssueDate
		dueDate = invoice.DueDate
		subtotal = invoice.Subtotal
		totalDiscount = invoice.DiscountAmount
		totalTax = invoice.TaxAmount
		totalAmount = invoice.TotalAmount
		invStatus = invoice.Status
		paymentMethod = invoice.PaymentMethod
		notes = invoice.Notes
	}

	if order != nil {
		orderNum = order.OrderNumber
		if issueDate.IsZero() {
			issueDate = order.CreatedAt
		}
		if subtotal.IsZero() {
			subtotal = order.Subtotal
		}
		if totalDiscount.IsZero() {
			totalDiscount = order.TotalDiscount
		}
		if totalAmount.IsZero() {
			totalAmount = order.TotalAmount
		}
		paymentMethod = string(order.PaymentMethod)
		paymentStatus = string(order.PaymentStatus)
		if notes == "" {
			notes = order.Notes
		}
	}

	if issueDate.IsZero() {
		issueDate = time.Now().UTC()
	}
	if dueDate.IsZero() {
		dueDate = issueDate.AddDate(0, 0, 30)
	}

	qrData := url.QueryEscape(fmt.Sprintf("Dawa24|Seller:%s|TaxID:%s|Invoice:%s|Date:%s|Total:%s|VAT:0.00",
		vendorInfo.DisplayName, vendorInfo.TaxNumber, invNumber, issueDate.Format("2006-01-02T15:04:05Z"), totalAmount.String()))

	return &billing.PrintableInvoiceData{
		InvoiceID:      0,
		InvoiceNumber:  invNumber,
		OrderNumber:    orderNum,
		IssueDate:      issueDate,
		DueDate:        dueDate,
		Vendor:         vendorInfo,
		Customer:       custInfo,
		Lines:          printableLines,
		Subtotal:       subtotal,
		TotalDiscount:  totalDiscount,
		TaxableAmount:  totalAmount,
		VATAmtExempt:   money.Zero,
		VATAmtStandard: money.Zero,
		TotalTax:       totalTax,
		TotalAmount:    totalAmount,
		Status:         invStatus,
		PaymentMethod:  paymentMethod,
		PaymentStatus:  paymentStatus,
		Notes:          notes,
		QRCodeData:     qrData,
	}, nil
}

// FavoritesPage lists the signed-in user's favourited products.
func (h *UIHandler) FavoritesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	var products []*catalog.Product
	if h.idSvc != nil && h.catSvc != nil {
		ids, err := h.idSvc.ListFavorites(ctx, actor.UserID)
		if err != nil {
			h.log.WarnContext(ctx, "favorites: list favorites", "error", err)
		} else {
			for _, id := range ids {
				if p, _, err := h.catSvc.GetProduct(ctx, id); err != nil {
					h.log.DebugContext(ctx, "favorites: get product optional", "id", id, "error", err)
				} else if p != nil {
					products = append(products, p)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.FavoritesPage(lang, dir, products).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render favorites page", "error", err)
	}
}

// FavoriteRemoveSubmit removes a product from the user's favourites.
func (h *UIHandler) FavoriteRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	if h.idSvc != nil {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			h.log.DebugContext(ctx, "favorite remove: invalid id", "error", err)
		} else {
			if err := h.idSvc.RemoveFavorite(ctx, actor.UserID, id); err != nil {
				h.log.WarnContext(ctx, "favorite remove: failed", "id", id, "error", err)
			}
		}
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// FavoriteAddSubmit adds a product to the user's favourites.
func (h *UIHandler) FavoriteAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	productID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if productID <= 0 {
		productID, _ = strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	}

	if h.idSvc != nil && productID > 0 {
		if err := h.idSvc.AddFavorite(ctx, actor.UserID, productID); err != nil {
			h.log.WarnContext(ctx, "favorite add: failed", "product_id", productID, "error", err)
		}
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// FavoriteToggleSubmit toggles a product in the user's favourites.
func (h *UIHandler) FavoriteToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/favorites", http.StatusSeeOther)
		return
	}

	productID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if productID <= 0 {
		productID, _ = strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	}

	if h.idSvc != nil && productID > 0 {
		favs, err := h.idSvc.ListFavorites(ctx, actor.UserID)
		isFav := false
		if err != nil {
			h.log.WarnContext(ctx, "favorite toggle: list favorites", "error", err)
		} else {
			for _, id := range favs {
				if id == productID {
					isFav = true
					break
				}
			}
		}
		if isFav {
			_ = h.idSvc.RemoveFavorite(ctx, actor.UserID, productID)
		} else {
			_ = h.idSvc.AddFavorite(ctx, actor.UserID, productID)
		}
	}

	redirect := r.Header.Get("Referer")
	if redirect == "" {
		redirect = "/favorites"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
