-- name: CreateWarehouse :one
INSERT INTO inventory.warehouses (
    organization_id, branch_id, name, code, address, phone, latitude, longitude, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING id, public_id, created_at, updated_at;

-- name: GetWarehouseByID :one
SELECT id, public_id, organization_id, branch_id, name, code, address, phone,
       latitude, longitude, is_active, created_at, updated_at, deleted_at
FROM inventory.warehouses
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListWarehouses :many
SELECT id, public_id, organization_id, branch_id, name, code, address, phone,
       latitude, longitude, is_active, created_at, updated_at, deleted_at
FROM inventory.warehouses
WHERE deleted_at IS NULL
ORDER BY name ASC;

-- name: GetStockByWarehouseAndVariant :one
SELECT id, organization_id, warehouse_id, product_id, product_variant_id,
       quantity, min_threshold, negotiation, created_at, updated_at, deleted_at
FROM inventory.stocks
WHERE warehouse_id = $1 AND product_variant_id = $2 AND deleted_at IS NULL;

-- name: ListStocksByWarehouse :many
SELECT id, organization_id, warehouse_id, product_id, product_variant_id,
       quantity, min_threshold, negotiation, created_at, updated_at, deleted_at
FROM inventory.stocks
WHERE warehouse_id = $1 AND deleted_at IS NULL
ORDER BY id ASC;
