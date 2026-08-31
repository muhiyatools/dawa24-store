package commerce

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Availability is the single source of truth for "may this pharmacy buy this
// quantity of this variant from this supplier, right now?".
//
// Before this existed, AddToCartSubmit made the decision inline and got it
// wrong five ways: a missing vendor id silently became organization 1, a variant
// with StockQty == 0 skipped validation entirely because the check was guarded
// by StockQty > 0, an over-stock quantity was silently clamped instead of
// refused, the stock lookup's error was swallowed, and nothing checked coverage
// or branch eligibility at all. Every surface now calls CheckAvailability so
// those rules cannot drift apart again.

// Reason is a machine-readable refusal code. The UI maps it to a message; tests
// assert on it rather than on Arabic text.
type Reason string

const (
	ReasonOK                Reason = ""
	ReasonVendorInvalid     Reason = "vendor_invalid"
	ReasonVendorUnapproved  Reason = "vendor_unapproved"
	ReasonVariantInvalid    Reason = "variant_invalid"
	ReasonVariantInactive   Reason = "variant_inactive"
	ReasonWrongVendor       Reason = "wrong_vendor"
	ReasonOutOfStock        Reason = "out_of_stock"
	ReasonInsufficientStock Reason = "insufficient_stock"
	ReasonBelowMinimum      Reason = "below_minimum"
	ReasonBranchInvalid     Reason = "branch_invalid"
	ReasonBranchNotOwned    Reason = "branch_not_owned"
	ReasonBranchNoLocation  Reason = "branch_no_location"
	ReasonNotCovered        Reason = "not_covered"
	ReasonQuantityInvalid   Reason = "quantity_invalid"
)

// VariantAvailability is the slice of a catalog variant that the rule needs.
type VariantAvailability struct {
	ID             int64
	OrganizationID int64
	StockQty       int
	MinOrderQty    int
	Active         bool
}

// VendorAvailability is the slice of a supplier organization that the rule needs.
type VendorAvailability struct {
	ID       int64
	IsVendor bool
	Approved bool
}

// BranchAvailability is the slice of a pharmacy branch that the rule needs.
type BranchAvailability struct {
	ID             int64
	OrganizationID int64
	Latitude       *float64
	Longitude      *float64
}

// AvailabilityProbe is what commerce needs from the catalog, org and workflow
// modules. Modules must not import each other (ADR 0002), so the composition
// root in cmd/server/routes.go implements this over the real services — the
// same shape as RequiredDocsChecker.
type AvailabilityProbe interface {
	Variant(ctx context.Context, variantID int64) (VariantAvailability, error)
	Vendor(ctx context.Context, orgID int64) (VendorAvailability, error)
	CustomerBranch(ctx context.Context, branchID int64) (BranchAvailability, error)
	VendorCovers(ctx context.Context, vendorOrgID int64, lat, lon float64, day time.Weekday) (bool, error)
}

// AvailabilityRequest describes one prospective purchase line.
type AvailabilityRequest struct {
	VariantID        int64
	VendorOrgID      int64
	CustomerOrgID    int64
	CustomerBranchID int64
	Quantity         int
	When             time.Time // decides which weekday's coverage applies
}

// AvailabilityResult is the verdict. MaxQuantity is filled whenever the caller
// asked for more than is available, so the UI can say how many there actually
// are instead of silently reducing the number the pharmacy typed.
type AvailabilityResult struct {
	Allowed     bool
	MaxQuantity int
	Reason      Reason
	MessageAr   string
	MessageEn   string
}

func denied(reason Reason, max int, ar, en string) AvailabilityResult {
	return AvailabilityResult{Allowed: false, MaxQuantity: max, Reason: reason, MessageAr: ar, MessageEn: en}
}

