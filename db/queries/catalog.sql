-- name: CreateProduct :one
INSERT INTO catalog.products (
    organization_id, category_id, brand_id, branch_id, name, description, sku, barcode,
    price, discount, old_price, image, image_link, status, is_featured, dosage_form,
    scientific_name, pharmacology, active, concentration, unit, manufacturing_companies
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
) RETURNING id, public_id, created_at, updated_at;

-- name: GetProductByID :one
SELECT id, public_id, organization_id, category_id, brand_id, branch_id, name, description,
       sku, barcode, price, discount, old_price, image, image_link, status, sold_times,
       is_featured, dosage_form, scientific_name, pharmacology, active, concentration,
       unit, manufacturing_companies, created_at, updated_at, deleted_at
FROM catalog.products
WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateProductVariant :one
INSERT INTO catalog.product_variants (
    organization_id, product_id, name, sku, barcode, price, cost_price, discount, unit, image, status, is_featured
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
) RETURNING id, public_id, created_at, updated_at;

-- name: ListVariantsByProduct :many
SELECT id, public_id, organization_id, product_id, name, sku, barcode, price, cost_price,
       discount, unit, image, status, is_featured, created_at, updated_at, deleted_at
FROM catalog.product_variants
WHERE product_id = $1 AND deleted_at IS NULL;

-- name: CreateCategory :one
INSERT INTO catalog.categories (
    parent_id, name, description, icon, image, status, sort_order
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING id, public_id, created_at, updated_at;

-- name: ListCategories :many
SELECT id, public_id, parent_id, name, description, icon, image, status, sort_order, created_at, updated_at, deleted_at
FROM catalog.categories
WHERE deleted_at IS NULL
ORDER BY sort_order ASC, name->>'ar' ASC;
