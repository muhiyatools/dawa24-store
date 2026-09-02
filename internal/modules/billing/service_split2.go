package billing

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AssignDefaultSubscription guarantees an organization or user has an active basic subscription.
func (s *Service) AssignDefaultSubscription(ctx context.Context, userID int64, orgID *int64) (*Subscription, error) {
	if orgID != nil && *orgID > 0 {
		if sub, err := s.repo.GetActiveSubscriptionByOrg(ctx, *orgID); err == nil && sub != nil {
			return sub, nil
		}
	} else if userID > 0 {
		if sub, err := s.repo.GetActiveSubscription(ctx, userID); err == nil && sub != nil {
			return sub, nil
		}
	}

	plan, err := s.repo.GetDefaultPlan(ctx)
	if err != nil || plan == nil {
		plan, err = s.repo.GetPlanBySlug(ctx, "basic")
		if err != nil || plan == nil {
			return nil, fmt.Errorf("default plan not found: %w", err)
		}
	}

	now := time.Now().UTC()
	duration := time.Duration(plan.DurationDays) * 24 * time.Hour
	if duration <= 0 {
		duration = 3650 * 24 * time.Hour
	}

	sub := &Subscription{
		UserID:         userID,
		OrganizationID: orgID,
		PlanID:         plan.ID,
		Status:         SubActive,
		StartsAt:       now,
		ExpiresAt:      now.Add(duration),
		SourceSystem:   "registration_default",
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}
	s.syncAIPlan(ctx, orgID)
	s.log.InfoContext(ctx, "default subscription assigned", "user_id", userID, "org_id", orgID, "plan", plan.Slug)
	return sub, nil
}

