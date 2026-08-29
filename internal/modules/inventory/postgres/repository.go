package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements inventory.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a PostgreSQL inventory repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateWarehouse creates a storage warehouse for an organization.
func (r *Repository) CreateWarehouse(ctx context.Context, w *inventory.Warehouse) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO inventory.warehouses (
				organization_id, branch_id, name, code, address, phone,
				latitude, longitude, is_active
			) VALUES (
				$1,
				COALESCE($2, (SELECT b.id FROM org.branches b WHERE b.organization_id = $1 AND b.deleted_at IS NULL ORDER BY b.is_main DESC, b.id ASC LIMIT 1)),
				$3, $4, $5, $6, $7, $8, $9
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			w.OrganizationID, w.BranchID, w.Name, w.Code, w.Address, w.Phone,
			w.Latitude, w.Longitude, w.IsActive,
		).Scan(&w.ID, &w.PublicID, &w.CreatedAt, &w.UpdatedAt)
	})
}

// GetWarehouseByID retrieves a warehouse by ID.
func (r *Repository) GetWarehouseByID(ctx context.Context, id int64) (*inventory.Warehouse, error) {
	var w inventory.Warehouse
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, branch_id, name, code, address, phone,
			       latitude, longitude, is_active, created_at, updated_at, deleted_at
			FROM inventory.warehouses
			WHERE id = $1 AND deleted_at IS NULL;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&w.ID, &w.PublicID, &w.OrganizationID, &w.BranchID, &w.Name, &w.Code,
			&w.Address, &w.Phone, &w.Latitude, &w.Longitude, &w.IsActive,
			&w.CreatedAt, &w.UpdatedAt, &w.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("warehouse")
			}
			return fmt.Errorf("inventory postgres: get warehouse: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListWarehouses lists all active warehouses for the current tenant.
func (r *Repository) ListWarehouses(ctx context.Context) ([]*inventory.Warehouse, error) {
	var list []*inventory.Warehouse
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, branch_id, name, code, address, phone,
			       latitude, longitude, is_active, created_at, updated_at, deleted_at
			FROM inventory.warehouses
			WHERE deleted_at IS NULL
			ORDER BY name ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var w inventory.Warehouse
			if err := rows.Scan(
				&w.ID, &w.PublicID, &w.OrganizationID, &w.BranchID, &w.Name, &w.Code,
				&w.Address, &w.Phone, &w.Latitude, &w.Longitude, &w.IsActive,
				&w.CreatedAt, &w.UpdatedAt, &w.DeletedAt,
			); err != nil {
				return err
			}
			list = append(list, &w)
		}
		return rows.Err()
	})
	return list, err
}

// GetStock retrieves a stock record for a given warehouse and product variant.
func (r *Repository) GetStock(ctx context.Context, warehouseID, variantID int64) (*inventory.Stock, error) {
	var s inventory.Stock
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, warehouse_id, product_id, product_variant_id,
			       quantity, min_threshold, negotiation, created_at, updated_at, deleted_at
			FROM inventory.stocks
			WHERE warehouse_id = $1 AND product_variant_id = $2 AND deleted_at IS NULL;
		`
		err := tx.QueryRow(txCtx, query, warehouseID, variantID).Scan(
			&s.ID, &s.OrganizationID, &s.WarehouseID, &s.ProductID, &s.ProductVariantID,
			&s.Quantity, &s.MinThreshold, &s.Negotiation, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("stock")
			}
			return fmt.Errorf("inventory postgres: get stock: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpsertStock creates or updates initial stock metadata.
func (r *Repository) UpsertStock(ctx context.Context, s *inventory.Stock) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO inventory.stocks (
				organization_id, warehouse_id, product_id, product_variant_id,
				quantity, min_threshold, negotiation
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (warehouse_id, product_variant_id) DO UPDATE SET
				min_threshold = EXCLUDED.min_threshold,
				negotiation = EXCLUDED.negotiation,
				updated_at = now()
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			s.OrganizationID, s.WarehouseID, s.ProductID, s.ProductVariantID,
			s.Quantity, s.MinThreshold, s.Negotiation,
		).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	})
}

// ClearWarehouseStocks deletes all stock records for a warehouse (for clear_and_add import mode).
func (r *Repository) ClearWarehouseStocks(ctx context.Context, warehouseID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM inventory.stocks WHERE warehouse_id = $1;`
		_, err := tx.Exec(txCtx, query, warehouseID)
		return err
	})
}

