package smartorder

import "testing"

// okOffer is an offer that passes every check; each test breaks exactly one.
func okOffer() OfferCheck {
	return OfferCheck{
		BuyerOrgID:             50,
		VendorOrgID:            51,
		ProductActive:          true,
		InstitutionallyVisible: true,
		Covered:                true,
		StockQty:               100,
		RequestedQty:           10,
		MinOrderQty:            1,
	}
}

func TestEligibleOfferPasses(t *testing.T) {
	ok, reason := Evaluate(okOffer())
	if !ok {
		t.Fatalf("expected the offer to be eligible, rejected for %q", reason)
	}
}

// FR-025. A buyer must never be sold its own stock.
func TestOwnOrganisationIsNeverEligible(t *testing.T) {
	c := okOffer()
	c.VendorOrgID = c.BuyerOrgID
	ok, reason := Evaluate(c)
	if ok {
		t.Fatal("a buyer must never be offered its own product")
	}
	if reason != ReasonOwnOrg {
		t.Fatalf("expected own_org, got %q", reason)
	}
}

// The own-org rule outranks everything, so it holds even for an offer that is
// broken in every other way too.
func TestOwnOrganisationOutranksEveryOtherFailure(t *testing.T) {
	c := okOffer()
	c.VendorOrgID = c.BuyerOrgID
	c.ProductActive = false
	c.Covered = false
	c.StockQty = 0
	if _, reason := Evaluate(c); reason != ReasonOwnOrg {
		t.Fatalf("expected own_org to be reported first, got %q", reason)
	}
}

func TestEachCheckReportsItsOwnReason(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*OfferCheck)
		want   IneligibleReason
	}{
		{"inactive", func(c *OfferCheck) { c.ProductActive = false }, ReasonInactive},
		{"institutional", func(c *OfferCheck) { c.InstitutionallyVisible = false }, ReasonInstitutional},
		{"coverage", func(c *OfferCheck) { c.Covered = false }, ReasonCoverage},
		{"stock", func(c *OfferCheck) { c.StockQty = 0 }, ReasonStock},
		{"min qty", func(c *OfferCheck) { c.MinOrderQty = 50; c.RequestedQty = 10 }, ReasonMinQty},
	}
	for _, tc := range cases {
		c := okOffer()
		tc.mutate(&c)
		ok, reason := Evaluate(c)
		if ok {
			t.Errorf("%s: expected rejection", tc.name)
			continue
		}
		if reason != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.name, tc.want, reason)
		}
	}
}

// The ordering is the design, not an accident: the first failure is the most
// actionable answer for the buyer.
func TestCoverageIsReportedBeforeStock(t *testing.T) {
	c := okOffer()
	c.Covered = false
	c.StockQty = 0
	if _, reason := Evaluate(c); reason != ReasonCoverage {
		t.Fatalf("an offer outside coverage should say so, not blame stock: got %q", reason)
	}
}

func TestInstitutionalIsReportedBeforeCoverage(t *testing.T) {
	c := okOffer()
	c.InstitutionallyVisible = false
	c.Covered = false
	if _, reason := Evaluate(c); reason != ReasonInstitutional {
		t.Fatalf("a permissions failure outranks a delivery one, got %q", reason)
	}
}

func TestMinOrderQuantityIgnoredWhenNothingRequested(t *testing.T) {
	// A line with no quantity yet must not be rejected for being under a
	// minimum it has not been measured against.
	c := okOffer()
	c.MinOrderQty = 50
	c.RequestedQty = 0
	if ok, reason := Evaluate(c); !ok {
		t.Fatalf("expected eligibility with no quantity requested, got %q", reason)
	}
}

func TestOutcomeForUnmatchedLine(t *testing.T) {
	if got, _ := OutcomeFor(false, 5, nil); got != OutcomeUnmatched {
		t.Fatalf("expected unmatched, got %s", got)
	}
}

