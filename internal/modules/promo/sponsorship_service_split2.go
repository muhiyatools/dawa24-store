package promo

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AdminListSponsorshipRequestsWithTotal returns all requests for admin moderation with total count.
func (s *Service) AdminListSponsorshipRequestsWithTotal(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, int, error) {
	return s.repo.ListAllSponsorshipRequestsWithTotal(database.AsSystem(ctx), limit, offset)
}

// ListSponsorshipRequestsByOrgWithTotal returns the vendor's sponsorship requests with total count.
func (s *Service) ListSponsorshipRequestsByOrgWithTotal(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, int, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, 0, database.ErrNoTenant
	}
	return s.repo.ListSponsorshipRequestsByOrgWithTotal(ctx, orgID, limit, offset)
}
