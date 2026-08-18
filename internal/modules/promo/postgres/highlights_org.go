package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListHighlightSectionsByOrg returns an organization's own storefront
// merchandising rows (066 — org.highlight_sections merged into promo).
func (r *Repository) ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*promo.HighlightSection, error) {
	var list []*promo.HighlightSection
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, title, slug, display_order, is_active, owner_type, organization_id, created_at
			FROM promo.highlight_sections
			WHERE owner_type = 'organization' AND organization_id = $1
			ORDER BY display_order ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var h promo.HighlightSection
			if err := rows.Scan(&h.ID, &h.PublicID, &h.Title, &h.Slug, &h.DisplayOrder, &h.IsActive, &h.OwnerType, &h.OrganizationID, &h.CreatedAt); err != nil {
				return err
			}
			list = append(list, &h)
		}
		return rows.Err()
	})
	return list, err
}

// AddHighlightItem links a product to a storefront merchandising section.
func (r *Repository) AddHighlightItem(ctx context.Context, item *promo.HighlightSectionItem) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.highlight_section_items (section_id, product_id, offer_id)
			VALUES ($1, $2, $3)
			RETURNING id;
		`
		return tx.QueryRow(txCtx, query, item.SectionID, item.ProductID, item.OfferID).
			Scan(&item.ID)
	})
}

// ListHighlightItems returns a section's items ordered by display order.
func (r *Repository) ListHighlightItems(ctx context.Context, sectionID int64) ([]*promo.HighlightSectionItem, error) {
	var list []*promo.HighlightSectionItem
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, section_id, product_id, offer_id
			FROM promo.highlight_section_items
			WHERE section_id = $1
			ORDER BY display_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, sectionID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item promo.HighlightSectionItem
			if err := rows.Scan(&item.ID, &item.SectionID, &item.ProductID, &item.OfferID); err != nil {
				return err
			}
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}