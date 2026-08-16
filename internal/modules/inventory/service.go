package inventory

import (
	"context"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Service coordinates inventory management and stock ledger operations.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new inventory service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreateWarehouse creates a storage warehouse for the current tenant.
func (s *Service) CreateWarehouse(ctx context.Context, w *Warehouse) (*Warehouse, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	w.OrganizationID = orgID

	if w.Name == "" {
		return nil, apperr.Validation("warehouse.name_required", "Warehouse name is required.", nil)
	}
	w.IsActive = true

	if err := s.repo.CreateWarehouse(ctx, w); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "warehouse created", "warehouse_id", w.ID, "org_id", w.OrganizationID)
	return w, nil
}

// ListWarehouses returns all active warehouses for the current tenant.
func (s *Service) ListWarehouses(ctx context.Context) ([]*Warehouse, error) {
	return s.repo.ListWarehouses(ctx)
}

// AdjustStockInput carries parameters for a stock adjustment.
type AdjustStockInput struct {
	StockID       int64        `json:"stock_id"`
	Delta         int          `json:"delta"`
	Type          MovementType `json:"type"`
	Details       string       `json:"details,omitempty"`
	ReferenceType string       `json:"reference_type,omitempty"`
	ReferenceID   *int64       `json:"reference_id,omitempty"`
	UserID        *int64       `json:"user_id,omitempty"`
}

// AdjustStock updates stock atomically and adds an entry to the movement ledger.
func (s *Service) AdjustStock(ctx context.Context, input AdjustStockInput) (*Stock, error) {
	if input.StockID <= 0 {
		return nil, apperr.Validation("stock_id.required", "Stock ID is required.", nil)
	}
	if input.Delta == 0 {
		return nil, apperr.Validation("delta.invalid", "Delta quantity cannot be zero.", nil)
	}
	if input.Type == "" {
		input.Type = MovementAdjustment
	}

	movement := StockMovement{
		Type:          input.Type,
		QuantityDelta: input.Delta,
		Details:       input.Details,
		ReferenceType: input.ReferenceType,
		ReferenceID:   input.ReferenceID,
		UserID:        input.UserID,
	}

	stock, err := s.repo.AdjustStock(ctx, input.StockID, input.Delta, movement)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "stock adjusted", "stock_id", stock.ID, "delta", input.Delta, "balance", stock.Quantity)
	return stock, nil
}

// TransferStock transfers stock between two warehouses belonging to the tenant.
func (s *Service) TransferStock(ctx context.Context, t *WarehouseTransfer) (*WarehouseTransfer, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	t.OrganizationID = orgID

	if err := t.Validate(); err != nil {
		return nil, err
	}

	// 1. Check source stock
	fromStock, err := s.repo.GetStock(ctx, t.FromWarehouseID, t.ProductVariantID)
	if err != nil {
		return nil, err
	}
	if fromStock.Quantity < t.Quantity {
		return nil, apperr.Validation("transfer.insufficient_source", "Source warehouse has insufficient stock.", nil)
	}

	// 2. Deduct the source only. The destination is credited on receipt, not
	//    on dispatch — see transfer_state.go. Goods on a van belong to neither
	//    warehouse's sellable quantity, and crediting early would let the
	//    destination sell medicine it has not received.
	_, err = s.repo.AdjustStock(ctx, fromStock.ID, -t.Quantity, StockMovement{
		Type:          MovementTransfer,
		QuantityDelta: -t.Quantity,
		Details:       "Outbound transfer dispatched",
		UserID:        t.InitiatedBy,
	})
	if err != nil {
		return nil, err
	}

	t.Status = TransferInTransit
	if err := s.repo.CreateTransfer(ctx, t); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "warehouse transfer dispatched",
		"transfer_id", t.ID, "quantity", t.Quantity,
		"from", t.FromWarehouseID, "to", t.ToWarehouseID)
	return t, nil
}

// ListStocksByWarehouse retrieves stock rows for a warehouse.
func (s *Service) ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*Stock, error) {
	return s.repo.ListStocksByWarehouse(ctx, warehouseID)
}

// ListStockMovements retrieves the movement ledger for a stock row.
func (s *Service) ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*StockMovement, error) {
	return s.repo.ListStockMovements(ctx, stockID, limit)
}
