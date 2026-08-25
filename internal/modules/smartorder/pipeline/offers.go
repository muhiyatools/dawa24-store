package pipeline

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Supplier resolution.
//
// One query loads every vendor variant of every matched product in the file.
// Coverage and Corporate Operations are then applied per candidate in memory,
// because both are decisions about a (vendor, buyer-branch, moment) triple that
// no single SQL predicate expresses cleanly — and because evaluating them in Go
// keeps the rules in the services that already own them.

// CoverageGate answers whether a vendor can deliver to a point right now.
//
// An interface rather than a direct dependency on the workflow module: modules
// must not import each other, and it also makes the "window closed" case
// trivially testable without a database.
type CoverageGate interface {
	Serves(ctx context.Context, vendorOrgID int64, day time.Weekday, lat, lng float64) (bool, int, error)
}

// InstitutionalGate answers whether a buyer may see a restricted product.
//
// Simple mode, matching what ordinary catalogue browsing does: a product with no
// restriction is visible to everyone. Smart ordering must never show a buyer
// something they could not have found by browsing.
type InstitutionalGate interface {
	Visible(ctx context.Context, buyerOrgID int64, workIDs []int64) (bool, error)
}

// BranchLocation is where the order is going.
type BranchLocation struct {
	BranchID int64
	Lat      float64
	Lng      float64
	HasCoord bool
}

// Supplier resolves and selects vendors for matched lines.
type Supplier struct {
	repo          smartorder.Repository
	coverage      CoverageGate
	institutional InstitutionalGate
	cfg           *smartorder.Config
	branch        BranchLocation
	now           func() time.Time
}

// NewSupplier constructs the supplier stage.
func NewSupplier(repo smartorder.Repository, cov CoverageGate, inst InstitutionalGate,
	cfg *smartorder.Config, branch BranchLocation) *Supplier {
	return &Supplier{
		repo: repo, coverage: cov, institutional: inst,
		cfg: cfg, branch: branch, now: time.Now,
	}
}

// Resolve loads offers for every matched line, evaluates eligibility, selects a
// supplier, and returns the running order total.
func (s *Supplier) Resolve(ctx context.Context, lines []*smartorder.Line) (money.Amount, error) {
	productIDs := matchedProductIDs(lines)
	if len(productIDs) == 0 {
		return money.Amount{}, nil
	}

	// One query for the whole file.
	offers, err := s.repo.LoadOffers(ctx, s.cfg.OrganizationID, productIDs)
	if err != nil {
		return money.Amount{}, err
	}
	byProduct := make(map[int64][]smartorder.Offer, len(productIDs))
	for _, o := range offers {
		byProduct[o.ProductID] = append(byProduct[o.ProductID], o)
	}

	// Coverage and institutional verdicts are cached per vendor: a file with ten
	// thousand lines typically touches a few dozen vendors, and asking the same
	// question once per line would undo the batching everywhere else.
	covCache := make(map[int64]coverageVerdict)
	instCache := make(map[int64]bool)

	total := money.Amount{}
	var selections []*smartorder.Selection

	for _, l := range lines {
		if !l.Matched() {
			l.Outcome = smartorder.OutcomeUnmatched
			continue
		}
		if l.EffectiveQty <= 0 {
			l.Outcome = smartorder.OutcomeZeroQty
			continue
		}

		candidates, err := s.buildCandidates(ctx, l, byProduct[*l.MatchedProductID], covCache, instCache)
		if err != nil {
			return money.Amount{}, err
		}
		if err := s.repo.ReplaceCandidates(ctx, l.ID, candidates); err != nil {
			return money.Amount{}, err
		}

		outcome, reason := smartorder.OutcomeFor(true, l.EffectiveQty, candidates)
		l.Outcome = outcome
		l.OutcomeReason = string(reason)
		if outcome != smartorder.OutcomeOrdered {
			continue
		}

		// Candidates were written without ids; read them back so the selection
		// references rows that exist.
		stored, err := s.repo.ListCandidates(ctx, s.cfg.OrganizationID, l.ID)
		if err != nil {
			return money.Amount{}, err
		}
		sel, ok := smartorder.Select(s.cfg, l.ID, stored)
		if !ok {
			l.Outcome = smartorder.OutcomeNoSupplier
			continue
		}
		chosen := findCandidate(stored, sel.CandidateID)
		if chosen != nil {
			net, err := smartorder.LineNet(chosen.NetUnitPrice, l.EffectiveQty)
			if err != nil {
				return money.Amount{}, err
			}
			sel.LineNet = net
			if total, err = total.Add(net); err != nil {
				return money.Amount{}, err
			}
		}
		selections = append(selections, sel)
	}

	if err := s.repo.UpsertSelections(ctx, selections); err != nil {
		return money.Amount{}, err
	}
	return total, nil
}

