package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// Corporate Operations at the supplier stage.
//
// The gate used to be asked "does the buyer organisation hold one of this
// PRODUCT's institutional works?" — a different question from the one
// commerce.CheckAvailability asks at checkout, which is whether the buyer's
// BRANCH is connected to a branch of the supplier. Because the two disagreed, a
// run's review screen could show a line as orderable and checkout would then
// refuse the whole order at the last click.
//
// These tests pin the contract that makes the two agree: the gate is handed the
// branches, its answer is cached per supplier branch rather than per product,
// and a refusal lands on the line as institutional_blocked while the results are
// still being reviewed.

// recordingGate captures what the pipeline asks and answers from a fixture.
type recordingGate struct {
	calls   []smartorder.InstitutionalCheck
	allowed map[int64]bool // vendor org id -> visible
}

func (g *recordingGate) Visible(_ context.Context, c smartorder.InstitutionalCheck) (bool, error) {
	g.calls = append(g.calls, c)
	return g.allowed[c.VendorOrgID], nil
}

type stubCoverage struct{}

func (stubCoverage) Serves(context.Context, int64, time.Weekday, float64, float64) (bool, int, error) {
	return true, 100, nil
}

func TestInstitutionalGateReceivesBothBranches(t *testing.T) {
	vendorBranch := int64(68)
	gate := &recordingGate{allowed: map[int64]bool{187: true}}

	s := newTestSupplier(gate, 186, 65)
	line := matchedLine(1, 10, 5)

	offers := []smartorder.Offer{{
		ProductID: 10, VariantID: 900, VendorOrgID: 187,
		BranchID: &vendorBranch, VariantBranchID: &vendorBranch,
		PriceMinor: 5000, MinOrderQty: 1, StockQty: 50,
		VendorActive: true, ProductActive: true,
	}}

	if _, err := s.buildCandidates(context.Background(), line, offers,
		map[int64]coverageVerdict{}, map[instKey]bool{}); err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}

	if len(gate.calls) != 1 {
		t.Fatalf("expected one gate call, got %d", len(gate.calls))
	}
	got := gate.calls[0]
	if got.BuyerOrgID != 186 {
		t.Errorf("buyer org: got %d, want 186", got.BuyerOrgID)
	}
	if got.BuyerBranchID != 65 {
		t.Errorf("buyer branch: got %d, want 65 — without it the rule cannot be evaluated at all", got.BuyerBranchID)
	}
	if got.VendorOrgID != 187 {
		t.Errorf("vendor org: got %d, want 187", got.VendorOrgID)
	}
	if got.VendorBranchID == nil || *got.VendorBranchID != vendorBranch {
		t.Errorf("vendor branch: got %v, want %d", got.VendorBranchID, vendorBranch)
	}
	if got.VariantID != 900 {
		t.Errorf("variant: got %d, want 900", got.VariantID)
	}
}

// A variant that names no branch is satisfiable from any branch of the
// supplier, so the gate must be told there is no branch rather than being
// handed the fallback main-branch id the candidate ships from. Passing the
// fallback would refuse a supplier whose main branch lacks the works while
// another branch has them.
func TestOfferWithNoOwnBranchAsksAboutTheWholeSupplier(t *testing.T) {
	mainBranch := int64(67)
	gate := &recordingGate{allowed: map[int64]bool{187: true}}

	s := newTestSupplier(gate, 186, 65)
	offers := []smartorder.Offer{{
		ProductID: 10, VariantID: 900, VendorOrgID: 187,
		BranchID: &mainBranch, VariantBranchID: nil,
		PriceMinor: 5000, MinOrderQty: 1, StockQty: 50,
		VendorActive: true, ProductActive: true,
	}}

	if _, err := s.buildCandidates(context.Background(), matchedLine(1, 10, 5), offers,
		map[int64]coverageVerdict{}, map[instKey]bool{}); err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}
	if gate.calls[0].VendorBranchID != nil {
		t.Fatalf("an offer with no branch of its own must ask about every branch, got branch %d",
			*gate.calls[0].VendorBranchID)
	}
}