// AdjustStock updates stock atomically and records an entry in the stock movements ledger.
func (r *Repository) AdjustStock(ctx context.Context, stockID int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	var updatedStock inventory.Stock
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		querySelect := `
			SELECT id, organization_id, warehouse_id, product_id, product_variant_id,
			       quantity, min_threshold, negotiation, created_at, updated_at
			FROM inventory.stocks
			WHERE id = $1 FOR UPDATE;
		`
		err := tx.QueryRow(txCtx, querySelect, stockID).Scan(
			&updatedStock.ID, &updatedStock.OrganizationID, &updatedStock.WarehouseID,
			&updatedStock.ProductID, &updatedStock.ProductVariantID, &updatedStock.Quantity,
			&updatedStock.MinThreshold, &updatedStock.Negotiation, &updatedStock.CreatedAt,
			&updatedStock.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("stock")
			}
			return fmt.Errorf("inventory postgres: lock stock: %w", err)
		}

		newQuantity := updatedStock.Quantity + delta
		if newQuantity < 0 {
			return apperr.Validation("stock.insufficient", "Insufficient stock quantity for this operation.", map[string]string{
				"available": strconv.Itoa(updatedStock.Quantity),
				"requested": strconv.Itoa(-delta),
			})
		}

		queryUpdate := `UPDATE inventory.stocks SET quantity = $2, updated_at = now() WHERE id = $1 RETURNING updated_at;`
		if err := tx.QueryRow(txCtx, queryUpdate, stockID, newQuantity).Scan(&updatedStock.UpdatedAt); err != nil {
			return fmt.Errorf("inventory postgres: update quantity: %w", err)
		}
		updatedStock.Quantity = newQuantity

		queryMovement := `
			INSERT INTO inventory.stock_movements (
				organization_id, stock_id, type, quantity_delta, balance_after,
				details, reference_type, reference_id, user_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
		`
		_, err = tx.Exec(txCtx, queryMovement,
			updatedStock.OrganizationID, stockID, string(movement.Type), delta,
			newQuantity, movement.Details, movement.ReferenceType, movement.ReferenceID, movement.UserID,
		)
		if err != nil {
			return fmt.Errorf("inventory postgres: record movement: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updatedStock, nil
}

// ListStocksByWarehouse retrieves all stocks located in a given warehouse.
func (r *Repository) ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
			       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			WHERE s.warehouse_id = $1 AND s.deleted_at IS NULL
			ORDER BY s.id ASC;
		`
		rows, err := tx.Query(txCtx, query, warehouseID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s inventory.Stock
			if err := rows.Scan(
				&s.ID, &s.OrganizationID, &s.WarehouseID, &s.ProductID, &s.ProductVariantID,
				&s.Quantity, &s.MinThreshold, &s.Negotiation, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
			); err != nil {
				return err
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// ListDetailedStocksByWarehouse retrieves joined variant, product, and stock information for a warehouse.
func (r *Repository) ListDetailedStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.DetailedWarehouseStockView, error) {
	var list []*inventory.DetailedWarehouseStockView
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT s.id, s.warehouse_id, s.organization_id, s.product_id, s.product_variant_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''),
			       COALESCE(v.name->>'ar', v.name->>'en', ''),
			       COALESCE(p.scientific_name, ''),
			       COALESCE(p.dosage_form, ''),
			       COALESCE(p.concentration, ''),
			       COALESCE(p.manufacturing_companies, p.company, ''),
			       COALESCE(v.sku, p.sku, ''),
			       COALESCE(v.barcode, p.barcode, ''),
			       COALESCE(v.batch_number, ''),
			       v.expiry_date,
			       COALESCE(v.price::text, p.price::text, '0.00'),
			       COALESCE(v.cost_price::text, '0.00'),
			       COALESCE(p.old_price::text, p.price::text, '0.00'),
			       COALESCE(v.discount::text, p.discount::text, '0.00'),
			       s.quantity,
			       s.min_threshold,
			       COALESCE(v.is_negotiable, false),
			       COALESCE(v.status, 'active'),
			       s.updated_at
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			WHERE s.warehouse_id = $1 AND s.deleted_at IS NULL
			ORDER BY s.id ASC;
		`
		rows, err := tx.Query(txCtx, query, warehouseID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v inventory.DetailedWarehouseStockView
			if err := rows.Scan(
				&v.StockID, &v.WarehouseID, &v.OrganizationID, &v.ProductID, &v.ProductVariantID,
				&v.ProductName, &v.VariantName, &v.ScientificName, &v.DosageForm, &v.Concentration,
				&v.Manufacturer, &v.SKU, &v.Barcode, &v.BatchNumber,
				&v.ExpiryDate, &v.PriceStr, &v.CostPriceStr, &v.PublicPriceStr, &v.DiscountStr,
				&v.Quantity, &v.MinThreshold, &v.IsNegotiable, &v.Status, &v.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &v)
		}
		return rows.Err()
	})
	return list, err
}

