package billing

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// TogglePlatformPaymentMethod activates or deactivates a payment method.
func (s *Service) TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error {
	if id == "" {
		return apperr.Validation("payment_method.invalid_id", "ID is required.", nil)
	}
	return s.repo.TogglePlatformPaymentMethod(ctx, id, active)
}

// DeletePlatformPaymentMethod deletes a platform payment channel.
func (s *Service) DeletePlatformPaymentMethod(ctx context.Context, id string) error {
	if id == "" {
		return apperr.Validation("payment_method.invalid_id", "ID is required.", nil)
	}
	return s.repo.DeletePlatformPaymentMethod(ctx, id)
}

// ListPayments returns payments for an organization.
func (s *Service) ListPayments(ctx context.Context, orgID int64, limit, offset int) ([]*Payment, error) {
	return s.repo.ListPaymentsByOrg(ctx, orgID, limit, offset)
}