type coverageVerdict struct {
	covered  bool
	distance int
}

// buildCandidates turns raw offers into evaluated candidates for one line.
func (s *Supplier) buildCandidates(ctx context.Context, l *smartorder.Line, offers []smartorder.Offer,
	covCache map[int64]coverageVerdict, instCache map[int64]bool) ([]smartorder.Candidate, error) {

	out := make([]smartorder.Candidate, 0, len(offers))
	weekday := s.now().Weekday()

	for _, o := range offers {
		c := smartorder.Candidate{
			LineID:         l.ID,
			OrganizationID: s.cfg.OrganizationID,
			VendorOrgID:    o.VendorOrgID,
			VariantID:      o.VariantID,
			BranchID:       o.BranchID,
			Price:          money.FromMinor(o.PriceMinor),
			DiscountBps:    o.DiscountBps,
			Unit:           o.Unit,
			MinOrderQty:    o.MinOrderQty,
			StockQty:       o.StockQty,
			IsFollowed:     o.IsFollowed,
		}
		// Net after discount, in minor units. The discount is basis points, so
		// the arithmetic stays exact — no float touches a price.
		discount := c.Price.ApplyPercent(o.DiscountBps)
		net, err := c.Price.Sub(discount)
		if err != nil {
			return nil, err
		}
		c.NetUnitPrice = net

		visible, ok := instCache[o.ProductID]
		if !ok {
			visible, err = s.institutional.Visible(ctx, s.cfg.OrganizationID, o.InstitutionalWorkIDs)
			if err != nil {
				return nil, err
			}
			instCache[o.ProductID] = visible
		}

		verdict, cached := covCache[o.VendorOrgID]
		if !cached {
			// A branch with no coordinates cannot be tested against a radius.
			// Treating that as "covered" matches how the rest of the platform
			// behaves when coverage data is absent, and refusing every supplier
			// because an address is incomplete would be worse.
			if !s.branch.HasCoord {
				verdict = coverageVerdict{covered: true}
			} else {
				covered, distance, err := s.coverage.Serves(ctx, o.VendorOrgID, weekday, s.branch.Lat, s.branch.Lng)
				if err != nil {
					return nil, err
				}
				verdict = coverageVerdict{covered: covered, distance: distance}
			}
			covCache[o.VendorOrgID] = verdict
		}
		if verdict.covered && verdict.distance > 0 {
			d := verdict.distance
			c.CoverageDistanceM = &d
		}

		eligible, reason := smartorder.Evaluate(smartorder.OfferCheck{
			BuyerOrgID:             s.cfg.OrganizationID,
			VendorOrgID:            o.VendorOrgID,
			ProductActive:          o.ProductActive && o.VendorActive,
			InstitutionallyVisible: visible,
			Covered:                verdict.covered,
			StockQty:               o.StockQty,
			RequestedQty:           l.EffectiveQty,
			MinOrderQty:            o.MinOrderQty,
		})
		c.Eligible = eligible
		c.IneligibleReason = reason
		out = append(out, c)
	}
	return out, nil
}

func matchedProductIDs(lines []*smartorder.Line) []int64 {
	seen := make(map[int64]bool)
	var out []int64
	for _, l := range lines {
		if !l.Matched() {
			continue
		}
		id := *l.MatchedProductID
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func findCandidate(candidates []smartorder.Candidate, id int64) *smartorder.Candidate {
	for i := range candidates {
		if candidates[i].ID == id {
			return &candidates[i]
		}
	}
	return nil
}
