package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// validateSpecialOfferForCheckout refuses an offer bundle that must not be
// sold: withdrawn by the vendor, rejected/pending with the platform admins,
// or outside its date window. Empty means the offer may be checked out.
func validateSpecialOfferForCheckout(spo *promo.SpecialOffer) string {
	if spo == nil {
		return "العرض المطلوب غير موجود."
	}
	if spo.Status == "inactive" || spo.Status == "draft" {
		return "هذا العرض موقوف حالياً من المورد ولا يمكن إتمام الطلب عليه."
	}
	if spo.Status == "expired" {
		return "انتهت صلاحية هذا العرض ولا يمكن إتمام الطلب عليه."
	}
	if spo.AdminStatus == "pending" {
		return "هذا العرض قيد مراجعة إدارة المنصة ولم يُعتمد بعد."
	}
	if spo.AdminStatus == "rejected" {
		return "تم رفض هذا العرض من إدارة المنصة ولا يمكن إتمام الطلب عليه."
	}
	now := time.Now()
	if spo.StartDate != nil && now.Before(*spo.StartDate) {
		return "هذا العرض لم يبدأ بعد ولا يمكن إتمام الطلب عليه."
	}
	if spo.EndDate != nil && now.After(*spo.EndDate) {
		return "انتهت صلاحية هذا العرض ولا يمكن إتمام الطلب عليه."
	}
	return ""
}

// checkoutValidationMessage maps checkout validation codes to specific Arabic
// messages. It reports false for non-validation errors, which keep the
// generic renderError path.
func checkoutValidationMessage(lang string, err error) (string, bool) {
	_ = lang
	ae, ok := apperr.As(err)
	if !ok || ae == nil || ae.Kind != apperr.KindValidation {
		return "", false
	}
	switch {
	case ae.Code == "checkout.min_order_not_met":
		total := ""
		min := ""
		if ae.Fields != nil {
			total = ae.Fields["order_total"]
			min = ae.Fields["min_order_total"]
		}
		if total != "" && min != "" {
			return fmt.Sprintf("إجمالي الطلب (%s ج.م) أقل من الحد الأدنى المطلوب للعرض (%s ج.م). أضف أصنافاً أو ارفع الكمية ثم أعد المحاولة.", total, min), true
		}
		return "إجمالي الطلب أقل من الحد الأدنى المطلوب لهذا العرض. أضف أصنافاً أو ارفع الكمية ثم أعد المحاولة.", true
	case ae.Code == "item.vendor_required":
		return "تعذر تحديد المورد لأحد سطور السلة. احذف السطر وأعد إضافته من صفحة العرض، ثم أعد المحاولة.", true
	case ae.Code == "checkout.empty_cart":
		return "سلة المشتريات فارغة. أضف أصنافاً أولاً.", true
	case ae.Code == "item.quantity_invalid":
		return "كمية غير صالحة في أحد سطور السلة. راجع الكميات ثم أعد المحاولة.", true
	case strings.HasPrefix(ae.Code, "checkout.line_unavailable."):
		// Availability refusals already carry the specific Arabic reason.
		if ae.Msg != "" {
			return ae.Msg, true
		}
		return "أحد الأصناف غير متاح حالياً (نفد المخزون أو خارج التغطية). راجع السلة ثم أعد المحاولة.", true
	default:
		// Every other validation refusal already carries a message the domain
		// wrote for a person to read.
		if ae.Msg != "" {
			return ae.Msg, true
		}
		return "", false
	}
}
