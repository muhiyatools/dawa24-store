package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// MarketProducts searches the catalogue a pharmacy may buy from.
//
// This is the one pharmacy read that crosses tenants, and it does so
// deliberately: the marketplace exists to let a pharmacy see suppliers'
// catalogues, exactly as the market screen does (catalog/postgres does the same
// with the same AsSystem note). What keeps it honest is that only published,
// active, non-deleted rows are returned, and only the columns a buyer sees on
// that screen — never cost price, never stock, never another tenant's orders.
func (r *Repository) MarketProducts(
	ctx context.Context, actor authctx.Actor, q assistant.ProductQuery,
) (assistant.Page[assistant.MarketProductRow], error) {
	var empty assistant.Page[assistant.MarketProductRow]
	if _, err := scopeOf(actor); err != nil {
		return empty, err
	}

	term := strings.TrimSpace(q.Search)
	args := []any{term, q.Limit + 1, q.Offset}

	var out []assistant.MarketProductRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT p.id, `+nameExpr("p.name")+`,
			       COALESCE(`+nameExpr("org.name")+`,''),
			       p.price::text, p.discount::text,
			       COALESCE(p.unit,''), COALESCE(p.manufacturing_companies,''),
			       COALESCE(p.scientific_name,'')
			  FROM catalog.products p
			  LEFT JOIN org.organizations org ON org.id = p.organization_id
			 WHERE p.deleted_at IS NULL
			   AND p.status = 'active'
			   AND (
			        platform.normalize_arabic(p.name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%'
			     OR p.name->>'en' ILIKE '%' || $1 || '%'
			     OR COALESCE(p.sku,'') ILIKE '%' || $1 || '%'
			     OR COALESCE(p.barcode,'') ILIKE '%' || $1 || '%'
			     OR COALESCE(p.scientific_name,'') ILIKE '%' || $1 || '%'
			     OR COALESCE(p.active,'') ILIKE '%' || $1 || '%'
			   )
			 ORDER BY (p.discount > 0) DESC, p.price ASC, p.id ASC
			 LIMIT $2 OFFSET $3;
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.MarketProductRow
			var price, disc string
			if err := rows.Scan(&row.ID, &row.Name, &row.Supplier, &price, &disc,
				&row.Unit, &row.Company, &row.Scientific); err != nil {
				return err
			}
			row.Price, row.Discount = amount(price), amount(disc)
			final, subErr := row.Price.Sub(row.Discount)
			if subErr != nil || final.IsNegative() {
				final = row.Price
			}
			row.FinalPrice = final
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: market products: %w", err)
	}
	return pageOf(out, q.Limit, q.Offset), nil
}