func TestOutcomeForZeroQuantityBeatsSupplierState(t *testing.T) {
	// Quantity zero means the buyer has not asked for it; supplier availability
	// is not the interesting fact about that line.
	if got, _ := OutcomeFor(true, 0, []Candidate{{Eligible: true}}); got != OutcomeZeroQty {
		t.Fatalf("expected zero_qty, got %s", got)
	}
}

func TestOutcomeForNoCandidatesIsNoSupplier(t *testing.T) {
	if got, _ := OutcomeFor(true, 5, nil); got != OutcomeNoSupplier {
		t.Fatalf("expected no_supplier, got %s", got)
	}
}

func TestOutcomeForAnyEligibleCandidateIsOrdered(t *testing.T) {
	candidates := []Candidate{
		{Eligible: false, IneligibleReason: ReasonCoverage},
		{Eligible: true},
	}
	if got, _ := OutcomeFor(true, 5, candidates); got != OutcomeOrdered {
		t.Fatalf("one eligible candidate is enough, got %s", got)
	}
}

// FR-026: the buyer must be able to tell these apart. Reporting the least
// severe obstacle means reporting the one closest to being fixable.
func TestOutcomeForReportsLeastSevereObstacle(t *testing.T) {
	candidates := []Candidate{
		{Eligible: false, IneligibleReason: ReasonInstitutional},
		{Eligible: false, IneligibleReason: ReasonCoverage},
		{Eligible: false, IneligibleReason: ReasonStock},
	}
	got, reason := OutcomeFor(true, 5, candidates)
	if got != OutcomeOutOfStock {
		t.Fatalf("expected out_of_stock as the most actionable answer, got %s", got)
	}
	if reason != ReasonStock {
		t.Fatalf("expected reason stock, got %q", reason)
	}
}

func TestOutcomeForDistinguishesCoverageFromInstitutional(t *testing.T) {
	coverage := []Candidate{{Eligible: false, IneligibleReason: ReasonCoverage}}
	if got, _ := OutcomeFor(true, 5, coverage); got != OutcomeCoverageBlocked {
		t.Fatalf("expected coverage_blocked, got %s", got)
	}

	institutional := []Candidate{{Eligible: false, IneligibleReason: ReasonInstitutional}}
	if got, _ := OutcomeFor(true, 5, institutional); got != OutcomeInstitutionalBlocked {
		t.Fatalf("expected institutional_blocked, got %s", got)
	}
}

func TestCountByOutcome(t *testing.T) {
	pid := int64(1)
	lines := []*Line{
		{MatchedProductID: &pid, Outcome: OutcomeOrdered},
		{MatchedProductID: &pid, Outcome: OutcomeCoverageBlocked},
		{MatchedProductID: &pid, Outcome: OutcomeInstitutionalBlocked},
		{MatchedProductID: &pid, Outcome: OutcomeNoSupplier},
		{Outcome: OutcomeUnmatched},
		{MatchedProductID: &pid, Outcome: OutcomeBelowMinQty},
	}
	s := CountByOutcome(lines)

	if s.TotalRows != 6 {
		t.Errorf("total: expected 6, got %d", s.TotalRows)
	}
	if s.MatchedRows != 5 {
		t.Errorf("matched: expected 5, got %d", s.MatchedRows)
	}
	if s.UnmatchedRows != 1 {
		t.Errorf("unmatched: expected 1, got %d", s.UnmatchedRows)
	}
	if s.CoverageBlockedRows != 1 {
		t.Errorf("coverage: expected 1, got %d", s.CoverageBlockedRows)
	}
	if s.InstitutionalBlockedRows != 1 {
		t.Errorf("institutional: expected 1, got %d", s.InstitutionalBlockedRows)
	}
	if s.NoSupplierRows != 1 {
		t.Errorf("no supplier: expected 1, got %d", s.NoSupplierRows)
	}
	if s.BelowMinQtyRows != 1 {
		t.Errorf("below min qty: expected 1, got %d", s.BelowMinQtyRows)
	}
}
