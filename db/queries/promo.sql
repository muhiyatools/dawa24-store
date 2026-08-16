-- name: CreateOffer :one
INSERT INTO promo.offers (
    organization_id, title, description, discount_type, discount_value, min_order_value, starts_at, expires_at, is_active
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, public_id, created_at, updated_at;

-- name: GetOfferByID :one
SELECT id, public_id, organization_id, title, description, discount_type, discount_value,
       min_order_value, starts_at, expires_at, is_active, views_count, clicks_count,
       created_at, updated_at, deleted_at
FROM promo.offers
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListActiveOffers :many
SELECT id, public_id, organization_id, title, description, discount_type, discount_value,
       min_order_value, starts_at, expires_at, is_active, views_count, clicks_count,
       created_at, updated_at, deleted_at
FROM promo.offers
WHERE is_active = true AND starts_at <= now() AND expires_at >= now() AND deleted_at IS NULL
ORDER BY id DESC
LIMIT $1 OFFSET $2;
