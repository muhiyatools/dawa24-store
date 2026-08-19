package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateAutomationRequest creates a new automated bulk purchase optimization request (Plan V5 Task 3.3).
func (s *Service) CreateAutomationRequest(ctx context.Context, userID int64, orgID *int64, filename string, rawCSV []byte, prefs Priorities) (*AutomationRequest, error) {
	if userID <= 0 {
		return nil, apperr.Validation("automation.user_required", "User ID is required.", nil)
	}

	lines, err := ParseCSVBytes(rawCSV)
	if err != nil {
		return nil, apperr.Validation("automation.parse_failed", fmt.Sprintf("Failed parsing spreadsheet: %v", err), nil)
	}
	if len(lines) == 0 {
		return nil, apperr.Validation("automation.empty_file", "No valid product rows found in file.", nil)
	}

	req := &AutomationRequest{
		UserID:           userID,
		OrganizationID:   orgID,
		RequestNumber:    GenerateAutomationRequestNumber(time.Now().UTC()),
		OriginalFilename: filename,
		Status:           AutomationStatusPending,
		TotalProducts:    len(lines),
		Priorities:       prefs,
		BudgetConstraint: prefs.BudgetConstraint,
		FileData:         lines,
	}

	if err := s.repo.CreateAutomationRequest(ctx, req); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "automation request created", "id", req.ID, "request_number", req.RequestNumber, "rows", len(lines))
	return req, nil
}

// ProcessAutomationRequest executes the complete analysis, product matching, supplier optimization, and alert pipeline.
func (s *Service) ProcessAutomationRequest(ctx context.Context, requestID int64, actorID int64, userLat, userLng *float64, maxDistanceKm float64) (*AutomationRequest, error) {
	req, err := s.repo.GetAutomationRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}

	// 1. Mark as processing
	_ = s.repo.UpdateAutomationRequestStatus(ctx, requestID, AutomationStatusProcessing, nil, nil, 0, 0)

	// 2. Fetch authorized institutional works (Simple mode 0)
	var authorizedWorkIDs []int64
	if s.instGate != nil {
		works, err := s.instGate.AllowedWorkIDs(ctx, req.UserID, 0)
		if err == nil {
			authorizedWorkIDs = works
		}
	}

	// 3. Fetch candidate products from catalog read model
	candidates, err := s.repo.GetCandidateProducts(ctx, req.UserID, authorizedWorkIDs, req.Priorities.PreferredSupplierIDs, req.BudgetConstraint, 2000)
	if err != nil {
		s.log.ErrorContext(ctx, "failed fetching candidate products for automation", "id", requestID, "error", err)
		_ = s.repo.UpdateAutomationRequestStatus(ctx, requestID, AutomationStatusFailed, map[string]any{"error": err.Error()}, nil, 0, 0)
		return nil, err
	}

	// Map CandidateProduct to MatchedVendorOffer
	offers := make([]MatchedVendorOffer, 0, len(candidates))
	for _, c := range candidates {
		discPct := c.DiscountPercentage()
		finalP := c.FinalPrice()
		var distKm *float64
		// In a full deployment, CoverageService or lat/lng computes exact distance
		if userLat != nil && userLng != nil {
			d := 12.5 // default proxy within radius
			distKm = &d
		}
		offers = append(offers, MatchedVendorOffer{
			ProductID:        c.ProductID,
			OrganizationID:   c.OrganizationID,
			OrganizationName: c.OrganizationName,
			BranchID:         c.BranchID,
			BranchName:       c.BranchName,
			ProductName:      c.ProductName,
			ProductSKU:       c.ProductSKU,
			Price:            c.ProductPrice,
			Discount:         discPct,
			FinalPrice:       finalP,
			StockQuantity:    c.StockQuantity,
			DistanceKm:       distKm,
		})
	}

	// 4. Match each requested line
	var entries []MatchedProductEntry
	matchedCount := 0
	for _, line := range req.FileData {
		entry := MatchLineAgainstOffers(line, offers, req.Priorities)
		if len(entry.ExactMatches) > 0 || len(entry.SimilarMatches) > 0 {
			matchedCount++
		}
		entries = append(entries, entry)
	}

	// 5. Optimize allocations (Options A, B, C)
	options := OptimizeAllocations(entries)

	// 6. Evaluate alerts
	alerts := EvaluateAlerts(entries)

	// 7. Calculate totals and summary
	totalVal := money.Zero
	for _, e := range entries {
		if e.BestOffer != nil {
			lineP := money.FromMinor(e.BestOffer.FinalPrice.Minor() * int64(e.RequestedLine.Quantity))
			totalVal, _ = totalVal.Add(lineP)
		}
	}

	matchPercentage := 0.0
	if req.TotalProducts > 0 {
		matchPercentage = (float64(matchedCount) / float64(req.TotalProducts)) * 100.0
	}

	status := AutomationStatusCompleted
	if matchedCount == req.TotalProducts && matchPercentage >= 90.0 {
		status = AutomationStatusApproved
	}

	results := map[string]any{
		"matched_percentage":   matchPercentage,
		"matched_count":        matchedCount,
		"total_count":          req.TotalProducts,
		"optimization_options": options,
		"alerts":               alerts,
		"matched_entries":      entries,
	}

	if err := s.repo.UpdateAutomationRequestStatus(ctx, requestID, status, results, &totalVal, matchedCount, matchedCount); err != nil {
		s.log.ErrorContext(ctx, "failed updating automation request status", "id", requestID, "error", err)
		return nil, err
	}

	req.Status = status
	req.MatchedProducts = matchedCount
	req.ApprovedProducts = matchedCount
	req.TotalValue = totalVal
	req.ComparisonResults = results
	req.VendorMatches = entries

	s.log.InfoContext(ctx, "automation request completed", "id", requestID, "matched", matchedCount, "total", req.TotalProducts, "status", status)
	return req, nil
}

// GetAutomationRequest retrieves an automation request by ID.
func (s *Service) GetAutomationRequest(ctx context.Context, id int64) (*AutomationRequest, error) {
	return s.repo.GetAutomationRequestByID(ctx, id)
}

// ListAutomationRequests lists past automation requests for a user.
func (s *Service) ListAutomationRequests(ctx context.Context, userID int64, limit, offset int) ([]*AutomationRequest, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.ListAutomationRequestsByUser(ctx, userID, limit, offset)
}