// The verdict is a fact about a supplier branch, not about a product. Caching
// it under the product id answered once and then applied the FIRST supplier's
// verdict to every other supplier of the same product.
func TestVerdictIsCachedPerSupplierBranchNotPerProduct(t *testing.T) {
	branchA, branchB := int64(68), int64(70)
	gate := &recordingGate{allowed: map[int64]bool{187: true, 188: false}}
	s := newTestSupplier(gate, 186, 65)
	cache := map[instKey]bool{}

	offers := []smartorder.Offer{
		{ProductID: 10, VariantID: 900, VendorOrgID: 187, BranchID: &branchA, VariantBranchID: &branchA,
			PriceMinor: 5000, MinOrderQty: 1, StockQty: 50, VendorActive: true, ProductActive: true},
		{ProductID: 10, VariantID: 901, VendorOrgID: 188, BranchID: &branchB, VariantBranchID: &branchB,
			PriceMinor: 4000, MinOrderQty: 1, StockQty: 50, VendorActive: true, ProductActive: true},
		// Same supplier and branch as the first: this one must be served from
		// the cache, not asked again.
		{ProductID: 11, VariantID: 902, VendorOrgID: 187, BranchID: &branchA, VariantBranchID: &branchA,
			PriceMinor: 5500, MinOrderQty: 1, StockQty: 50, VendorActive: true, ProductActive: true},
	}

	got, err := s.buildCandidates(context.Background(), matchedLine(1, 10, 5), offers,
		map[int64]coverageVerdict{}, cache)
	if err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}

	if len(gate.calls) != 2 {
		t.Fatalf("expected one question per supplier branch (2), got %d", len(gate.calls))
	}
	if !got[0].Eligible {
		t.Error("the connected supplier must stay eligible")
	}
	if got[1].Eligible {
		t.Error("the unconnected supplier must be refused")
	}
	if got[1].IneligibleReason != smartorder.ReasonInstitutional {
		t.Errorf("reason: got %q, want %q", got[1].IneligibleReason, smartorder.ReasonInstitutional)
	}
	if !got[2].Eligible {
		t.Error("the cached verdict for the same supplier branch must be reused, not inverted")
	}
}

// A line whose every supplier is institutionally blocked has to report that at
// the review step, in those words. "No supplier" would send the buyer looking
// for a sourcing problem that does not exist.
func TestBlockedLineReportsInstitutionalRatherThanNoSupplier(t *testing.T) {
	branch := int64(68)
	gate := &recordingGate{allowed: map[int64]bool{}} // nothing is connected
	s := newTestSupplier(gate, 186, 65)

	line := matchedLine(1, 10, 5)
	offers := []smartorder.Offer{{
		ProductID: 10, VariantID: 900, VendorOrgID: 187,
		BranchID: &branch, VariantBranchID: &branch,
		PriceMinor: 5000, MinOrderQty: 1, StockQty: 50,
		VendorActive: true, ProductActive: true,
	}}

	candidates, err := s.buildCandidates(context.Background(), line, offers,
		map[int64]coverageVerdict{}, map[instKey]bool{})
	if err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}

	outcome, _ := smartorder.OutcomeFor(true, line.EffectiveQty, candidates)
	if outcome != smartorder.OutcomeInstitutionalBlocked {
		t.Fatalf("outcome: got %q, want %q — the buyer must be told this is a Corporate Operations "+
			"restriction while reviewing, not at the last click", outcome, smartorder.OutcomeInstitutionalBlocked)
	}
}

// The degenerate gate is still the product-work intersection, for a deployment
// that has no org service to ask.
func TestSimpleGateStillReadsTheProductsOwnWorks(t *testing.T) {
	gate := smartorder.SimpleInstitutionalGate([]int64{2, 3})

	unrestricted, err := gate.Visible(context.Background(), smartorder.InstitutionalCheck{})
	if err != nil || !unrestricted {
		t.Fatalf("an unrestricted product must be visible: %v %v", unrestricted, err)
	}
	held, _ := gate.Visible(context.Background(), smartorder.InstitutionalCheck{ProductWorkIDs: []int64{3}})
	if !held {
		t.Error("a product restricted to a held work must be visible")
	}
	notHeld, _ := gate.Visible(context.Background(), smartorder.InstitutionalCheck{ProductWorkIDs: []int64{9}})
	if notHeld {
		t.Error("a product restricted to a work the buyer does not hold must be refused")
	}
}

func newTestSupplier(gate InstitutionalGate, buyerOrgID, buyerBranchID int64) *Supplier {
	return NewSupplier(nil, stubCoverage{}, gate,
		&smartorder.Config{OrganizationID: buyerOrgID},
		BranchLocation{BranchID: buyerBranchID, Lat: 30.04, Lng: 31.23, HasCoord: true})
}

func matchedLine(id, productID int64, qty float64) *smartorder.Line {
	pid := productID
	return &smartorder.Line{ID: id, MatchedProductID: &pid, EffectiveQty: qty}
}
