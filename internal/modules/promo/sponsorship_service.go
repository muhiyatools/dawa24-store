package promo

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// WalletDebiter is injected from the composition root so the promo module can
// charge a vendor's wallet for a sponsorship package purchase without
// importing the billing module (ADR 0002). It debits amount from the org's
// wallet and returns the wallet transaction reference.
type WalletDebiter func(ctx context.Context, orgID int64, amount money.Amount, description string) (paymentID *int64, err error)

// SetWalletDebiter installs the wallet payment hook.
func (s *Service) SetWalletDebiter(fn WalletDebiter) { s.walletDebit = fn }

// PurchaseSponsorshipPackage buys a sponsorship package for the vendor's
// organization, charging their wallet and creating a purchase record with the
// package's credits. Returns the purchase with its remaining credits.
func (s *Service) PurchaseSponsorshipPackage(ctx context.Context, packageID int64, autoRenew bool, billingCycle string) (*SponsorshipPurchase, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	pkg, err := s.repo.GetPackageByID(ctx, packageID)
	if err != nil {
		return nil, err
	}
	if !pkg.IsActive {
		return nil, apperr.Validation("package.inactive", "هذه الباقة غير متاحة حالياً.", nil)
	}

	cost := pkg.Price
	if billingCycle != "annual" && billingCycle != "monthly" {
		billingCycle = "monthly"
	}
	durationDays := pkg.DurationDays
	if durationDays <= 0 {
		durationDays = 30
	}

	// Charge wallet if the package has a price and a debiter is wired.
	var sourceID *int64
	if s.walletDebit != nil && cost.IsPositive() {
		desc := "شراء باقة رعاية: " + pkg.Name.Get("ar")
		txID, err := s.walletDebit(ctx, orgID, cost, desc)
		if err != nil {
			return nil, err
		}
		sourceID = txID
	}

	now := time.Now().UTC()
	purchase := &SponsorshipPurchase{
		OrganizationID: orgID,
		PackageID:      packageID,
		CreditsTotal:   pkg.Credits,
		CreditsUsed:    0,
		StartsAt:       now,
		ExpiresAt:      now.Add(time.Duration(durationDays) * 24 * time.Hour),
		Status:         PurchaseActive,
		AutoRenew:      autoRenew,
		BillingCycle:   billingCycle,
		Amount:         cost,
		PaymentID:      nil,
		SourceSystem:   "wallet_checkout",
		SourceID:       sourceID,
	}

	if err := s.repo.CreateSponsorshipPurchase(ctx, purchase); err != nil {
		return nil, err
	}
	purchase.CreditsRemaining = purchase.CreditsRemainingInt()

	s.log.InfoContext(ctx, "sponsorship package purchased", "org_id", orgID, "package_id", packageID, "credits", pkg.Credits, "cost", cost.String())
	return purchase, nil
}

// ListSponsorshipPurchases returns all purchases for the active tenant.
func (s *Service) ListSponsorshipPurchases(ctx context.Context) ([]*SponsorshipPurchase, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	purchases, err := s.repo.ListSponsorshipPurchasesByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, p := range purchases {
		if p != nil {
			p.CreditsRemaining = p.CreditsRemainingInt()
		}
	}
	return purchases, nil
}

// ListActiveSponsorshipPurchases returns only active purchases.
func (s *Service) ListActiveSponsorshipPurchases(ctx context.Context) ([]*SponsorshipPurchase, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	purchases, err := s.repo.ListActiveSponsorshipPurchasesByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, p := range purchases {
		if p != nil {
			p.CreditsRemaining = p.CreditsRemainingInt()
		}
	}
	return purchases, nil
}

// SubmitSponsorshipRequest creates a sponsorship request for a product or
// offer, consuming one credit from the vendor's active purchase. The request
// starts in pending status and must be admin-approved before it becomes active.
func (s *Service) SubmitSponsorshipRequest(ctx context.Context, itemType SponsorshipItemType, itemID, packageID int64) (*SponsorshipRequest, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	pkg, err := s.repo.GetPackageByID(ctx, packageID)
	if err != nil {
		return nil, err
	}

	// Find an active purchase with remaining credits for this package.
	purchases, err := s.repo.ListActiveSponsorshipPurchasesByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var purchase *SponsorshipPurchase
	for _, p := range purchases {
		if p != nil && p.PackageID == packageID && p.CreditsRemainingInt() > 0 {
			purchase = p
			break
		}
	}
	if purchase == nil {
		return nil, apperr.Conflict("sponsorship.no_credits", "لا توجد رعايات متاحة في هذه الباقة. يرجى شراء باقة جديدة أو انتظار موافقة الطلبات السابقة.")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(pkg.DurationDays) * 24 * time.Hour)
	if expiresAt.After(purchase.ExpiresAt) {
		expiresAt = purchase.ExpiresAt
	}

	sr := &SponsorshipRequest{
		OrganizationID: orgID,
		PurchaseID:      &purchase.ID,
		PackageID:       packageID,
		ItemType:        itemType,
		ItemID:          itemID,
		CreditsUsed:     1,
		AdminStatus:     AdminPending,
		Status:          SRSPending,
		StartsAt:        now,
		ExpiresAt:       expiresAt,
	}

	if err := s.repo.CreateSponsorshipRequest(ctx, sr); err != nil {
		return nil, err
	}

	// Reserve the credit immediately so concurrent submissions don't overspend.
	// The credit is refunded if the admin rejects the request (in the admin
	// activation path the credit was already reserved at submission time, so
	// rejection releases it).
	_ = s.repo.IncrementSponsorshipPurchaseCreditsUsed(ctx, purchase.ID, 1)

	s.log.InfoContext(ctx, "sponsorship request submitted", "request_id", sr.ID, "item_type", string(itemType), "item_id", itemID, "package_id", packageID)
	return sr, nil
}

