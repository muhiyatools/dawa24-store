package inventory

import (
	"context"
)

// Repository defines storage operations for the inventory bounded context.
type Repository interface {
	CreateWarehouse(ctx context.Context, w *Warehouse) error
	GetWarehouseByID(ctx context.Context, id int64) (*Warehouse, error)
	ListWarehouses(ctx context.Context) ([]*Warehouse, error)

	GetStock(ctx context.Context, warehouseID, variantID int64) (*Stock, error)
	UpsertStock(ctx context.Context, s *Stock) error
	AdjustStock(ctx context.Context, stockID int64, delta int, movement StockMovement) (*Stock, error)
	ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*Stock, error)
	ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*StockMovement, error)

	CreateTransfer(ctx context.Context, t *WarehouseTransfer) error
	GetTransferByID(ctx context.Context, id int64) (*WarehouseTransfer, error)
	UpdateTransferStatus(ctx context.Context, id int64, status TransferStatus) error
}
