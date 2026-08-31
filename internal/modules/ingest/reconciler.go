package ingest

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CatalogAdapter abstracts variant queries and persistence for the ingest reconciler.
type CatalogAdapter interface {
	GetVariantBySKUOrBarcode(ctx context.Context, orgID int64, sku, barcode string) (*catalog.ProductVariant, error)
	GetVariantByProductAndOrg(ctx context.Context, orgID int64, productID int64) (*catalog.ProductVariant, error)
	CreateVariant(ctx context.Context, v *catalog.ProductVariant) (*catalog.ProductVariant, error)
	UpdateVariant(ctx context.Context, id int64, v *catalog.ProductVariant) (*catalog.ProductVariant, error)
	GetProduct(ctx context.Context, id int64) (*catalog.Product, []*catalog.ProductVariant, error)
}

// InventoryAdapter abstracts stock operations for the ingest reconciler.
type InventoryAdapter interface {
	ClearWarehouseStocks(ctx context.Context, warehouseID int64) error
	UpsertStock(ctx context.Context, s *inventory.Stock) error
}

// ReconcileOutcome summarizes the result of committing an import session.
type ReconcileOutcome struct {
	TotalProcessed int `json:"total_processed"`
	Inserted       int `json:"inserted"`
	Updated        int `json:"updated"`
	Skipped        int `json:"skipped"`
	Errors         int `json:"errors"`
}