// ListStocksByOrg retrieves all stocks belonging to an organization across warehouses.
func (r *Repository) ListStocksByOrg(ctx context.Context, orgID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT s.id, s.organization_id, s.warehouse_id, s.product_id, s.product_variant_id,
			       s.quantity, s.min_threshold, s.negotiation, s.created_at, s.updated_at, s.deleted_at
			FROM inventory.stocks s
			JOIN catalog.products p ON p.id = s.product_id AND p.deleted_at IS NULL
			JOIN catalog.product_variants v ON v.id = s.product_variant_id AND v.deleted_at IS NULL
			WHERE s.organization_id = $1 AND s.deleted_at IS NULL
			ORDER BY s.id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s inventory.Stock
			if err := rows.Scan(
				&s.ID, &s.OrganizationID, &s.WarehouseID, &s.ProductID, &s.ProductVariantID,
				&s.Quantity, &s.MinThreshold, &s.Negotiation, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
			); err != nil {
				return err
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// ListStockMovements retrieves ledger movements for an individual stock.
func (r *Repository) ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*inventory.StockMovement, error) {
	var list []*inventory.StockMovement
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, stock_id, type, quantity_delta, balance_after,
			       details, reference_type, reference_id, user_id, created_at
			FROM inventory.stock_movements
			WHERE stock_id = $1
			ORDER BY created_at DESC
			LIMIT $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		rows, err := tx.Query(txCtx, query, stockID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m inventory.StockMovement
			var typeStr string
			if err := rows.Scan(
				&m.ID, &m.OrganizationID, &m.StockID, &typeStr, &m.QuantityDelta,
				&m.BalanceAfter, &m.Details, &m.ReferenceType, &m.ReferenceID,
				&m.UserID, &m.CreatedAt,
			); err != nil {
				return err
			}
			m.Type = inventory.MovementType(typeStr)
			list = append(list, &m)
		}
		return rows.Err()
	})
	return list, err
}

// CreateTransfer initiates an inter-warehouse transfer.
func (r *Repository) CreateTransfer(ctx context.Context, t *inventory.WarehouseTransfer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO inventory.warehouse_transfers (
				organization_id, from_warehouse_id, to_warehouse_id, product_id,
				product_variant_id, quantity, status, initiated_by, notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			t.OrganizationID, t.FromWarehouseID, t.ToWarehouseID, t.ProductID,
			t.ProductVariantID, t.Quantity, string(t.Status), t.InitiatedBy, t.Notes,
		).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	})
}

// GetTransferByID retrieves a transfer record.
func (r *Repository) GetTransferByID(ctx context.Context, id int64) (*inventory.WarehouseTransfer, error) {
	var t inventory.WarehouseTransfer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, organization_id, from_warehouse_id, to_warehouse_id, product_id,
			       product_variant_id, quantity, status, initiated_by, notes, created_at, updated_at
			FROM inventory.warehouse_transfers
			WHERE id = $1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&t.ID, &t.OrganizationID, &t.FromWarehouseID, &t.ToWarehouseID, &t.ProductID,
			&t.ProductVariantID, &t.Quantity, &statusStr, &t.InitiatedBy, &t.Notes,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("warehouse_transfer")
			}
			return fmt.Errorf("inventory postgres: get transfer: %w", err)
		}
		t.Status = inventory.TransferStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTransferStatus updates the transfer lifecycle state.
func (r *Repository) UpdateTransferStatus(ctx context.Context, id int64, from, to inventory.TransferStatus) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// The `status = $2` predicate is the compare half of the swap. Without
		// it, two concurrent receives both succeed and the destination is
		// credited twice.
		query := `
			UPDATE inventory.warehouse_transfers
			SET status = $3, updated_at = now()
			WHERE id = $1 AND status = $2;
		`
		res, err := tx.Exec(txCtx, query, id, string(from), string(to))
		if err != nil {
			return fmt.Errorf("inventory postgres: update transfer status: %w", err)
		}
		if res.RowsAffected() == 0 {
			// Either the transfer does not exist, or another request changed
			// its state first. Both mean this caller must not proceed.
			return apperr.Conflict("transfer.state_changed",
				"This transfer was already updated by someone else. Reload and try again.")
		}
		return nil
	})
}

// AvailableQuantity totals the sellable stock of one variant across a supplier's
// warehouses.
//
// It exists because catalog.ProductVariant.StockQty is a field no query ever
// populates — the column does not exist on catalog.product_variants, and stock
// lives here in inventory.stocks. Anything that trusted that field was reading a
// permanent zero, which is why the old cart's stock guard
// (`if variant.StockQty > 0 && ...`) never fired.
//
// AsSystem: a pharmacy asking whether it may buy is by definition asking about
// another tenant's stock. Only the total crosses the boundary.
func (r *Repository) AvailableQuantity(ctx context.Context, variantID int64) (int, error) {
	var qty int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const q = `
			SELECT COALESCE(SUM(s.quantity), 0)
			FROM inventory.stocks s
			WHERE s.product_variant_id = $1
			  AND s.deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, q, variantID).Scan(&qty)
	})
	if qty < 0 {
		// A negative total means stock movements are inconsistent. Report none
		// rather than a negative, so callers cannot read it as availability.
		qty = 0
	}
	return qty, err
}
