package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Reading the variant edit form.
//
// Every field follows one rule: a key the form did not send leaves the stored
// value alone. The handler this replaces assigned unconditionally from
// r.FormValue, and because the dialog carries no SKU, barcode, English-name or
// unit input, every save wrote an empty string over all four.
//
// The rule is expressed once, in formField, rather than repeated as an if per
// column — which is how the previous version came to guard some fields and not
// others.

// formField returns a submitted value and whether the form carried it at all.
func formField(r *http.Request, key string) (string, bool) {
	if r.PostForm == nil {
		return "", false
	}
	if !r.PostForm.Has(key) {
		return "", false
	}
	return strings.TrimSpace(r.PostFormValue(key)), true
}

// applyVariantEdit folds the submitted form onto a stored variant.
func applyVariantEdit(r *http.Request, v *catalog.ProductVariant, lang string) error {
	// The name is bilingual and the dialog only carries Arabic. Writing
	// i18n.New(nameAr, "") erased the English name on every save.
	nameAr, hasAr := formField(r, "name_ar")
	nameEn, hasEn := formField(r, "name_en")
	if hasAr || hasEn {
		next := v.Name
		if next == nil {
			next = i18n.Text{}
		}
		if hasAr && nameAr != "" {
			next[i18n.AR] = nameAr
		}
		if hasEn {
			next[i18n.EN] = nameEn
		}
		v.Name = next
	}

	if sku, ok := formField(r, "sku"); ok {
		v.SKU = sku
	}
	if barcode, ok := formField(r, "barcode"); ok {
		v.Barcode = barcode
	}
	if unit, ok := formField(r, "unit"); ok && unit != "" {
		v.Unit = unit
	}
	if batch, ok := formField(r, "batch_number"); ok {
		v.BatchNumber = batch
	}

	if raw, ok := formField(r, "price"); ok && raw != "" {
		price, err := money.Parse(raw)
		if err != nil || price.IsNegative() {
			return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_price"))
		}
		v.Price = price
	}
	if raw, ok := formField(r, "discount"); ok {
		if raw == "" {
			v.Discount = money.Zero
		} else {
			discount, err := money.Parse(raw)
			if err != nil || discount.IsNegative() || discount.Minor() > 10000 {
				return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_discount"))
			}
			v.Discount = discount
		}
	}

	// Cost price and its discount are optional, and clearing them is a real
	// choice — but only when the form actually carried an empty field.
	if raw, ok := formField(r, "cost_price"); ok {
		if raw == "" {
			v.CostPrice = nil
		} else {
			cost, err := money.Parse(raw)
			if err != nil || cost.IsNegative() {
				return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_price"))
			}
			v.CostPrice = &cost
		}
	}
	if raw, ok := formField(r, "cost_discount_percentage"); ok {
		if raw == "" {
			v.CostDiscountPercentage = 0
		} else {
			pct, err := strconv.ParseFloat(raw, 64)
			if err != nil || pct < 0 || pct > 100 {
				return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_discount"))
			}
			v.CostDiscountPercentage = pct
		}
	}

	if raw, ok := formField(r, "min_order_qty"); ok && raw != "" {
		qty, err := strconv.Atoi(raw)
		if err != nil || qty <= 0 {
			return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_min_qty"))
		}
		v.MinOrderQty = qty
	}

	if raw, ok := formField(r, "expiry_date"); ok {
		if raw == "" {
			v.ExpiryDate = nil
		} else {
			t, err := time.Parse("2006-01-02", raw)
			if err != nil {
				return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_expiry"))
			}
			v.ExpiryDate = &t
		}
	}

	if raw, ok := formField(r, "status"); ok && raw != "" {
		switch catalog.ProductStatus(raw) {
		case catalog.StatusActive, catalog.StatusInactive,
			catalog.StatusPending, catalog.StatusRejected:
			v.Status = catalog.ProductStatus(raw)
		default:
			return fmt.Errorf("%s", i18n.T(lang, "vendor.catalog.invalid_status"))
		}
	}

	if raw, ok := formField(r, "is_negotiable"); ok {
		v.IsNegotiable = isTruthy(raw)
	}

	return nil
}