// CommitSessionWithReconciliation executes the selected import mode with full transaction safety.
func (s *Service) CommitSessionWithReconciliation(
	ctx context.Context,
	sessionID int64,
	catAdapter CatalogAdapter,
	invAdapter InventoryAdapter,
) (*ReconcileOutcome, error) {
	session, err := s.repo.GetImportSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.Status == StatusCompleted {
		return nil, apperr.Conflict("session.already_completed", "Import session is already completed.")
	}

	orgID := session.OrganizationID
	if orgID <= 0 {
		if tenant, ok := database.TenantFrom(ctx); ok {
			orgID = tenant
		}
	}

	warehouseID := int64(0)
	if session.WarehouseID != nil {
		warehouseID = *session.WarehouseID
	}

	rows, err := s.repo.ListImportRows(ctx, sessionID, "", 10000, 0)
	if err != nil {
		return nil, err
	}

	outcome := &ReconcileOutcome{
		TotalProcessed: len(rows),
	}

	// For ModeClearAndAdd: atomically clear existing warehouse stocks first
	if session.ImportMode == ModeClearAndAdd && warehouseID > 0 && invAdapter != nil {
		if err := invAdapter.ClearWarehouseStocks(ctx, warehouseID); err != nil {
			s.log.ErrorContext(ctx, "failed to clear warehouse stock for import", "warehouse_id", warehouseID, "error", err)
			return nil, fmt.Errorf("failed to clear warehouse stocks: %w", err)
		}
	}

	nameCol := session.ColumnMapping[FieldProductName]
	priceCol := session.ColumnMapping[FieldPrice]
	costPriceCol := session.ColumnMapping[FieldCostPrice]
	qtyCol := session.ColumnMapping[FieldQuantity]
	discCol := session.ColumnMapping[FieldDiscount]
	barcodeCol := session.ColumnMapping[FieldBarcode]
	skuCol := session.ColumnMapping[FieldSKU]
	unitCol := session.ColumnMapping[FieldUnit]
	minThresholdCol := session.ColumnMapping[FieldMinThreshold]
	minOrderQtyCol := session.ColumnMapping[FieldMinOrderQty]

	var actionUpdates []RowActionUpdate

	for _, row := range rows {
		if !row.IsApproved {
			actionUpdates = append(actionUpdates, RowActionUpdate{
				RowID:        row.ID,
				ImportAction: "skip",
				ErrorDetails: i18n.TDefault("w4_mod.s_395_395"),
			})
			outcome.Skipped++
			continue
		}

		rawName := getRawStringWithFallback(row.RawData, nameCol, FieldProductName)
		rawPriceStr := getRawStringWithFallback(row.RawData, priceCol, FieldPrice)
		rawCostPriceStr := getRawStringWithFallback(row.RawData, costPriceCol, FieldCostPrice)
		rawQtyStr := getRawStringWithFallback(row.RawData, qtyCol, FieldQuantity)
		rawDiscStr := getRawStringWithFallback(row.RawData, discCol, FieldDiscount)
		rawBarcode := getRawStringWithFallback(row.RawData, barcodeCol, FieldBarcode)
		rawSKU := getRawStringWithFallback(row.RawData, skuCol, FieldSKU)
		rawUnit := getRawStringWithFallback(row.RawData, unitCol, FieldUnit)
		rawMinThresholdStr := getRawStringWithFallback(row.RawData, minThresholdCol, FieldMinThreshold)
		rawMinOrderQtyStr := getRawStringWithFallback(row.RawData, minOrderQtyCol, FieldMinOrderQty)

		if rawName == "" && row.NormalizedName != "" {
			rawName = row.NormalizedName
		}

		price, _ := money.Parse(rawPriceStr)
		costPrice, _ := money.Parse(rawCostPriceStr)
		discount, _ := money.Parse(rawDiscStr)
		qty, _ := strconv.Atoi(rawQtyStr)
		minThreshold, _ := strconv.Atoi(rawMinThresholdStr)
		minOrderQty, _ := strconv.Atoi(rawMinOrderQtyStr)

		if minOrderQty <= 0 {
			minOrderQty = 1
		}
		if minThreshold <= 0 {
			minThreshold = 5
		}

		// Determine target product_id
		productID := int64(0)
		if row.MatchedProductID != nil && *row.MatchedProductID > 0 {
			productID = *row.MatchedProductID
		}

		// Locate existing variant
		var existingVariant *catalog.ProductVariant
		if catAdapter != nil {
			if rawSKU != "" || rawBarcode != "" {
				existingVariant, _ = catAdapter.GetVariantBySKUOrBarcode(ctx, orgID, rawSKU, rawBarcode)
			}
			if existingVariant == nil && productID > 0 {
				existingVariant, _ = catAdapter.GetVariantByProductAndOrg(ctx, orgID, productID)
			}
		}

		switch session.ImportMode {
		case ModeAddNewOnly:
			if existingVariant != nil {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "skip",
					ErrorDetails: i18n.TDefault("w4_mod.w4str_215_215"),
				})
				outcome.Skipped++
				continue
			}
			if productID <= 0 {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "error",
					ErrorDetails: i18n.TDefault("w4_mod.s_396_396"),
				})
				outcome.Errors++
				continue
			}
			v, err := s.createVariantAndStock(ctx, orgID, productID, warehouseID, rawName, rawSKU, rawBarcode, rawUnit, price, costPrice, discount, qty, minThreshold, minOrderQty, catAdapter, invAdapter)
			if err != nil {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "error",
					ErrorDetails: err.Error(),
				})
				outcome.Errors++
			} else {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "insert",
					ErrorDetails: fmt.Sprintf(i18n.TDefault("w4_mod.d_216"), v.ID),
				})
				outcome.Inserted++
			}

		case ModeUpdateExistingOnly:
			if existingVariant == nil {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "skip",
					ErrorDetails: i18n.TDefault("w4_mod.w4str_217_217"),
				})
				outcome.Skipped++
				continue
			}
			err := s.updateVariantAndStock(ctx, existingVariant, warehouseID, price, costPrice, discount, rawUnit, rawBarcode, rawSKU, qty, minThreshold, catAdapter, invAdapter)
			if err != nil {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "error",
					ErrorDetails: err.Error(),
				})
				outcome.Errors++
			} else {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "update",
					ErrorDetails: fmt.Sprintf(i18n.TDefault("w4_mod.d_218"), existingVariant.ID),
				})
				outcome.Updated++
			}

		case ModeClearAndAdd:
			if productID <= 0 {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "error",
					ErrorDetails: i18n.TDefault("w4_mod.s_396_396"),
				})
				outcome.Errors++
				continue
			}
			v, err := s.createVariantAndStock(ctx, orgID, productID, warehouseID, rawName, rawSKU, rawBarcode, rawUnit, price, costPrice, discount, qty, minThreshold, minOrderQty, catAdapter, invAdapter)
			if err != nil {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "error",
					ErrorDetails: err.Error(),
				})
				outcome.Errors++
			} else {
				actionUpdates = append(actionUpdates, RowActionUpdate{
					RowID:        row.ID,
					ImportAction: "insert",
					ErrorDetails: fmt.Sprintf(i18n.TDefault("w4_mod.d_219"), v.ID),
				})
				outcome.Inserted++
			}

		case ModeUpdateAndAdd:
			fallthrough
		default:
			if existingVariant != nil {
				err := s.updateVariantAndStock(ctx, existingVariant, warehouseID, price, costPrice, discount, rawUnit, rawBarcode, rawSKU, qty, minThreshold, catAdapter, invAdapter)
				if err != nil {
					actionUpdates = append(actionUpdates, RowActionUpdate{
						RowID:        row.ID,
						ImportAction: "error",
						ErrorDetails: err.Error(),
					})
					outcome.Errors++
				} else {
					actionUpdates = append(actionUpdates, RowActionUpdate{
						RowID:        row.ID,
						ImportAction: "update",
						ErrorDetails: fmt.Sprintf(i18n.TDefault("w4_mod.d_218"), existingVariant.ID),
					})
					outcome.Updated++
				}
			} else {
				if productID <= 0 {
					actionUpdates = append(actionUpdates, RowActionUpdate{
						RowID:        row.ID,
						ImportAction: "error",
						ErrorDetails: i18n.TDefault("w4_mod.s_396_396"),
					})
					outcome.Errors++
					continue
				}
				v, err := s.createVariantAndStock(ctx, orgID, productID, warehouseID, rawName, rawSKU, rawBarcode, rawUnit, price, costPrice, discount, qty, minThreshold, minOrderQty, catAdapter, invAdapter)
				if err != nil {
					actionUpdates = append(actionUpdates, RowActionUpdate{
						RowID:        row.ID,
						ImportAction: "error",
						ErrorDetails: err.Error(),
					})
					outcome.Errors++
				} else {
					actionUpdates = append(actionUpdates, RowActionUpdate{
						RowID:        row.ID,
						ImportAction: "insert",
						ErrorDetails: fmt.Sprintf(i18n.TDefault("w4_mod.d_216"), v.ID),
					})
					outcome.Inserted++
				}
			}
		}
	}

	// Flush all row action updates in batches of 250 rows per statement
	const batchChunk = 250
	for i := 0; i < len(actionUpdates); i += batchChunk {
		end := i + batchChunk
		if end > len(actionUpdates) {
			end = len(actionUpdates)
		}
		_ = s.repo.BatchUpdateImportRowActions(ctx, actionUpdates[i:end])
	}

	_ = s.repo.UpdateSessionStatus(ctx, sessionID, StatusCompleted)
	return outcome, nil
}

