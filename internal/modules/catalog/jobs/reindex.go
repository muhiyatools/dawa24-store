package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
)

// ProductReindexWorker synchronizes the catalog.product_index read model asynchronously.
type ProductReindexWorker struct {
	river.WorkerDefaults[queue.ProductReindexArgs]
	db  *database.DB
	log *slog.Logger
}

// NewProductReindexWorker constructs the reindexing river worker.
func NewProductReindexWorker(db *database.DB, log *slog.Logger) *ProductReindexWorker {
	return &ProductReindexWorker{db: db, log: log}
}

// Work handles both single-product idempotent upserts and full catalogue reindexing sweeps.
func (w *ProductReindexWorker) Work(ctx context.Context, job *river.Job[queue.ProductReindexArgs]) error {
	pID := job.Args.ProductID
	w.log.InfoContext(ctx, "processing product reindex job", "product_id", pID)

	return w.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if pID == 0 {
			// Full catalogue sweep
			if _, err := tx.Exec(txCtx, `TRUNCATE TABLE catalog.product_index;`); err != nil {
				return fmt.Errorf("truncate product_index: %w", err)
			}
			query := `
				INSERT INTO catalog.product_index (
					unique_row_id, product_id, variant_id, sku, name_ar, name_en,
					search_text, search_ar, search_en, search_simple,
					organization_name, branch_city, scientific_name, price, discount,
					stock_quantity, category_id, brand_id, has_discount,
					discount_percentage, price_after_discount, organization_id, branch_id,
					status, product_type, institutional_work_ids
				)
				SELECT 
					'p_' || p.id::text AS unique_row_id,
					p.id AS product_id,
					NULL::bigint AS variant_id,
					p.sku,
					p.name->>'ar' AS name_ar,
					p.name->>'en' AS name_en,
					CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, ''), COALESCE(p.sku, '')) AS search_text,
					CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, '')) AS search_ar,
					CONCAT_WS(' ', COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, '')) AS search_en,
					CONCAT_WS(' ', platform.normalize_arabic(COALESCE(p.name->>'ar', '')), COALESCE(p.name->>'en', ''), COALESCE(p.sku, '')) AS search_simple,
					o.legal_name AS organization_name,
					b.city AS branch_city,
					p.scientific_name,
					p.price,
					p.discount,
					0 AS stock_quantity,
					p.category_id,
					p.brand_id,
					(p.discount > 0) AS has_discount,
					p.discount AS discount_percentage,
					ROUND(p.price * (1 - p.discount / 100), 2) AS price_after_discount,
					p.organization_id,
					p.branch_id,
					p.status::text AS status,
					'parent' AS product_type,
					COALESCE(p.institutional_work_ids, '{}'::bigint[])
				FROM catalog.products p
				JOIN org.organizations o ON p.organization_id = o.id
				LEFT JOIN org.branches b ON p.branch_id = b.id
				WHERE p.deleted_at IS NULL AND o.deleted_at IS NULL AND p.status = 'active';
			`
			_, err := tx.Exec(txCtx, query)
			return err
		}

		// Single product upsert
		query := `
			INSERT INTO catalog.product_index (
				unique_row_id, product_id, variant_id, sku, name_ar, name_en,
				search_text, search_ar, search_en, search_simple,
				organization_name, branch_city, scientific_name, price, discount,
				stock_quantity, category_id, brand_id, has_discount,
				discount_percentage, price_after_discount, organization_id, branch_id,
				status, product_type, institutional_work_ids, updated_at
			)
			SELECT 
				'p_' || p.id::text AS unique_row_id,
				p.id AS product_id,
				NULL::bigint AS variant_id,
				p.sku,
				p.name->>'ar' AS name_ar,
				p.name->>'en' AS name_en,
				CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, ''), COALESCE(p.sku, '')) AS search_text,
				CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, '')) AS search_ar,
				CONCAT_WS(' ', COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, '')) AS search_en,
				CONCAT_WS(' ', platform.normalize_arabic(COALESCE(p.name->>'ar', '')), COALESCE(p.name->>'en', ''), COALESCE(p.sku, '')) AS search_simple,
				o.legal_name AS organization_name,
				b.city AS branch_city,
				p.scientific_name,
				p.price,
				p.discount,
				0 AS stock_quantity,
				p.category_id,
				p.brand_id,
				(p.discount > 0) AS has_discount,
				p.discount AS discount_percentage,
				ROUND(p.price * (1 - p.discount / 100), 2) AS price_after_discount,
				p.organization_id,
				p.branch_id,
				p.status::text AS status,
				'parent' AS product_type,
				COALESCE(p.institutional_work_ids, '{}'::bigint[]),
				now()
			FROM catalog.products p
			JOIN org.organizations o ON p.organization_id = o.id
			LEFT JOIN org.branches b ON p.branch_id = b.id
			WHERE p.id = $1 AND p.deleted_at IS NULL AND o.deleted_at IS NULL
			ON CONFLICT (unique_row_id) DO UPDATE SET
				sku = EXCLUDED.sku,
				name_ar = EXCLUDED.name_ar,
				name_en = EXCLUDED.name_en,
				search_text = EXCLUDED.search_text,
				search_ar = EXCLUDED.search_ar,
				search_en = EXCLUDED.search_en,
				search_simple = EXCLUDED.search_simple,
				organization_name = EXCLUDED.organization_name,
				branch_city = EXCLUDED.branch_city,
				scientific_name = EXCLUDED.scientific_name,
				price = EXCLUDED.price,
				discount = EXCLUDED.discount,
				stock_quantity = EXCLUDED.stock_quantity,
				category_id = EXCLUDED.category_id,
				brand_id = EXCLUDED.brand_id,
				has_discount = EXCLUDED.has_discount,
				discount_percentage = EXCLUDED.discount_percentage,
				price_after_discount = EXCLUDED.price_after_discount,
				organization_id = EXCLUDED.organization_id,
				branch_id = EXCLUDED.branch_id,
				status = EXCLUDED.status,
				product_type = EXCLUDED.product_type,
				institutional_work_ids = EXCLUDED.institutional_work_ids,
				updated_at = now();
		`
		_, err := tx.Exec(txCtx, query, pID)
		return err
	})
}