// Subscribe grants a user a subscription to a plan tier.
func (s *Service) Subscribe(
	ctx context.Context,
	userID int64,
	orgID *int64,
	planSlug string,
	sourceSystem string,
	sourceID *int64,
) (*Subscription, error) {
	plan, err := s.repo.GetPlanBySlug(ctx, planSlug)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	duration := time.Duration(plan.DurationDays) * 24 * time.Hour
	if duration <= 0 {
		duration = 30 * 24 * time.Hour
	}

	sub := &Subscription{
		UserID:         userID,
		OrganizationID: orgID,
		PlanID:         plan.ID,
		Status:         SubActive,
		BillingCycle:   "monthly",
		AutoRenew:      false,
		StartsAt:       now,
		ExpiresAt:      now.Add(duration),
		SourceSystem:   sourceSystem,
		SourceID:       sourceID,
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	s.syncAIPlan(ctx, orgID)
	s.log.InfoContext(ctx, "subscription activated", "user_id", userID, "plan", planSlug, "expires", sub.ExpiresAt)
	return sub, nil
}

// SubscribeWithWallet purchases or upgrades a subscription plan using the user's/org's wallet balance.
func (s *Service) SubscribeWithWallet(
	ctx context.Context,
	userID int64,
	orgID *int64,
	planSlug string,
	cycle string,
	autoRenew bool,
) (*Subscription, error) {
	plan, err := s.repo.GetPlanBySlug(ctx, planSlug)
	if err != nil {
		return nil, err
	}

	if cycle != "annual" && cycle != "monthly" {
		cycle = "monthly"
	}

	cost := plan.PriceMonth
	durationDays := 30
	if cycle == "annual" {
		cost = plan.PriceYear
		durationDays = 365
	}

	wallet, err := s.repo.GetOrCreateWallet(ctx, userID, "EGP")
	if err != nil {
		return nil, err
	}

	if !cost.IsZero() && !cost.IsNegative() {
		if wallet.Balance.Minor() < cost.Minor() {
			return nil, apperr.Conflict("wallet.insufficient_funds", i18n.T("ar", "billing.err.insufficient_funds"))
		}

		negCost := money.FromMinor(-cost.Minor())
		desc := fmt.Sprintf(i18n.TDefault("w4_mod.s_s_70"), plan.Name.Get("ar"), func() string {
			if cycle == "annual" {
				return i18n.T("ar", "billing.cycle.annual")
			}
			return i18n.T("ar", "billing.cycle.monthly")
		}())

		_, err = s.repo.RecordTransaction(ctx, wallet.ID, TxPurchase, negCost, "subscription_checkout", nil, desc)
		if err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	sub := &Subscription{
		UserID:         userID,
		OrganizationID: orgID,
		PlanID:         plan.ID,
		Status:         SubActive,
		BillingCycle:   cycle,
		AutoRenew:      autoRenew,
		StartsAt:       now,
		ExpiresAt:      now.Add(time.Duration(durationDays) * 24 * time.Hour),
		SourceSystem:   "wallet_checkout",
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	// The upgrade the user just paid for is only real once their AI quota
	// follows it. This is the transition that used to be silently dropped.
	s.syncAIPlan(ctx, orgID)

	s.log.InfoContext(ctx, "subscription purchased via wallet", "user_id", userID, "org_id", orgID, "plan", planSlug, "cycle", cycle, "cost", cost.String(), "auto_renew", autoRenew)
	return sub, nil
}

// ProcessDueSubscriptionRenewals checks and executes auto-renewals for all due subscriptions.
func (s *Service) ProcessDueSubscriptionRenewals(ctx context.Context) (renewed int, failed int, err error) {
	now := time.Now().UTC()
	dueSubs, err := s.repo.ListDueSubscriptionsForRenewal(ctx, now)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to list due subscriptions for renewal", "error", err)
		return 0, 0, err
	}

	for _, sub := range dueSubs {
		plan, err := s.repo.GetPlanByID(ctx, sub.PlanID)
		if err != nil {
			s.log.ErrorContext(ctx, "renewal: failed to fetch plan", "sub_id", sub.ID, "plan_id", sub.PlanID, "error", err)
			continue
		}

		cost := plan.PriceMonth
		durationDays := 30
		if sub.BillingCycle == "annual" {
			cost = plan.PriceYear
			durationDays = 365
		}

		// Calculate new expiration date anchored on current expiration (or now if past)
		baseTime := sub.ExpiresAt
		if baseTime.Before(now.Add(-24 * time.Hour)) {
			baseTime = now
		}
		newExpiresAt := baseTime.Add(time.Duration(durationDays) * 24 * time.Hour)

		wallet, err := s.repo.GetOrCreateWallet(ctx, sub.UserID, "EGP")
		if err != nil {
			s.log.ErrorContext(ctx, "renewal: failed to get wallet", "sub_id", sub.ID, "user_id", sub.UserID, "error", err)
			continue
		}

		desc := fmt.Sprintf(i18n.T("ar", "billing.renewal_desc"), plan.Name.Get("ar"), sub.BillingCycle)
		err = s.repo.RenewSubscription(ctx, sub.ID, wallet.ID, cost, newExpiresAt, desc)
		if err != nil {
			s.log.WarnContext(ctx, "subscription renewal failed", "sub_id", sub.ID, "user_id", sub.UserID, "error", err)
			_ = s.repo.UpdateSubscriptionStatus(ctx, sub.ID, SubPastDue, sub.RenewalAttempts+1)
			failed++
		} else {
			// A renewal can follow a plan change made while the old term ran,
			// and it reopens the Gateway's budget window either way.
			s.syncAIPlan(ctx, sub.OrganizationID)
			s.log.InfoContext(ctx, "subscription renewed successfully", "sub_id", sub.ID, "user_id", sub.UserID, "new_expires_at", newExpiresAt)
			renewed++
		}
	}

	return renewed, failed, nil
}

// CheckEntitlement resolves whether a user has access to a specific feature key.
func (s *Service) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	return s.repo.CheckEntitlement(ctx, userID, featureKey)
}

// CheckOrgEntitlement resolves whether an organization/user has active access to featureKey.
func (s *Service) CheckOrgEntitlement(ctx context.Context, orgID, userID int64, featureKey string) (bool, error) {
	return s.repo.CheckOrgEntitlement(ctx, orgID, userID, featureKey)
}

// CreateInvoice generates a new B2B invoice.
func (s *Service) CreateInvoice(ctx context.Context, inv *Invoice) (*Invoice, error) {
	if inv.OrganizationID <= 0 {
		return nil, apperr.Validation("invoice.org_required", "Organization ID is required.", nil)
	}
	if inv.InvoiceNumber == "" {
		inv.InvoiceNumber = fmt.Sprintf("INV-%d-%d", inv.OrganizationID, time.Now().Unix())
	}
	if inv.Status == "" {
		inv.Status = InvoiceDraft
	}

	var subtotal money.Amount
	for _, l := range inv.Lines {
		subtotal, _ = subtotal.Add(l.TotalPrice)
	}
	inv.Subtotal = subtotal
	total, _ := subtotal.Add(inv.TaxAmount)
	total, _ = total.Sub(inv.DiscountAmount)
	inv.TotalAmount = total

	if err := s.repo.CreateInvoice(ctx, inv); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "invoice created", "invoice_id", inv.ID, "number", inv.InvoiceNumber, "total", inv.TotalAmount.String())
	return inv, nil
}

