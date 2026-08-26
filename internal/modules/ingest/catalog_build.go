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
	v.CostPrice = row.CostPrice
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
	net := row.NetPrice
	if !net.IsPositive() || net.Minor() >= list.Minor() {
		return list, money.Zero
	}
	diff, err := list.Sub(net)
	if err != nil {
		return list, money.Zero
	}
	return list, diff
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
	switch row.Status {
	case "active":
		return catalog.StatusActive
	case "inactive":
		return catalog.StatusInactive
	}
	if w.settings.PublishImmediately {
		return catalog.StatusActive
	}
	return catalog.StatusInactive
}

// buildProduct renders a row as a shared-catalogue product.
//
// It is deliberately thin. Only what the supplier actually stated is carried
// across, plus the two fields the engine read out of the name; the price is the
// public one because that is the figure the catalogue is meant to hold, and the
// vendor's own discount belongs to their variant, not to the shared record.
//
// The status follows the vendor's own publish setting rather than being pinned
// to `pending`. Pinning it was a defensible policy that turned out to be a trap:
// every product a bulk import introduced sat unapproved, and because matching —
// both the smart order's and the next import's — only ever resolves to live
// catalogue rows, three and a half thousand pharmaceutical products were
// invisible to the engine that was supposed to find them. A catalogue nothing
// can match is not a cautious catalogue, it is an empty one.
func (w *importWriter) buildProduct(row *productmatch.Row) *catalog.Product {
	// `pending` rather than `inactive` when the vendor has not asked for
	// immediate publication. The two are not interchangeable on a shared
	// catalogue row: `pending` means nobody has looked at it yet and it belongs
	// in the approval queue, `inactive` means somebody looked and withdrew it.
	status := catalog.StatusActive
	if !w.settings.PublishImmediately {
		status = catalog.StatusPending
	}
	if row.Status == "inactive" {
		status = catalog.StatusInactive
	}
	// The supplier's own item code is deliberately not carried across. A shared
	// catalogue keyed on one vendor's internal numbering collides the moment a
	// second vendor uploads; their code stays on their variant, where it means
	// something.
	p := &catalog.Product{
		Name:                   variantName(row),
		Barcode:                row.Barcode,
		Price:                  row.PublicPrice,
		OldPrice:               row.PublicPrice,
		ScientificName:         row.Scientific,
		DosageForm:             row.DosageForm,
		Concentration:          row.Concentration,
		Unit:                   row.Unit,
		ManufacturingCompanies: row.Manufacturer,
		Status:                 status,
		InstitutionalWorkIDs:   []int64{},
	}
	return p
}
