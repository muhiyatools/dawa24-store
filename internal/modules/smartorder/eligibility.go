package smartorder

// Offer eligibility.
//
// A vendor's offer has to survive six checks before a buyer may order it. The
// checks are ordered, and the order is the point: the **first** failure is what
// gets reported, because that is the most actionable answer for the buyer.
//
// A product that is both out of stock and outside the coverage window is
// reported as a coverage problem, not a stock problem — the stock will be
// irrelevant tomorrow morning when the window reopens, and telling the buyer
// "out of stock" sends them hunting for a different supplier they did not need
// to find. Ordering cheap, structural checks first also means the expensive ones
// (coverage needs a distance calculation) run on fewer candidates.
//
// Coverage and Corporate Operations are decided by the existing services and
// arrive here as booleans. This function owns the *ordering* and the reasons,
// not the rules themselves — those must stay in one place and be identical to
// what ordinary purchasing enforces.

// OfferCheck is everything needed to decide whether one offer is orderable.
type OfferCheck struct {
	BuyerOrgID  int64
	VendorOrgID int64

	// ProductActive covers product, variant and vendor approval together: any
	// of them being inactive makes the offer unbuyable for the same reason and
	// with the same remedy.
	ProductActive bool

	// InstitutionallyVisible is the Corporate Operations verdict, evaluated in
	// the same mode ordinary catalogue browsing uses.
	InstitutionallyVisible bool

	// Covered is the weekly-coverage verdict for the delivery branch, evaluated
	// at this moment: right weekday, inside the time window, inside the radius.
	Covered bool

	StockQty     int
	RequestedQty float64
	MinOrderQty  int
}

// Evaluate returns whether the offer is orderable, and if not, the first reason
// it failed.
func Evaluate(c OfferCheck) (bool, IneligibleReason) {
	// 1. A buyer never buys from itself. Cheapest check, and a marketplace
	//    invariant rather than a business rule that might be relaxed.
	if c.VendorOrgID == c.BuyerOrgID {
		return false, ReasonOwnOrg
	}

	// 2. Inactive product, variant or unapproved vendor.
	if !c.ProductActive {
		return false, ReasonInactive
	}

	// 3. Corporate Operations. Before coverage because it is a permissions
	//    question: a buyer who may not see this product at all should not be
	//    told about delivery windows for it.
	if !c.InstitutionallyVisible {
		return false, ReasonInstitutional
	}

	// 4. Coverage for the delivery branch, at this moment.
	if !c.Covered {
		return false, ReasonCoverage
	}

	// 5. Stock.
	if c.StockQty <= 0 {
		return false, ReasonStock
	}

	// 6. Minimum order quantity. Last because it is the only check that depends
	//    on what the buyer asked for rather than on the offer alone — and the
	//    only one the buyer can fix by editing a number.
	if c.MinOrderQty > 0 && c.RequestedQty > 0 && c.RequestedQty < float64(c.MinOrderQty) {
		return false, ReasonMinQty
	}

	return true, ""
}

// OutcomeFor derives a line's outcome from the candidates found for it.
//
// When every candidate was rejected, the line reports the *least severe*
// obstacle across them — the one closest to being orderable. If one vendor is
// merely out of stock while three others are outside coverage, "out of stock" is
// the useful answer: it is the one a buyer can act on by waiting or by asking
// that vendor, whereas coverage is a fact about geography.
func OutcomeFor(matched bool, effectiveQty float64, candidates []Candidate) (Outcome, IneligibleReason) {
	if !matched {
		return OutcomeUnmatched, ""
	}
	if effectiveQty <= 0 {
		return OutcomeZeroQty, ""
	}
	if len(candidates) == 0 {
		return OutcomeNoSupplier, ""
	}

	for _, c := range candidates {
		if c.Eligible {
			return OutcomeOrdered, ""
		}
	}

	// Severity order, least severe first.
	ranked := []struct {
		reason  IneligibleReason
		outcome Outcome
	}{
		{ReasonMinQty, OutcomeBelowMinQty},
		{ReasonStock, OutcomeOutOfStock},
		{ReasonCoverage, OutcomeCoverageBlocked},
		{ReasonInstitutional, OutcomeInstitutionalBlocked},
		{ReasonInactive, OutcomeNoSupplier},
		{ReasonOwnOrg, OutcomeNoSupplier},
	}
	for _, r := range ranked {
		for _, c := range candidates {
			if c.IneligibleReason == r.reason {
				return r.outcome, r.reason
			}
		}
	}

	return OutcomeNoSupplier, ""
}

// CountByOutcome tallies lines for the results statistics.
func CountByOutcome(lines []*Line) Stats {
	var s Stats
	for _, l := range lines {
		s.TotalRows++
		if l.Matched() {
			s.MatchedRows++
		}
		switch l.Outcome {
		case OutcomeUnmatched:
			s.UnmatchedRows++
		case OutcomeNoSupplier:
			s.NoSupplierRows++
		case OutcomeCoverageBlocked:
			s.CoverageBlockedRows++
		case OutcomeInstitutionalBlocked:
			s.InstitutionalBlockedRows++
		case OutcomeBelowMinQty:
			s.BelowMinQtyRows++
		}
	}
	return s
}
