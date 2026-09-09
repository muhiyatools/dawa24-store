package ui

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The buying surfaces' view of coverage.
//
// Every listing that pages in SQL needs the covering suppliers as a set before
// it runs its query, because a predicate applied to the rows a page returns
// cannot correct the count that page was cut from. This is that resolution, in
// one place, so the catalogue and the supplier profile cannot disagree about
// who delivers.

// coveringVendorsFor resolves which suppliers can deliver to a buying branch
// today.
//
// The second return value distinguishes "coverage was evaluated" from "coverage
// does not apply here". A visitor with no branch selected is browsing rather
// than buying and still sees the catalogue; a pharmacy whose branch nobody
// reaches sees an empty one, which is the truthful answer and the one the pager
// must agree with.
//
// A branch with neither coordinates nor a city is not covered by anyone —
// commerce.CheckAvailability refuses it with branch_no_location, and a listing
// that showed those offers anyway would be offering rows checkout will refuse.
func (h *UIHandler) coveringVendorsFor(ctx context.Context, branchID int64) ([]int64, bool) {
	vendors, _, evaluated := h.coveringVendorBranchesFor(ctx, branchID)
	return vendors, evaluated
}

// coveringVendorBranchesFor resolves both supplier organizations and the
// supplier branches that cover a buying branch. A vendor can cover Cairo from
// one branch while its Aswan variant belongs to another branch, so an
// organization-only set is not sufficient for offer-level pagination.
func (h *UIHandler) coveringVendorBranchesFor(ctx context.Context, branchID int64) ([]int64, []int64, bool) {
	if branchID <= 0 {
		return nil, nil, false
	}
	if h.coverageSvc == nil || h.orgSvc == nil {
		return nil, nil, true
	}

	branch, err := h.orgSvc.GetBranch(database.AsSystem(ctx), branchID)
	if err != nil || branch == nil {
		return nil, nil, true
	}

	coord := workflow.Coord{CityID: branch.CityID}
	if branch.Latitude != nil {
		coord.Lat = *branch.Latitude
	}
	if branch.Longitude != nil {
		coord.Lon = *branch.Longitude
	}
	hasLocation := (branch.Latitude != nil && branch.Longitude != nil) ||
		(branch.CityID != nil && *branch.CityID > 0)
	if !hasLocation {
		// Nothing can be evaluated against a branch with no location, and
		// showing everything would contradict the purchase rule. Coverage
		// applies and admits nobody.
		return nil, nil, true
	}

	covered, err := h.coverageSvc.VendorBranchesServing(ctx, time.Now().Weekday(), coord)
	if err != nil {
		h.log.WarnContext(ctx, "could not resolve covering suppliers for a buying branch",
			"branch_id", branchID, "error", err)
		// Coverage is a purchase precondition. An outage must not turn into an
		// unfiltered catalogue; the shared availability check also fails closed.
		return nil, nil, true
	}
	orgSet := make(map[int64]bool, len(covered))
	branchIDs := make([]int64, 0, len(covered))
	for _, row := range covered {
		if row.OrganizationID <= 0 {
			continue
		}
		orgSet[row.OrganizationID] = true
		if row.BranchID > 0 {
			branchIDs = append(branchIDs, row.BranchID)
		}
	}
	orgIDs := make([]int64, 0, len(orgSet))
	for id := range orgSet {
		orgIDs = append(orgIDs, id)
	}
	return orgIDs, branchIDs, true
}