// GetInvoice returns an invoice by ID.
func (s *Service) GetInvoice(ctx context.Context, id int64) (*Invoice, error) {
	return s.repo.GetInvoiceByID(ctx, id)
}

// GetInvoiceByOrderID returns an invoice by its associated order ID.
func (s *Service) GetInvoiceByOrderID(ctx context.Context, orderID int64) (*Invoice, error) {
	return s.repo.GetInvoiceByOrderID(ctx, orderID)
}

// ListInvoices lists invoices for an organization.
func (s *Service) ListInvoices(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, error) {
	return s.repo.ListInvoicesByOrg(ctx, orgID, limit, offset)
}

// ListInvoicesWithTotal lists paginated invoices for an organization with total count.
func (s *Service) ListInvoicesWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, int, error) {
	return s.repo.ListInvoicesByOrgWithTotal(ctx, orgID, limit, offset)
}

// MarkInvoicePaid updates invoice status to paid.
func (s *Service) MarkInvoicePaid(ctx context.Context, id int64) error {
	return s.repo.UpdateInvoiceStatus(ctx, id, InvoicePaid)
}

// AddPaymentMethod saves a user payment method.
func (s *Service) AddPaymentMethod(ctx context.Context, pm *UserPaymentMethod) error {
	if pm.UserID <= 0 || pm.Provider == "" || pm.AccountIdentifier == "" {
		return apperr.Validation("payment_method.invalid", "User ID, provider, and account identifier are required.", nil)
	}
	return s.repo.AddPaymentMethod(ctx, pm)
}

// GetPaymentMethodByID returns a single payment method for a user.
func (s *Service) GetPaymentMethodByID(ctx context.Context, userID, id int64) (*UserPaymentMethod, error) {
	if userID <= 0 || id <= 0 {
		return nil, apperr.Validation("payment_method.invalid_id", "Valid user ID and payment method ID are required.", nil)
	}
	return s.repo.GetPaymentMethodByID(ctx, userID, id)
}

// ListPaymentMethods returns saved payment methods for a user.
func (s *Service) ListPaymentMethods(ctx context.Context, userID int64) ([]*UserPaymentMethod, error) {
	return s.repo.ListPaymentMethods(ctx, userID)
}

// UpdatePaymentMethod updates a user payment method.
func (s *Service) UpdatePaymentMethod(ctx context.Context, pm *UserPaymentMethod) error {
	if pm.ID <= 0 || pm.UserID <= 0 || pm.Provider == "" || pm.AccountIdentifier == "" {
		return apperr.Validation("payment_method.invalid", "Valid payment method ID, user ID, provider, and account identifier are required.", nil)
	}
	return s.repo.UpdatePaymentMethod(ctx, pm)
}

// SetDefaultPaymentMethod sets one payment method as default for a user.
func (s *Service) SetDefaultPaymentMethod(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return apperr.Validation("payment_method.invalid_id", "Valid user ID and payment method ID are required.", nil)
	}
	return s.repo.SetDefaultPaymentMethod(ctx, userID, id)
}

// DeletePaymentMethod removes a user payment method scoped to the owner.
func (s *Service) DeletePaymentMethod(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return apperr.Validation("payment_method.invalid_id", "Valid user ID and payment method ID are required.", nil)
	}
	return s.repo.DeletePaymentMethod(ctx, userID, id)
}

// ListPlatformPaymentMethods returns all platform configured payment channels.
func (s *Service) ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*PlatformPaymentMethod, error) {
	return s.repo.ListPlatformPaymentMethods(ctx, onlyActive)
}

// GetPlatformPaymentMethod returns a single platform payment method.
func (s *Service) GetPlatformPaymentMethod(ctx context.Context, id string) (*PlatformPaymentMethod, error) {
	return s.repo.GetPlatformPaymentMethod(ctx, id)
}

// SavePlatformPaymentMethod adds or updates a platform payment channel configuration.
func (s *Service) SavePlatformPaymentMethod(ctx context.Context, pm *PlatformPaymentMethod) error {
	if pm.ID == "" || pm.Name.IsEmpty() {
		return apperr.Validation("payment_method.invalid", "ID and Name are required for platform payment method.", nil)
	}
	return s.repo.SavePlatformPaymentMethod(ctx, pm)
}
