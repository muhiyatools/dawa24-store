package smartorder

import (
	"context"
	"time"
)

// Adapters that let the composition root wire smart ordering to services owned
// by other modules without smartorder importing them.
//
// Each is a function type satisfying the interface the pipeline or finalizer
// declares. The composition root closes over the real service; tests close over
// a stub. This is what keeps AGENTS.md rule 5 — modules do not import modules —
// from turning into either a copy of the coverage rules or a cyclic import.

// CoverageFunc adapts workflow.CoverageService.ServesPoint.
type CoverageFunc func(ctx context.Context, vendorOrgID int64, day time.Weekday, lat, lng float64) (bool, int, error)

// Serves satisfies the pipeline's CoverageGate.
func (f CoverageFunc) Serves(ctx context.Context, vendorOrgID int64, day time.Weekday, lat, lng float64) (bool, int, error) {
	return f(ctx, vendorOrgID, day, lat, lng)
}

// InstitutionalFunc adapts the org module's institutional gate in Simple mode.
type InstitutionalFunc func(ctx context.Context, buyerOrgID int64, workIDs []int64) (bool, error)

// Visible satisfies the pipeline's InstitutionalGate.
func (f InstitutionalFunc) Visible(ctx context.Context, buyerOrgID int64, workIDs []int64) (bool, error) {
	return f(ctx, buyerOrgID, workIDs)
}

// BranchLocationFunc adapts the org module's branch lookup.
type BranchLocationFunc func(ctx context.Context, orgID, branchID int64) (lat, lng float64, ok bool, err error)

// Location satisfies the worker's BranchResolver.
func (f BranchLocationFunc) Location(ctx context.Context, orgID, branchID int64) (float64, float64, bool, error) {
	return f(ctx, orgID, branchID)
}

// PlaceOrderFunc adapts commerce checkout.
type PlaceOrderFunc func(ctx context.Context, req PlaceOrderRequest) (int64, error)

// PlaceOrder satisfies OrderPlacer.
func (f PlaceOrderFunc) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (int64, error) {
	return f(ctx, req)
}

// SimpleInstitutionalGate is the Simple-mode rule, which is the one ordinary
// buyer catalogue browsing applies: a product with no restriction is visible to
// everyone, and a restricted product is visible when the buyer holds one of its
// works.
//
// Implemented here rather than called across a module boundary because it is
// three lines of set intersection, and the alternative — an interface call per
// candidate — would undo the batching the pipeline depends on. The *authorised
// works* still come from org; only the comparison lives here.
func SimpleInstitutionalGate(authorizedWorkIDs []int64) InstitutionalFunc {
	authorized := make(map[int64]bool, len(authorizedWorkIDs))
	for _, id := range authorizedWorkIDs {
		authorized[id] = true
	}
	return func(_ context.Context, _ int64, workIDs []int64) (bool, error) {
		if len(workIDs) == 0 {
			return true, nil // unrestricted
		}
		for _, id := range workIDs {
			if authorized[id] {
				return true, nil
			}
		}
		return false, nil
	}
}

// AlwaysCovered is the coverage gate used when the buyer's branch has no
// coordinates.
//
// Refusing every supplier because an address is incomplete would be worse than
// the alternative, and it matches how the rest of the platform behaves when
// coverage data is absent. The worker logs a warning when this path is taken, so
// it is visible rather than silent.
func AlwaysCovered() CoverageFunc {
	return func(context.Context, int64, time.Weekday, float64, float64) (bool, int, error) {
		return true, 0, nil
	}
}
