package main

// The compare tool's view of the shared catalogue.
//
// A module may not import another module's types, so compare declares its own
// CatalogProduct and the catalogue declares MatchProduct, and the two are the
// same eleven strings. This is the one place that says so. It is three lines of
// copying per product and it buys the property the architecture is for: the
// compare tool can be built, tested and reasoned about without the catalogue
// module, and the catalogue does not know the compare tool exists.

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// catalogLister is the half of the catalogue service the compare tool needs.
type catalogLister interface {
	ListMatchProducts(ctx context.Context) ([]catalog.MatchProduct, error)
}

// compareCatalog adapts the catalogue service to the compare tool's port.
type compareCatalog struct{ src catalogLister }

func newCompareCatalog(src catalogLister) compare.CatalogSource {
	if src == nil {
		return nil
	}
	return &compareCatalog{src: src}
}

func (c *compareCatalog) ListMatchProducts(ctx context.Context) ([]compare.CatalogProduct, error) {
	// As the system: the shared catalogue belongs to no tenant, and a vendor
	// matching their own price list against it is reading, not writing.
	products, err := c.src.ListMatchProducts(database.AsSystem(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]compare.CatalogProduct, 0, len(products))
	for _, p := range products {
		out = append(out, compare.CatalogProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barcode, Scientific: p.Scientific, DosageForm: p.DosageForm,
			Concentration: p.Concentration, Unit: p.Unit,
			Manufacturer: p.Manufacturer, PublicPrice: p.PublicPrice,
		})
	}
	return out, nil
}
