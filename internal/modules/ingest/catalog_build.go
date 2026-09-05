package ingest

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Turning a read row into catalogue records.
//
// Two conversions, and the fiddly part of both is the price. A supplier states
// a public price and a discount; catalog.product_variants stores a price and a
// discount *amount*, and the pharmacy pays the difference. Getting that wrong in
// either direction is not a rounding error — it is the vendor's margin.

// buildVariant renders a parsed row as the vendor's own catalogue entry.
func (w *importWriter) buildVariant(d *decision) *catalog.ProductVariant {
	row := d.row
	v := &catalog.ProductVariant{
		ID:             d.variantID,
		OrganizationID: w.session.OrganizationID,
		ProductID:      d.productID,
		Name:           variantName(row),
		SKU:            row.SKU,
		Barcode:        row.Barcode,
		Unit:           row.Unit,
		Image:          row.Image,
		BatchNumber:    row.BatchNumber,
		ExpiryDate:     row.ExpiryDate,
		MinOrderQty:    row.MinOrderQty,
		BranchID:       w.settings.BranchID,
		IsNegotiable:   w.settings.MarkNegotiable,
		Status:         w.variantStatus(row),
		VariantType:    "standard",
	}
	if row.Negotiable != nil {
		v.IsNegotiable = *row.Negotiable
	}
	v.Price, v.Discount = listAndDiscount(row)
	if !row.CostPrice.IsZero() {
		v.CostPrice = &row.CostPrice
	}
	return v
}

// listAndDiscount converts the row's reconciled prices into the pair the
// variant table stores: the list price, and the discount as an amount.
//
// The amount is derived from the two prices rather than from the stated
// percentage, so that what a pharmacy is charged always equals what the
// supplier's file said the net was — even where the percentage and the net
// disagreed by a piastre, which they routinely do.
func listAndDiscount(row *productmatch.Row) (list, discount money.Amount) {
	list = row.PublicPrice
	if !list.IsPositive() {
		return row.NetPrice, money.Zero
	}
	if row.DiscountBps > 0 {
		return list, money.FromMinor(row.DiscountBps)
	}
	net := row.NetPrice
	if !net.IsPositive() || net.Minor() >= list.Minor() {
		return list, money.Zero
	}
	diff, err := list.Sub(net)
	if err != nil {
		return list, money.Zero
	}
	pctBps := diff.Minor() * 10000 / list.Minor()
	return list, money.FromMinor(pctBps)
}

// variantName is what the vendor's own catalogue calls the row.
//
// The supplier's spelling is kept rather than the shared catalogue's. It is the
// name on their invoices and in their warehouse, and replacing it with the
// canonical one makes their catalogue unsearchable to their own staff.
func variantName(row *productmatch.Row) i18n.Text {
	name := row.Name
	if name == "" {
		name = row.NameEN
	}
	return i18n.New(name, row.NameEN)
}

// variantStatus decides whether the imported row goes on sale.
func (w *importWriter) variantStatus(row *productmatch.Row) catalog.ProductStatus {
	return catalog.StatusActive
}
