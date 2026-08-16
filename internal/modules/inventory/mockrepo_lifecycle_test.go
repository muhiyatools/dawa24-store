package inventory_test

import (
	"context"
	"sort"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Lifecycle methods of mockInventoryRepo, kept separate from the original mock
// so inventory_test.go stays readable.

func (m *mockInventoryRepo) UpdateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	if _, ok := m.warehouses[w.ID]; !ok {
		return apperr.NotFound("warehouse")
	}
	m.warehouses[w.ID] = w
	return nil
}

func (m *mockInventoryRepo) SoftDeleteWarehouse(_ context.Context, id int64) error {
	if _, ok := m.warehouses[id]; !ok {
		return apperr.NotFound("warehouse")
	}
	delete(m.warehouses, id)
	return nil
}

func (m *mockInventoryRepo) CountStockInWarehouse(_ context.Context, warehouseID int64) (int, error) {
	count := 0
	for _, s := range m.stocksByID {
		if s.WarehouseID == warehouseID && s.Quantity > 0 {
			count++
		}
	}
	return count, nil
}

func (m *mockInventoryRepo) ListLowStock(_ context.Context, limit, offset int) ([]*inventory.Stock, error) {
	var low []*inventory.Stock
	for _, s := range m.stocksByID {
		if s.Quantity <= s.MinThreshold {
			low = append(low, s)
		}
	}
	sort.Slice(low, func(i, j int) bool { return low[i].ID < low[j].ID })
	return pageSlice(low, limit, offset), nil
}

func (m *mockInventoryRepo) ListMovementsByOrg(_ context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	var all []*inventory.StockMovement
	for _, ms := range m.movements {
		all = append(all, ms...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	return pageSlice(all, limit, offset), nil
}

func (m *mockInventoryRepo) ListTransfers(_ context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, error) {
	var list []*inventory.WarehouseTransfer
	for _, t := range m.transfers {
		if status == "" || string(t.Status) == status {
			list = append(list, t)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID > list[j].ID })
	return pageSlice(list, limit, offset), nil
}

// pageSlice applies limit and offset without panicking on out-of-range values.
func pageSlice[T any](in []T, limit, offset int) []T {
	if offset >= len(in) {
		return nil
	}
	end := offset + limit
	if limit <= 0 || end > len(in) {
		end = len(in)
	}
	return in[offset:end]
}
