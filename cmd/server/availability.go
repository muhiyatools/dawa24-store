package main

import (
	"context"
	"errors"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// availabilityProbe answers the cross-module questions behind
// commerce.CheckAvailability: is this a real approved vendor, does this variant
// exist and have stock, does the branch belong to the buyer, and does the
// supplier cover it today.
//
// It lives in the composition root because modules must not import each other
// (ADR 0002) — the same reason docsGate is built here. commerce declares the
// AvailabilityProbe interface; this is its only implementation.
type availabilityProbe struct {
	catalog   *catalog.Service
	org       *org.Service
	coverage  *workflow.CoverageService
	inventory *inventory.Service
}

func newAvailabilityProbe(cat *catalog.Service, o *org.Service, cov *workflow.CoverageService, inv *inventory.Service) commerce.AvailabilityProbe {
	return &availabilityProbe{catalog: cat, org: o, coverage: cov, inventory: inv}
}

// Variant reads the stock and ownership facts for one sellable variant.
//
// The read runs AsSystem: a pharmacy is buying from a supplier, so it is by
// definition reading another tenant's catalog row. Only the four fields below
// cross the boundary, and only to answer "may I buy this".
func (p *availabilityProbe) Variant(ctx context.Context, variantID int64) (commerce.VariantAvailability, error) {
	if p.catalog == nil {
		return commerce.VariantAvailability{}, nil
	}
	v, err := p.catalog.GetVariant(database.AsSystem(ctx), variantID)
	if err != nil {
		if isNotFound(err) {
			return commerce.VariantAvailability{}, nil // zero ID = "does not exist"
		}
		return commerce.VariantAvailability{}, err
	}
	if v == nil {
		return commerce.VariantAvailability{}, nil
	}
	// Stock comes from inventory.stocks, NOT from v.StockQty. That field is
	// declared on catalog.ProductVariant but no query populates it — the column
	// does not exist on catalog.product_variants — so it is a permanent zero.
	// Reading it here would refuse every purchase.
	qty := 0
	if p.inventory != nil {
		n, err := p.inventory.AvailableQuantity(ctx, variantID)
		if err != nil {
			return commerce.VariantAvailability{}, err
		}
		qty = n
	}

	return commerce.VariantAvailability{
		ID:             v.ID,
		OrganizationID: v.OrganizationID,
		StockQty:       qty,
		MinOrderQty:    v.MinOrderQty,
		Active:         v.Status == catalog.StatusActive,
	}, nil
}

// Vendor reports whether the supplier is a real, approved vendor organization.
// AsSystem for the same reason as Variant: the buyer is not this tenant.
func (p *availabilityProbe) Vendor(ctx context.Context, orgID int64) (commerce.VendorAvailability, error) {
	if p.org == nil {
		return commerce.VendorAvailability{}, nil
	}
	o, err := p.org.GetOrganization(database.AsSystem(ctx), orgID)
	if err != nil {
		if isNotFound(err) {
			return commerce.VendorAvailability{}, nil
		}
		return commerce.VendorAvailability{}, err
	}
	if o == nil {
		return commerce.VendorAvailability{}, nil
	}
	return commerce.VendorAvailability{
		ID:       o.ID,
		IsVendor: string(o.Type) == "vendor" || string(o.Type) == "supplier" || string(o.Type) == "company" || string(o.Type) == "agency",
		Approved: string(o.Status) == "approved",
	}, nil
}

// CustomerBranch reads the buying pharmacy's own branch. This one stays
// tenant-scoped: commerce compares the returned OrganizationID against the
// actor's, and a caller must not be able to probe another pharmacy's branches.
func (p *availabilityProbe) CustomerBranch(ctx context.Context, branchID int64) (commerce.BranchAvailability, error) {
	if p.org == nil {
		return commerce.BranchAvailability{}, nil
	}
	b, err := p.org.GetBranch(ctx, branchID)
	if err != nil {
		if isNotFound(err) {
			return commerce.BranchAvailability{}, nil
		}
		return commerce.BranchAvailability{}, err
	}
	if b == nil {
		return commerce.BranchAvailability{}, nil
	}
	return commerce.BranchAvailability{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		Latitude:       b.Latitude,
		Longitude:      b.Longitude,
	}, nil
}

// VendorCovers defers to the one implementation of the coverage rule. There is
// deliberately no second distance calculation anywhere in this codebase.
func (p *availabilityProbe) VendorCovers(ctx context.Context, vendorOrgID int64, lat, lon float64, day time.Weekday) (bool, error) {
	if p.coverage == nil {
		return false, nil // fail closed
	}
	served, _, err := p.coverage.ServesPoint(ctx, vendorOrgID, day, workflow.Coord{Lat: lat, Lon: lon})
	if err != nil {
		return false, err
	}
	return served, nil
}

// isNotFound distinguishes "this row does not exist (or is not visible to this
// tenant)" from a real failure. A missing row is a refusal reason, not an
// outage: commerce turns it into a message the pharmacy can act on, while a
// genuine error surfaces and stops the purchase.
func isNotFound(err error) bool {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Kind == apperr.KindNotFound
	}
	return database.IsNotFound(err)
}
