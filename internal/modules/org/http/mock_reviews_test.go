package http_test

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/org"
)

func (r stubRepo) ListReviewsForVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*org.Review, error) {
	r.fail("ListReviewsForVendor")
	return nil, nil
}

func (r stubRepo) GetReviewByOrderAndVendor(ctx context.Context, orderID, vendorOrgID int64) (*org.Review, error) {
	r.fail("GetReviewByOrderAndVendor")
	return nil, nil
}

func (r stubRepo) ListReviewsForOrder(ctx context.Context, orderID int64) ([]*org.Review, error) {
	r.fail("ListReviewsForOrder")
	return nil, nil
}

func (r stubRepo) HasDeliveredOrderFromVendor(ctx context.Context, customerOrgID, vendorOrgID int64) (bool, error) {
	r.fail("HasDeliveredOrderFromVendor")
	return false, nil
}

func (happyRepo) ListReviewsForVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*org.Review, error) {
	return nil, nil
}

func (happyRepo) GetReviewByOrderAndVendor(ctx context.Context, orderID, vendorOrgID int64) (*org.Review, error) {
	return nil, nil
}

func (happyRepo) ListReviewsForOrder(ctx context.Context, orderID int64) ([]*org.Review, error) {
	return nil, nil
}

func (happyRepo) HasDeliveredOrderFromVendor(ctx context.Context, customerOrgID, vendorOrgID int64) (bool, error) {
	return true, nil
}