-- name: GetOrCreateCart :one
INSERT INTO commerce.carts (user_id)
VALUES ($1)
ON CONFLICT (user_id) DO UPDATE SET updated_at = now()
RETURNING id, public_id, user_id, organization_id, created_at, updated_at;

-- name: GetCartItems :many
SELECT id, cart_id, product_id, product_variant_id, quantity, unit_price, created_at, updated_at
FROM commerce.cart_items
WHERE cart_id = $1
ORDER BY id ASC;

-- name: UpsertCartItem :exec
INSERT INTO commerce.cart_items (cart_id, product_id, product_variant_id, quantity, unit_price)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (cart_id, product_variant_id) DO UPDATE SET
    quantity = EXCLUDED.quantity,
    unit_price = EXCLUDED.unit_price,
    updated_at = now();

-- name: DeleteCartItem :exec
DELETE FROM commerce.cart_items
WHERE cart_id = $1 AND product_variant_id = $2;

-- name: ClearCartItems :exec
DELETE FROM commerce.cart_items
WHERE cart_id = $1;
