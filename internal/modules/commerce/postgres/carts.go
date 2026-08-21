package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// GetOrCreateCart retrieves an existing active cart or initializes a new one.
func (r *Repository) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	var c commerce.Cart
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO commerce.carts (user_id)
			VALUES ($1)
			ON CONFLICT (user_id) DO UPDATE SET updated_at = now()
			RETURNING id, public_id, user_id, organization_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, userID).Scan(
			&c.ID, &c.PublicID, &c.UserID, &c.OrganizationID, &c.CreatedAt, &c.UpdatedAt,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: get or create cart: %w", err)
	}
	return &c, nil
}

// GetCartWithItems loads the cart along with all its active line items.
func (r *Repository) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	var c commerce.Cart
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryCart := `SELECT id, public_id, user_id, organization_id, created_at, updated_at FROM commerce.carts WHERE id = $1;`
		if err := tx.QueryRow(txCtx, queryCart, cartID).Scan(
			&c.ID, &c.PublicID, &c.UserID, &c.OrganizationID, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("cart")
			}
			return err
		}

		queryItems := `
			SELECT ci.id, ci.cart_id, ci.product_id, ci.product_variant_id, ci.quantity, ci.unit_price,
			       ci.offer_id, ci.created_at, ci.updated_at,
			       COALESCE(
			           po.organization_id,
			           pv.organization_id,
			           (
			               SELECT w.organization_id 
			               FROM inventory.stocks s 
			               JOIN inventory.warehouses w ON w.id = s.warehouse_id 
			               WHERE s.product_variant_id = ci.product_variant_id AND s.deleted_at IS NULL 
			               LIMIT 1
			           ),
			           0
			       ),
			       COALESCE(p.name, '{"ar":"","en":""}'::jsonb),
			       COALESCE(o.name, '{"ar":"","en":""}'::jsonb),
			       COALESCE(o.min_order_price, 10.00),
			       COALESCE((
			           SELECT SUM(s.quantity)
			           FROM inventory.stocks s
			           WHERE s.product_variant_id = ci.product_variant_id
			             AND s.deleted_at IS NULL
			       ), 999) AS available_stock
			FROM commerce.cart_items ci
			LEFT JOIN catalog.products p ON p.id = ci.product_id
			LEFT JOIN catalog.product_variants pv ON pv.id = ci.product_variant_id
			LEFT JOIN promo.offers po ON po.id = ci.offer_id
			LEFT JOIN org.organizations o ON o.id = COALESCE(
			    po.organization_id,
			    pv.organization_id,
			    (
			        SELECT w.organization_id 
			        FROM inventory.stocks s 
			        JOIN inventory.warehouses w ON w.id = s.warehouse_id 
			        WHERE s.product_variant_id = ci.product_variant_id AND s.deleted_at IS NULL 
			        LIMIT 1
			    )
			)
			WHERE ci.cart_id = $1
			ORDER BY ci.id ASC;
		`
		rows, err := tx.Query(txCtx, queryItems, cartID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item commerce.CartItem
			if err := rows.Scan(
				&item.ID, &item.CartID, &item.ProductID, &item.ProductVariantID,
				&item.Quantity, &item.UnitPrice, &item.OfferID,
				&item.CreatedAt, &item.UpdatedAt,
				&item.OrganizationID, &item.ProductName, &item.SupplierName, &item.MinOrderPrice,
				&item.AvailableStock,
			); err != nil {
				return err
			}
			c.Items = append(c.Items, &item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// AddToCartItem adds or updates a product variant in the cart.
func (r *Repository) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO commerce.cart_items (cart_id, product_id, product_variant_id, quantity, unit_price, offer_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (cart_id, product_variant_id) DO UPDATE SET
				quantity = commerce.cart_items.quantity + EXCLUDED.quantity,
				unit_price = EXCLUDED.unit_price,
				offer_id = COALESCE(EXCLUDED.offer_id, commerce.cart_items.offer_id),
				updated_at = now();
		`
		_, err := tx.Exec(txCtx, query, cartID, item.ProductID, item.ProductVariantID, item.Quantity, item.UnitPrice, item.OfferID)
		return err
	})
}

// RemoveCartItem deletes a single variant from the cart.
func (r *Repository) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM commerce.cart_items WHERE cart_id = $1 AND product_variant_id = $2;`
		_, err := tx.Exec(txCtx, query, cartID, variantID)
		return err
	})
}

// ClearCart empties all items from the cart.
func (r *Repository) ClearCart(ctx context.Context, cartID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM commerce.cart_items WHERE cart_id = $1;`
		_, err := tx.Exec(txCtx, query, cartID)
		return err
	})
}
