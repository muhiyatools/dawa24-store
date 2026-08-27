package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	smartorderPG "github.com/muhiya/dawa24-store/internal/modules/smartorder/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// Wiring smart ordering.
//
// This is the composition root's job precisely because smartorder must not
// import workflow, org or commerce (AGENTS.md rule 5). Each dependency is
// adapted here into the narrow function type the module declares, so the module
// stays testable with closures and the modules stay independent of each other.

// wireSmartOrder assembles the feature and mounts it on the UI handler.
//
// Nil-safe: if anything it needs is missing the wizard reports itself
// unavailable rather than taking the rest of the customer surface down with it.
func wireSmartOrder(
	db *database.DB,
	uiHandler *ui.UIHandler,
	orgSvc *org.Service,
	wfCoverage *workflow.CoverageService,
	commSvc *commerce.Service,
	ai gateway.Client,
	log *slog.Logger,
) *smartorder.Service {
	if db == nil || uiHandler == nil {
		return nil
	}

	repo := smartorderPG.New(db)
	svc := smartorder.NewService(repo, log)

	// Process in this process. The River worker stays registered for
	// deployments that run it, and the two cannot collide: the runner only
	// claims a run that is still `queued`. See smartorder_runner.go.
	uiHandler.SetSmartOrder(svc, inlineSmartOrderRunner(db, orgSvc, ai, log))

	if commSvc != nil {
		uiHandler.SetFinalizer(smartorder.NewFinalizer(
			repo,
			placeSmartOrder(commSvc, orgSvc, log),
			&reverifier{wfCoverage: wfCoverage, orgSvc: orgSvc},
		))
	}
	return svc
}

// placeSmartOrder adapts commerce checkout.
//
// Smart orders go through the **same** checkout as the ordinary cart, so
// multi-vendor shipment partitioning, order numbering, status history and the
// documents gate are identical (FR-048). A separate order-creation path would
// drift from the one the rest of the platform uses, and the drift would only
// show up in an invoice.
func placeSmartOrder(commSvc *commerce.Service, orgSvc *org.Service, log *slog.Logger) smartorder.PlaceOrderFunc {
	return func(ctx context.Context, req smartorder.PlaceOrderRequest) (int64, error) {
		items := make([]commerce.CheckoutLineItem, 0, len(req.Lines))
		for _, l := range req.Lines {
			variantID := l.VariantID
			// The discount is carried as an amount per line rather than a
			// percentage, because that is what the order snapshot stores and it
			// keeps the arithmetic exact.
			gross, err := l.UnitPrice.MulInt(int64(l.Quantity))
			if err != nil {
				return 0, err
			}
			discount := money.Zero
			if gross.Minor() > l.LineNet.Minor() {
				if d, subErr := gross.Sub(l.LineNet); subErr == nil {
					discount = d
				}
			}
			raw := l.RawName
			if raw == "" {
				raw = "صنف دواء 24"
			}
			pName := i18n.New(raw, raw)
			items = append(items, commerce.CheckoutLineItem{
				VendorOrgID:      l.VendorOrgID,
				ProductVariantID: &variantID,
				ProductName:      pName,
				UnitPrice:        l.UnitPrice,
				Quantity:         int(l.Quantity),
				DiscountAmount:   discount,
				ListPrice:        l.UnitPrice,
			})
		}

		branchID := req.BranchID
		if branchID <= 0 && orgSvc != nil {
			branches, err := orgSvc.ListBranches(database.AsSystem(ctx), req.OrganizationID)
			if err == nil && len(branches) > 0 {
				for _, b := range branches {
					if b.IsMain {
						branchID = b.ID
						break
					}
				}
				if branchID <= 0 {
					branchID = branches[0].ID
				}
			}
		}

		log.InfoContext(ctx, "placing smart order",
			"run_id", req.SourceRunID, "lines", len(items), "branch_id", branchID, "total", req.Total.String())

		order, err := commSvc.Checkout(ctx, commerce.CheckoutInput{
			CustomerID:    req.UserID,
			CustomerOrgID: req.OrganizationID,
			BranchID:      &branchID,
			Items:         items,
			Notes:         "طلب ذكي",
		})
		if err != nil {
			return 0, err
		}
		return order.ID, nil
	}
}

// reverifier re-checks one candidate against the world as it is now.
type reverifier struct {
	wfCoverage *workflow.CoverageService
	orgSvc     *org.Service
}

// Recheck runs the same checks the pipeline ran, against current data.
//
// Coverage is the one that most often changes between generating an order and
// placing it: a buyer reviewing for ten minutes can cross the end of a delivery
// window without noticing, and the offer that was deliverable when the results
// rendered is not deliverable when they press the button.
func (rv *reverifier) Recheck(ctx context.Context, buyerOrgID, branchID int64,
	c smartorder.Candidate, qty float64) (bool, smartorder.IneligibleReason, error) {

	if branchID <= 0 && rv.orgSvc != nil {
		branches, err := rv.orgSvc.ListBranches(database.AsSystem(ctx), buyerOrgID)
		if err == nil && len(branches) > 0 {
			for _, b := range branches {
				if b.IsMain {
					branchID = b.ID
					break
				}
			}
			if branchID <= 0 {
				branchID = branches[0].ID
			}
		}
	}

	covered := true
	if rv.wfCoverage != nil && rv.orgSvc != nil && branchID > 0 {
		branch, err := rv.orgSvc.GetBranch(database.AsSystem(ctx), branchID)
		if err == nil && branch != nil {
			lat := branch.Latitude
			lon := branch.Longitude
			if lat == nil || lon == nil {
				defLat := 30.0444
				defLon := 31.2357
				lat = &defLat
				lon = &defLon
			}
			ok, _, err := rv.wfCoverage.ServesPoint(ctx, c.VendorOrgID, time.Now().Weekday(),
				workflow.Coord{Lat: *lat, Lon: *lon})
			if err == nil {
				covered = ok
			}
		}
	}

	// Product and vendor status are not re-read here: the candidate row was
	// written when the offer was known good, and Checkout re-validates both
	// before it creates anything. What this catches is the time-dependent pair —
	// coverage and stock — plus the minimum, which the buyer can change.
	ok, reason := smartorder.Evaluate(smartorder.OfferCheck{
		BuyerOrgID:             buyerOrgID,
		VendorOrgID:            c.VendorOrgID,
		ProductActive:          true,
		InstitutionallyVisible: true,
		Covered:                covered,
		StockQty:               c.StockQty,
		RequestedQty:           qty,
		MinOrderQty:            c.MinOrderQty,
	})
	return ok, reason, nil
}
