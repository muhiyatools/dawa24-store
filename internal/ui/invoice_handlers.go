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
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

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
		h.redirectWithNotice(w, r, "/orders", "info", i18n.T(lang, "invoice.customer_redirect_info"))
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

	h.renderPage(ctx, w, "render invoices page", pages.InvoicesPage(lang, dir, data))
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

	printableData, errBuild := h.buildPrintableInvoiceData(ctx, invoice, nil, lang)
	if errBuild != nil {
		h.log.ErrorContext(ctx, "failed to build printable invoice data", "error", errBuild)
		http.Error(w, i18n.T(lang, "invoice.prepare_failed"), http.StatusInternalServerError)
		return
	}

	h.renderPage(ctx, w, "render invoice print page", pages.InvoicePrintablePage(*printableData, lang, dir))
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
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", i18n.T(lang, "invoice.print_pending_error"))
		return
	}

	printableData, errBuild := h.buildPrintableInvoiceData(ctx, nil, order, lang)
	if errBuild != nil {
		h.log.ErrorContext(ctx, "failed to build printable invoice data for order", "error", errBuild)
		http.Error(w, i18n.T(lang, "invoice.prepare_failed"), http.StatusInternalServerError)
		return
	}

	h.renderPage(ctx, w, "render order invoice print page", pages.InvoicePrintablePage(*printableData, lang, dir))
}

func (h *UIHandler) buildPrintableInvoiceData(ctx context.Context, invoice *billing.Invoice, order *commerce.Order, lang any) (*billing.PrintableInvoiceData, error) {
	if invoice == nil && order == nil {
		return nil, fmt.Errorf("both invoice and order are nil")
	}

	if invoice == nil && order != nil && h.billSvc != nil {
		if inv, err := h.billSvc.GetInvoiceByOrderID(ctx, order.ID); err == nil && inv != nil {
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
		DisplayName:        i18n.T(lang, "invoice.default_vendor_display_name"),
		LegalName:          i18n.T(lang, "invoice.default_vendor_legal_name"),
		TaxNumber:          "100-245-890",
		CommercialRegister: "108920",
		Phone:              "0100002424",
		Address:            i18n.T(lang, "invoice.default_vendor_address"),
		City:               i18n.T(lang, "invoice.default_vendor_city"),
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
		if supp, err := h.orgSvc.GetSupplierProfile(ctx, invoice.OrganizationID); err == nil && supp != nil && supp.Image != "" {
			vendorInfo.LogoURL = supp.Image
		}
	}

	// Fetch Customer Info
	custInfo := billing.PrintableOrgInfo{
		DisplayName:        i18n.T(lang, "invoice.default_customer_display_name"),
		LegalName:          i18n.T(lang, "invoice.default_customer_legal_name"),
		TaxNumber:          "400-123-789",
		CommercialRegister: "98201",
		PharmacistLicense:  "PH-2026/884",
		Phone:              "0110002424",
		Address:            i18n.T(lang, "invoice.default_customer_address"),
		City:               i18n.T(lang, "invoice.default_customer_city"),
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
			if cOrg.OrganizationNumber != "" {
				custInfo.OrganizationNumber = cOrg.OrganizationNumber
			}
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

		// Also check user_organizations link for this customer with the vendor
		if invoice != nil && invoice.OrganizationID > 0 {
			if links, err := h.orgSvc.ListUserOrganizationsByVendor(ctx, invoice.OrganizationID, "approved"); err == nil {
				for _, link := range links {
					if link != nil && link.CustomerOrgID != nil && *link.CustomerOrgID == *custOrgID && link.OrganizationNumber != "" {
						custInfo.OrganizationNumber = link.OrganizationNumber
						break
					}
				}
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

	var deliveryCode, trackingNumber string
	if order != nil {
		for _, s := range order.Shipments {
			if s != nil {
				if deliveryCode == "" && s.DeliveryCode != "" {
					deliveryCode = s.DeliveryCode
				}
				if trackingNumber == "" && s.TrackingNumber != "" {
					trackingNumber = s.TrackingNumber
				}
			}
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
		DeliveryCode:   deliveryCode,
		TrackingNumber: trackingNumber,
		Notes:          notes,
		QRCodeData:     qrData,
	}, nil
}
