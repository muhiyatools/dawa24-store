package queue

import (
	"time"

	"github.com/riverqueue/river"
)

// OrderNotificationArgs defines job parameters for dispatching order transition alerts.
type OrderNotificationArgs struct {
	OrderID    int64     `json:"order_id"`
	CustomerID int64     `json:"customer_id"`
	ToStatus   string    `json:"to_status"`
	At         time.Time `json:"at"`
}

func (OrderNotificationArgs) Kind() string { return "notifications.order_status" }
func (OrderNotificationArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "notifications"}
}

// IngestBatchArgs defines job parameters for chunked catalog row processing.
type IngestBatchArgs struct {
	SessionID      int64 `json:"session_id"`
	OrganizationID int64 `json:"organization_id"`
	Offset         int   `json:"offset"`
	BatchSize      int   `json:"batch_size"`
}

func (IngestBatchArgs) Kind() string { return "ingest.process_batch" }
func (IngestBatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "imports"}
}

// ExpirePromotionsArgs defines periodic maintenance parameters for expiring stale offers and sponsorships.
type ExpirePromotionsArgs struct {
	At time.Time `json:"at"`
}

func (ExpirePromotionsArgs) Kind() string { return "promo.expire_promotions" }
func (ExpirePromotionsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "maintenance"}
}