func (s *Service) createVariantAndStock(
	ctx context.Context,
	orgID, productID, warehouseID int64,
	rawName, rawSKU, rawBarcode, rawUnit string,
	price, costPrice, discount money.Amount,
	qty, minThreshold, minOrderQty int,
	catAdapter CatalogAdapter,
	invAdapter InventoryAdapter,
) (*catalog.ProductVariant, error) {
	if catAdapter == nil {
		return nil, fmt.Errorf("catalog adapter unavailable")
	}

	if rawSKU == "" {
		rawSKU = fmt.Sprintf("SKU-%d-%d", productID, time.Now().UnixNano()%100000)
	}

	v := &catalog.ProductVariant{
		OrganizationID: orgID,
		ProductID:      productID,
		Name:           i18n.Text{"ar": rawName, "en": rawName},
		SKU:            rawSKU,
		Barcode:        rawBarcode,
		Price:          price,
		Discount:       discount,
		Unit:           rawUnit,
		MinOrderQty:    minOrderQty,
		Status:         catalog.StatusActive,
	}
	if !costPrice.IsZero() {
		v.CostPrice = &costPrice
	}

	created, err := catAdapter.CreateVariant(ctx, v)
	if err != nil {
		return nil, fmt.Errorf("create variant: %w", err)
	}

	if warehouseID > 0 && invAdapter != nil {
		stock := &inventory.Stock{
			OrganizationID:   orgID,
			WarehouseID:      warehouseID,
			ProductID:        productID,
			ProductVariantID: created.ID,
			Quantity:         qty,
			MinThreshold:     minThreshold,
		}
		_ = invAdapter.UpsertStock(ctx, stock)
	}

	return created, nil
}

func (s *Service) updateVariantAndStock(
	ctx context.Context,
	v *catalog.ProductVariant,
	warehouseID int64,
	price, costPrice, discount money.Amount,
	rawUnit, rawBarcode, rawSKU string,
	qty, minThreshold int,
	catAdapter CatalogAdapter,
	invAdapter InventoryAdapter,
) error {
	if catAdapter == nil {
		return fmt.Errorf("catalog adapter unavailable")
	}

	if price.IsPositive() {
		v.Price = price
	}
	if costPrice.IsPositive() {
		v.CostPrice = &costPrice
	}
	if !discount.IsNegative() {
		v.Discount = discount
	}
	if rawUnit != "" {
		v.Unit = rawUnit
	}
	if rawBarcode != "" {
		v.Barcode = rawBarcode
	}
	if rawSKU != "" {
		v.SKU = rawSKU
	}

	if _, err := catAdapter.UpdateVariant(ctx, v.ID, v); err != nil {
		return fmt.Errorf("update variant: %w", err)
	}

	if warehouseID > 0 && invAdapter != nil {
		stock := &inventory.Stock{
			OrganizationID:   v.OrganizationID,
			WarehouseID:      warehouseID,
			ProductID:        v.ProductID,
			ProductVariantID: v.ID,
			Quantity:         qty,
			MinThreshold:     minThreshold,
		}
		_ = invAdapter.UpsertStock(ctx, stock)
	}

	return nil
}
