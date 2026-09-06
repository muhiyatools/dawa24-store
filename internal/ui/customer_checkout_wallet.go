package ui

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// computeCheckoutGoodsAmount calculates the exact sum of line items net of discounts.
func computeCheckoutGoodsAmount(items []commerce.CheckoutLineItem) money.Amount {
	var total money.Amount
	for _, item := range items {
		lineSubtotal, err := item.UnitPrice.MulInt(int64(item.Quantity))
		if err != nil {
			continue
		}
		lineTotal := lineSubtotal
		if item.DiscountAmount.IsPositive() && item.DiscountAmount.Minor() < lineSubtotal.Minor() {
			if sub, err := lineSubtotal.Sub(item.DiscountAmount); err == nil {
				lineTotal = sub
			}
		}
		total, _ = total.Add(lineTotal)
	}
	return total
}

// processWalletPayment verifies tenant wallet funds and executes an atomic debit.
func (h *UIHandler) processWalletPayment(
	ctx context.Context,
	actor authctx.Actor,
	goodsAmount money.Amount,
) (int64, error) {
	if !actor.IsCustomer() || actor.OrganizationID <= 0 {
		return 0, apperr.Validation("wallet.pharmacy_required", "يجب تسجيل الدخول بحساب صيدلية تابعة لمؤسسة للاستفادة من الدفع بالمحفظة.", nil)
	}
	if h.billSvc == nil {
		return 0, apperr.Validation("wallet.service_unavailable", "خدمة المحفظة غير متاحة حالياً.", nil)
	}
	if !goodsAmount.IsPositive() {
		return 0, apperr.Validation("wallet.invalid_amount", "المبلغ المطلوب سداده غير صالح.", nil)
	}

	walletUserID, _ := resolveTenantUserIDs(ctx, h, actor)
	wallet, err := h.billSvc.GetWallet(ctx, walletUserID, "EGP")
	if err != nil || wallet == nil {
		return 0, apperr.Validation("wallet.not_found", "تعذر الوصول للمحفظة المؤسسية الخاصة بحسابك.", nil)
	}

	if wallet.Balance.Minor() < goodsAmount.Minor() {
		return 0, apperr.Validation("wallet.insufficient_funds",
			fmt.Sprintf("رصيد المحفظة المؤسسية (%s ج.م) غير كافٍ لسداد قيمة الأصناف المطلوبة (%s ج.م).", wallet.Balance.String(), goodsAmount.String()), nil)
	}

	// Atomic debit with pessimistic FOR UPDATE lock in billing repository
	desc := "سداد قيمة مشتريات أدوية من المحفظة المؤسسية"
	_, err = h.billSvc.Withdraw(ctx, walletUserID, "EGP", goodsAmount, "order", nil, desc)
	if err != nil {
		return 0, err
	}

	return walletUserID, nil
}