package ui

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// buildPrintableInvoiceData assembles the full printable invoice data model,
// including resolution of item codes, pharmaceutical batch numbers, and expiry dates.
func (h *UIHandler) buildPrintableInvoiceData(ctx context.Context, invoice *billing.Invoice, order *commerce.Order, lang any) (*billing.PrintableInvoiceData, error) {
	if invoice == nil && order == nil {
		return nil, fmt.Errorf("both invoice and order are nil")
	}

	if invoice == nil && order != nil && h.billSvc != nil {
		if inv, err := h.billSvc.GetInvoiceByOrderID(ctx, order.ID); err == nil && inv != nil {
			invoice = inv
		} else {
			invoice = h.synthesizeInvoiceFromOrder(ctx, order)
		}
	}

	if order == nil && invoice != nil && invoice.OrderID != nil && h.commSvc != nil {
		if ord, _ := h.commSvc.GetOrder(ctx, *invoice.OrderID); ord != nil {
			order = ord
		}
	}

	vendorInfo := h.buildPrintableVendorInfo(ctx, invoice, lang)
	custInfo := h.buildPrintableCustomerInfo(ctx, invoice, order, lang)
	printableLines := h.buildPrintableLines(ctx, invoice, order)

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

func (h *UIHandler) buildPrintableLines(ctx context.Context, invoice *billing.Invoice, order *commerce.Order) []*billing.PrintableInvoiceLine {
	var varIDs []int64
	var prodIDs []int64
	if order != nil {
		for _, ol := range order.Lines {
			if ol.ProductVariantID != nil && *ol.ProductVariantID > 0 {
				varIDs = append(varIDs, *ol.ProductVariantID)
			}
			if ol.ProductID != nil && *ol.ProductID > 0 {
				prodIDs = append(prodIDs, *ol.ProductID)
			}
		}
	}
	if invoice != nil {
		for _, il := range invoice.Lines {
			if il.ProductID != nil && *il.ProductID > 0 {
				prodIDs = append(prodIDs, *il.ProductID)
			}
		}
	}

	var variantsMap map[int64]*catalog.ProductVariant
	var prodVariantsMap map[int64][]*catalog.ProductVariant
	if h.catSvc != nil {
		if len(varIDs) > 0 {
			variantsMap, _ = h.catSvc.GetVariantsByIDs(ctx, varIDs)
		}
		if len(prodIDs) > 0 {
			prodVariantsMap, _ = h.catSvc.ListVariantsByProducts(ctx, prodIDs)
		}
	}

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

			var matchedVariantID *int64
			lineSKU := ""
			if order != nil && idx < len(order.Lines) {
				matchedVariantID = order.Lines[idx].ProductVariantID
				lineSKU = order.Lines[idx].SKU
			}

			sku, batch, expiry := resolveVariantLineInfo(l.ProductID, matchedVariantID, lineSKU, variantsMap, prodVariantsMap)
			printableLines = append(printableLines, &billing.PrintableInvoiceLine{
				Index:           idx + 1,
				ProductID:       l.ProductID,
				ItemName:        l.Description,
				SKU:             sku,
				BatchNumber:     batch,
				ExpiryDate:      expiry,
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

			sku, batch, expiry := resolveVariantLineInfo(l.ProductID, l.ProductVariantID, l.SKU, variantsMap, prodVariantsMap)
			printableLines = append(printableLines, &billing.PrintableInvoiceLine{
				Index:           idx + 1,
				ProductID:       l.ProductID,
				ItemName:        l.ProductName.Get("ar"),
				SKU:             sku,
				BatchNumber:     batch,
				ExpiryDate:      expiry,
				Quantity:        l.Quantity,
				UnitPrice:       unitPrice,
				DiscountPercent: discPct,
				NetUnitPrice:    netPrice,
				TotalPrice:      l.TotalPrice,
				IsExempt:        true,
			})
		}
	}
	return printableLines
}

func resolveVariantLineInfo(
	productID *int64,
	variantID *int64,
	existingSKU string,
	variantsMap map[int64]*catalog.ProductVariant,
	prodVariantsMap map[int64][]*catalog.ProductVariant,
) (sku, batch, expiry string) {
	sku = existingSKU
	if variantID != nil && variantsMap != nil {
		if v, ok := variantsMap[*variantID]; ok && v != nil {
			if sku == "" {
				sku = v.SKU
			}
			batch = v.BatchNumber
			if v.ExpiryDate != nil && !v.ExpiryDate.IsZero() {
				expiry = v.ExpiryDate.Format("2006-01-02")
			}
		}
	}
	if (batch == "" || expiry == "" || sku == "") && productID != nil && prodVariantsMap != nil {
		if list, ok := prodVariantsMap[*productID]; ok && len(list) > 0 {
			for _, v := range list {
				if v == nil {
					continue
				}
				if sku == "" && v.SKU != "" {
					sku = v.SKU
				}
				if batch == "" && v.BatchNumber != "" {
					batch = v.BatchNumber
				}
				if expiry == "" && v.ExpiryDate != nil && !v.ExpiryDate.IsZero() {
					expiry = v.ExpiryDate.Format("2006-01-02")
				}
				if sku != "" && batch != "" && expiry != "" {
					break
				}
			}
		}
	}
	return sku, batch, expiry
}