// ListSponsorshipRequestsByOrg returns the vendor's sponsorship requests.
func (s *Service) ListSponsorshipRequestsByOrg(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	return s.repo.ListSponsorshipRequestsByOrg(ctx, orgID, limit, offset)
}

// GetSponsorshipRequestByID retrieves a request by ID (tenant-scoped).
func (s *Service) GetSponsorshipRequestByID(ctx context.Context, id int64) (*SponsorshipRequest, error) {
	sr, err := s.repo.GetSponsorshipRequestByID(ctx, id)
	if err != nil {
		return nil, err
	}
	orgID, ok := database.TenantFrom(ctx)
	if ok && sr.OrganizationID != orgID {
		return nil, apperr.NotFound("sponsorship_request")
	}
	return sr, nil
}

// CancelSponsorshipRequest lets a vendor cancel their own pending request.
func (s *Service) CancelSponsorshipRequest(ctx context.Context, id int64) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	return s.repo.CancelSponsorshipRequest(ctx, id, orgID)
}

// AdminListSponsorshipRequests returns all requests for admin moderation.
func (s *Service) AdminListSponsorshipRequests(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, error) {
	return s.repo.ListAllSponsorshipRequests(database.AsSystem(ctx), limit, offset)
}

// AdminApproveSponsorshipRequest activates a pending request.
func (s *Service) AdminApproveSponsorshipRequest(ctx context.Context, id int64, notes string) (*SponsorshipRequest, error) {
	reviewerID, _ := authctx.UserID(ctx)
	sysCtx := database.AsSystem(ctx)

	sr, err := s.repo.GetSponsorshipRequestByID(sysCtx, id)
	if err != nil {
		return nil, err
	}
	if sr.AdminStatus != "pending" {
		return nil, apperr.Conflict("sponsorship.already_reviewed", "تمت مراجعة هذا الطلب مسبقاً.")
	}

	activated, err := s.repo.ActivateSponsorshipRequest(sysCtx, id, reviewerID)
	if err != nil {
		return nil, err
	}
	if notes != "" {
		_ = s.repo.UpdateSponsorshipRequestAdminStatus(sysCtx, id, AdminApproved, notes, reviewerID)
	}
	s.log.InfoContext(ctx, "sponsorship request approved", "request_id", id, "reviewer_id", reviewerID)
	return activated, nil
}

// AdminRejectSponsorshipRequest rejects a pending request and refunds the credit.
func (s *Service) AdminRejectSponsorshipRequest(ctx context.Context, id int64, notes string) error {
	reviewerID, _ := authctx.UserID(ctx)
	sysCtx := database.AsSystem(ctx)

	sr, err := s.repo.GetSponsorshipRequestByID(sysCtx, id)
	if err != nil {
		return err
	}
	if sr.AdminStatus != "pending" {
		return apperr.Conflict("sponsorship.already_reviewed", "تمت مراجعة هذا الطلب مسبقاً.")
	}

	if err := s.repo.UpdateSponsorshipRequestAdminStatus(sysCtx, id, AdminRejected, notes, reviewerID); err != nil {
		return err
	}

	// Refund the reserved credit if a purchase was linked.
	if sr.PurchaseID != nil {
		_ = s.repo.IncrementSponsorshipPurchaseCreditsUsed(sysCtx, *sr.PurchaseID, -sr.CreditsUsed)
	}
	s.log.InfoContext(ctx, "sponsorship request rejected", "request_id", id, "reviewer_id", reviewerID)
	return nil
}

// RankedSponsorshipsForProducts returns sponsored rankings for product IDs.
func (s *Service) RankedSponsorshipsForProducts(ctx context.Context, productIDs []int64) ([]*RankedSponsorship, error) {
	return s.repo.RankedSponsorshipsForProducts(database.AsSystem(ctx), productIDs)
}

// RankedSponsorshipsForOffers returns sponsored rankings for offer IDs.
func (s *Service) RankedSponsorshipsForOffers(ctx context.Context, offerIDs []int64) ([]*RankedSponsorship, error) {
	return s.repo.RankedSponsorshipsForOffers(database.AsSystem(ctx), offerIDs)
}

// IsSponsored returns the highest-tier sponsorship for a single item.
func (s *Service) IsSponsored(ctx context.Context, itemType SponsorshipItemType, itemID int64) (*RankedSponsorship, error) {
	return s.repo.IsSponsored(database.AsSystem(ctx), itemType, itemID)
}

// ExpireSponsorships runs the expiry sweep for purchases and requests.
func (s *Service) ExpireSponsorships(ctx context.Context) (int64, error) {
	n1, err := s.repo.ExpireSponsorshipPurchases(ctx)
	if err != nil {
		return 0, err
	}
	n2, err := s.repo.ExpireSponsorshipRequests(ctx)
	if err != nil {
		return 0, err
	}
	return n1 + n2, nil
}
