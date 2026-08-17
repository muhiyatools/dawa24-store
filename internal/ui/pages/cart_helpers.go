package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CartTotal sums the line totals of a cart. Overflow is impossible for real
// basket sizes, but on the off chance a value is absurd the line is skipped
// rather than panicking.
func CartTotal(cart *commerce.Cart) money.Amount {
	total := money.Zero
	if cart == nil {
		return total
	}
	for _, item := range cart.Items {
		line, err := item.UnitPrice.MulInt(int64(item.Quantity))
		if err != nil {
			continue
		}
		if sum, err := total.Add(line); err == nil {
			total = sum
		}
	}
	return total
}

// CartItemTotal calculates line total for a single cart item.
func CartItemTotal(item *commerce.CartItem) money.Amount {
	if item == nil {
		return money.Zero
	}
	line, err := item.UnitPrice.MulInt(int64(item.Quantity))
	if err != nil {
		return money.Zero
	}
	return line
}

// CartGroup is one supplier's slice of the cart.
type CartGroup struct {
	OrganizationID int64
	SupplierName   i18n.Text
	MinOrderPrice  money.Amount
	BelowMin       bool
	Items          []*commerce.CartItem
	Subtotal       money.Amount
}

// GroupCartBySupplier partitions the cart by supplier, in order of appearance,
// with a per-supplier subtotal and a minimum-order flag.
func GroupCartBySupplier(cart *commerce.Cart) []CartGroup {
	if cart == nil {
		return nil
	}
	var groups []CartGroup
	byOrg := map[int64]int{}
	for _, item := range cart.Items {
		idx, ok := byOrg[item.OrganizationID]
		if !ok {
			idx = len(groups)
			byOrg[item.OrganizationID] = idx
			groups = append(groups, CartGroup{
				OrganizationID: item.OrganizationID,
				SupplierName:   item.SupplierName,
				MinOrderPrice:  item.MinOrderPrice,
			})
		}
		groups[idx].Items = append(groups[idx].Items, item)
		if line, err := item.UnitPrice.MulInt(int64(item.Quantity)); err == nil {
			if sum, err := groups[idx].Subtotal.Add(line); err == nil {
				groups[idx].Subtotal = sum
			}
		}
	}
	for i := range groups {
		groups[i].BelowMin = groups[i].MinOrderPrice.Minor() > 0 && groups[i].Subtotal.Minor() < groups[i].MinOrderPrice.Minor()
	}
	return groups
}
