package workflow

import (
	"fmt"
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OptimizeAllocations produces the 3 core allocation strategies (Options A, B, C).
func OptimizeAllocations(entries []MatchedProductEntry) []OptimizationOption {
	var options []OptimizationOption

	// Option A: Lowest Total Cost
	optA := buildLowestCostOption(entries)
	options = append(options, optA)

	// Option B: Fastest Delivery
	optB := buildFastestDeliveryOption(entries)
	options = append(options, optB)

	// Option C: Minimal Vendor Split
	optC := buildMinimalVendorOption(entries)
	options = append(options, optC)

	return options
}

func buildLowestCostOption(entries []MatchedProductEntry) OptimizationOption {
	allocMap := make(map[int64]*VendorShipmentDraft)
	totalCost := money.Zero
	totalOriginal := money.Zero
	itemCount := 0

	for _, entry := range entries {
		offer := findCheapestOffer(entry)
		if offer == nil {
			continue
		}
		itemCount++

		lineTotal := money.FromMinor(offer.FinalPrice.Minor() * int64(entry.RequestedLine.Quantity))
		origTotal := money.FromMinor(offer.Price.Minor() * int64(entry.RequestedLine.Quantity))
		totalCost, _ = totalCost.Add(lineTotal)
		totalOriginal, _ = totalOriginal.Add(origTotal)

		draft, exists := allocMap[offer.OrganizationID]
		if !exists {
			draft = &VendorShipmentDraft{
				OrganizationID:   offer.OrganizationID,
				OrganizationName: offer.OrganizationName,
				BranchID:         offer.BranchID,
				Subtotal:         money.Zero,
			}
			allocMap[offer.OrganizationID] = draft
		}

		draft.Lines = append(draft.Lines, ShipmentLineDraft{
			ProductID:   offer.ProductID,
			ProductName: offer.ProductName,
			Quantity:    entry.RequestedLine.Quantity,
			UnitPrice:   offer.FinalPrice,
			Discount:    offer.Discount,
			TotalPrice:  lineTotal,
		})
		draft.Subtotal, _ = draft.Subtotal.Add(lineTotal)
	}

	savings, _ := totalOriginal.Sub(totalCost)
	var drafts []VendorShipmentDraft
	for _, d := range allocMap {
		drafts = append(drafts, *d)
	}

	return OptimizationOption{
		Key:               "lowest_cost",
		Title:             "خيار السلة الأوفر (أقل تكلفة إجمالية)",
		Description:       fmt.Sprintf("توزيع الأصناف على %d موردين لتحقيق أعلى خصم وأقل سعر شراء.", len(drafts)),
		TotalItems:        itemCount,
		TotalCost:         totalCost,
		TotalSavings:      savings,
		VendorCount:       len(drafts),
		EstimatedDays:     2,
		VendorAllocations: drafts,
	}
}

func buildFastestDeliveryOption(entries []MatchedProductEntry) OptimizationOption {
	allocMap := make(map[int64]*VendorShipmentDraft)
	totalCost := money.Zero
	totalOriginal := money.Zero
	itemCount := 0

	for _, entry := range entries {
		offer := findFastestOffer(entry)
		if offer == nil {
			continue
		}
		itemCount++

		lineTotal := money.FromMinor(offer.FinalPrice.Minor() * int64(entry.RequestedLine.Quantity))
		origTotal := money.FromMinor(offer.Price.Minor() * int64(entry.RequestedLine.Quantity))
		totalCost, _ = totalCost.Add(lineTotal)
		totalOriginal, _ = totalOriginal.Add(origTotal)

		draft, exists := allocMap[offer.OrganizationID]
		if !exists {
			draft = &VendorShipmentDraft{
				OrganizationID:   offer.OrganizationID,
				OrganizationName: offer.OrganizationName,
				BranchID:         offer.BranchID,
				Subtotal:         money.Zero,
			}
			allocMap[offer.OrganizationID] = draft
		}

		draft.Lines = append(draft.Lines, ShipmentLineDraft{
			ProductID:   offer.ProductID,
			ProductName: offer.ProductName,
			Quantity:    entry.RequestedLine.Quantity,
			UnitPrice:   offer.FinalPrice,
			Discount:    offer.Discount,
			TotalPrice:  lineTotal,
		})
		draft.Subtotal, _ = draft.Subtotal.Add(lineTotal)
	}

	savings, _ := totalOriginal.Sub(totalCost)
	var drafts []VendorShipmentDraft
	for _, d := range allocMap {
		drafts = append(drafts, *d)
	}

	return OptimizationOption{
		Key:               "fastest_delivery",
		Title:             "خيار التوريد الأسرع (أقرب مسافة تسليم)",
		Description:       fmt.Sprintf("توجيه الطلب للموردين الأقرب جغرافياً للتوصيل خلال يوم واحد."),
		TotalItems:        itemCount,
		TotalCost:         totalCost,
		TotalSavings:      savings,
		VendorCount:       len(drafts),
		EstimatedDays:     1,
		VendorAllocations: drafts,
	}
}

func buildMinimalVendorOption(entries []MatchedProductEntry) OptimizationOption {
	// Group available vendor counts
	vendorCounts := make(map[int64]int)
	for _, entry := range entries {
		seen := make(map[int64]bool)
		for _, o := range append(entry.ExactMatches, entry.SimilarMatches...) {
			if !seen[o.OrganizationID] {
				vendorCounts[o.OrganizationID]++
				seen[o.OrganizationID] = true
			}
		}
	}

	// Pick dominant vendor
	var dominantVendorID int64
	maxC := 0
	for vID, c := range vendorCounts {
		if c > maxC {
			maxC = c
			dominantVendorID = vID
		}
	}

	allocMap := make(map[int64]*VendorShipmentDraft)
	totalCost := money.Zero
	totalOriginal := money.Zero
	itemCount := 0

	for _, entry := range entries {
		var offer *MatchedVendorOffer
		for _, o := range entry.ExactMatches {
			if o.OrganizationID == dominantVendorID {
				temp := o
				offer = &temp
				break
			}
		}
		if offer == nil {
			offer = entry.BestOffer
		}
		if offer == nil {
			continue
		}
		itemCount++

		lineTotal := money.FromMinor(offer.FinalPrice.Minor() * int64(entry.RequestedLine.Quantity))
		origTotal := money.FromMinor(offer.Price.Minor() * int64(entry.RequestedLine.Quantity))
		totalCost, _ = totalCost.Add(lineTotal)
		totalOriginal, _ = totalOriginal.Add(origTotal)

		draft, exists := allocMap[offer.OrganizationID]
		if !exists {
			draft = &VendorShipmentDraft{
				OrganizationID:   offer.OrganizationID,
				OrganizationName: offer.OrganizationName,
				BranchID:         offer.BranchID,
				Subtotal:         money.Zero,
			}
			allocMap[offer.OrganizationID] = draft
		}

		draft.Lines = append(draft.Lines, ShipmentLineDraft{
			ProductID:   offer.ProductID,
			ProductName: offer.ProductName,
			Quantity:    entry.RequestedLine.Quantity,
			UnitPrice:   offer.FinalPrice,
			Discount:    offer.Discount,
			TotalPrice:  lineTotal,
		})
		draft.Subtotal, _ = draft.Subtotal.Add(lineTotal)
	}

	savings, _ := totalOriginal.Sub(totalCost)
	var drafts []VendorShipmentDraft
	for _, d := range allocMap {
		drafts = append(drafts, *d)
	}

	return OptimizationOption{
		Key:               "minimal_vendors",
		Title:             "خيار تجميع الموردين (أقل عدد شحنات)",
		Description:       fmt.Sprintf("تجميع الطلبيات في %d موردين لتقليل مصاريف وفواتير الشحن والتسليم.", len(drafts)),
		TotalItems:        itemCount,
		TotalCost:         totalCost,
		TotalSavings:      savings,
		VendorCount:       len(drafts),
		EstimatedDays:     2,
		VendorAllocations: drafts,
	}
}

func findCheapestOffer(entry MatchedProductEntry) *MatchedVendorOffer {
	all := append(entry.ExactMatches, entry.SimilarMatches...)
	if len(all) == 0 {
		return entry.BestOffer
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].FinalPrice.Minor() < all[j].FinalPrice.Minor()
	})
	return &all[0]
}

func findFastestOffer(entry MatchedProductEntry) *MatchedVendorOffer {
	all := append(entry.ExactMatches, entry.SimilarMatches...)
	if len(all) == 0 {
		return entry.BestOffer
	}
	sort.SliceStable(all, func(i, j int) bool {
		d1 := 9999.0
		if all[i].DistanceKm != nil {
			d1 = *all[i].DistanceKm
		}
		d2 := 9999.0
		if all[j].DistanceKm != nil {
			d2 = *all[j].DistanceKm
		}
		return d1 < d2
	})
	return &all[0]
}