// CheckAvailability runs every purchase precondition in order and returns the
// first failure. It never partially succeeds and never silently adjusts the
// requested quantity.
func (s *Service) CheckAvailability(ctx context.Context, req AvailabilityRequest) (AvailabilityResult, error) {
	if s.availability == nil {
		// Fail closed. A missing probe means we cannot prove the line is
		// buyable, and guessing is how the old code let anything through.
		return denied(ReasonVariantInvalid, 0,
			i18n.T("ar", "err.avail_check_failed"),
			"Availability cannot be verified right now."), nil
	}

	if req.Quantity <= 0 {
		return denied(ReasonQuantityInvalid, 0,
			i18n.T("ar", "err.qty_must_be_positive"),
			"Quantity must be greater than zero."), nil
	}

	// 1. The supplier must be a real, approved vendor. No default.
	if req.VendorOrgID <= 0 {
		return denied(ReasonVendorInvalid, 0,
			i18n.T("ar", "err.supplier_not_specified"),
			"No supplier was specified for this item."), nil
	}
	vendor, err := s.availability.Vendor(ctx, req.VendorOrgID)
	if err != nil {
		return AvailabilityResult{}, fmt.Errorf("availability: load vendor %d: %w", req.VendorOrgID, err)
	}
	if vendor.ID == 0 || !vendor.IsVendor {
		return denied(ReasonVendorInvalid, 0,
			i18n.T("ar", "err.supplier_invalid"),
			"The specified supplier is not valid."), nil
	}
	if !vendor.Approved {
		return denied(ReasonVendorUnapproved, 0,
			i18n.T("ar", "err.supplier_invalid"),
			"This supplier is not currently approved."), nil
	}

	// 2. The variant must exist, be active, and belong to that supplier.
	if req.VariantID <= 0 {
		return denied(ReasonVariantInvalid, 0,
			i18n.TDefault("w4_mod.w4str_126_126"),
			"No product variant was specified."), nil
	}
	variant, err := s.availability.Variant(ctx, req.VariantID)
	if err != nil {
		return AvailabilityResult{}, fmt.Errorf("availability: load variant %d: %w", req.VariantID, err)
	}
	if variant.ID == 0 {
		return denied(ReasonVariantInvalid, 0,
			i18n.TDefault("w4_mod.w4str_127_127"),
			"The requested product variant does not exist."), nil
	}
	if !variant.Active {
		return denied(ReasonVariantInactive, 0,
			i18n.TDefault("w4_mod.w4str_128_128"),
			"This product is not currently available."), nil
	}
	if variant.OrganizationID != req.VendorOrgID {
		return denied(ReasonWrongVendor, 0,
			i18n.TDefault("w4_mod.w4str_129_129"),
			"This product does not belong to the specified supplier."), nil
	}

	// 3. Stock. Zero stock is a refusal, not a skipped check.
	if variant.StockQty <= 0 {
		return denied(ReasonOutOfStock, 0,
			i18n.TDefault("w4_mod.w4str_130_130"),
			"This item is out of stock at the supplier."), nil
	}
	if req.Quantity > variant.StockQty {
		return denied(ReasonInsufficientStock, variant.StockQty,
			fmt.Sprintf(i18n.TDefault("w4_mod.d_131"), variant.StockQty),
			fmt.Sprintf("Only %d available from this supplier.", variant.StockQty)), nil
	}
	if variant.MinOrderQty > 0 && req.Quantity < variant.MinOrderQty {
		return denied(ReasonBelowMinimum, variant.StockQty,
			fmt.Sprintf(i18n.TDefault("w4_mod.d_132"), variant.MinOrderQty),
			fmt.Sprintf("Minimum order quantity for this item is %d.", variant.MinOrderQty)), nil
	}

	// 4. The delivery branch must belong to the buying pharmacy.
	if req.CustomerBranchID <= 0 {
		return denied(ReasonBranchInvalid, variant.StockQty,
			i18n.TDefault("w4_mod.w4str_133_133"),
			"Select a receiving branch first."), nil
	}
	branch, err := s.availability.CustomerBranch(ctx, req.CustomerBranchID)
	if err != nil {
		return AvailabilityResult{}, fmt.Errorf("availability: load branch %d: %w", req.CustomerBranchID, err)
	}
	if branch.ID == 0 {
		return denied(ReasonBranchInvalid, variant.StockQty,
			i18n.TDefault("w4_mod.w4str_134_134"),
			"The selected receiving branch does not exist."), nil
	}
	if req.CustomerOrgID > 0 && branch.OrganizationID != req.CustomerOrgID {
		return denied(ReasonBranchNotOwned, variant.StockQty,
			i18n.TDefault("w4_mod.w4str_135_135"),
			"The selected branch does not belong to your organization."), nil
	}

	// 5. The supplier must cover that branch's location on the relevant weekday.
	if branch.Latitude == nil || branch.Longitude == nil {
		return denied(ReasonBranchNoLocation, variant.StockQty,
			i18n.TDefault("w4_mod.w4str_136_136"),
			"This branch has no map location yet; set one to order."), nil
	}
	when := req.When
	if when.IsZero() {
		when = time.Now()
	}
	covered, err := s.availability.VendorCovers(ctx, req.VendorOrgID, *branch.Latitude, *branch.Longitude, when.Weekday())
	if err != nil {
		return AvailabilityResult{}, fmt.Errorf("availability: coverage for vendor %d: %w", req.VendorOrgID, err)
	}
	if !covered {
		return denied(ReasonNotCovered, variant.StockQty,
			i18n.TDefault("w4_mod.w4str_137_137"),
			"This supplier does not cover your branch's location on this day."), nil
	}

	return AvailabilityResult{Allowed: true, MaxQuantity: variant.StockQty, Reason: ReasonOK}, nil
}

// revalidateCheckoutLines re-runs the availability rule over every line being
// ordered. It exists because availability at add-to-cart time is not a promise:
// a cart can sit open for hours while another pharmacy takes the last unit or
// the supplier drops the delivery day.
//
// A refusal fails the whole checkout with the line's own reason rather than
// silently dropping that line — a pharmacy must not discover at delivery that
// part of its order quietly vanished.
func (s *Service) revalidateCheckoutLines(ctx context.Context, input CheckoutInput) error {
	if s.availability == nil {
		return nil // no probe wired: CheckAvailability already fails closed at the surfaces
	}
	branchID := int64(0)
	if input.BranchID != nil {
		branchID = *input.BranchID
	}
	for _, item := range input.Items {
		if item.ProductVariantID == nil || *item.ProductVariantID <= 0 {
			continue // a line with no variant carries no stock to check
		}
		res, err := s.CheckAvailability(ctx, AvailabilityRequest{
			VariantID:        *item.ProductVariantID,
			VendorOrgID:      item.VendorOrgID,
			CustomerOrgID:    input.CustomerOrgID,
			CustomerBranchID: branchID,
			Quantity:         item.Quantity,
			When:             time.Now(),
		})
		if err != nil {
			return err
		}
		if !res.Allowed {
			msg := res.MessageAr
			if msg == "" {
				msg = res.MessageEn
			}
			return apperr.Validation(
				"checkout.line_unavailable."+string(res.Reason),
				msg,
				map[string]string{"product": msg},
			)
		}
	}
	return nil
}
