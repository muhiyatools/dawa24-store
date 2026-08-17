package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
)

// CreateHighlightSection inserts a merchandising row.
func (r *Repository) CreateHighlightSection(ctx context.Context, s *org.HighlightSection) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO org.highlight_sections (organization_id, title, slug, display_order, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, s.OrganizationID, s.Title, s.Slug, s.DisplayOrder, s.IsActive).
			Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	})
}

// ListHighlightSections returns an organization's merchandising rows.
func (r *Repository) ListHighlightSections(ctx context.Context, orgID int64) ([]*org.HighlightSection, error) {
	var list []*org.HighlightSection
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, organization_id, title, slug, display_order, is_active, created_at, updated_at
			FROM org.highlight_sections
			WHERE organization_id = $1
			ORDER BY display_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s org.HighlightSection
			if err := rows.Scan(&s.ID, &s.OrganizationID, &s.Title, &s.Slug, &s.DisplayOrder, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// AddHighlightItem inserts a product or offer into a section.
func (r *Repository) AddHighlightItem(ctx context.Context, item *org.HighlightSectionItem) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO org.highlight_section_items (section_id, product_id, offer_id, display_order)
			VALUES ($1, $2, $3, $4)
			RETURNING id;
		`
		return tx.QueryRow(txCtx, query, item.SectionID, item.ProductID, item.OfferID, item.DisplayOrder).Scan(&item.ID)
	})
}

// ListHighlightItems returns a section's items.
func (r *Repository) ListHighlightItems(ctx context.Context, sectionID int64) ([]*org.HighlightSectionItem, error) {
	var list []*org.HighlightSectionItem
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, section_id, product_id, offer_id, display_order
			FROM org.highlight_section_items
			WHERE section_id = $1
			ORDER BY display_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, sectionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it org.HighlightSectionItem
			if err := rows.Scan(&it.ID, &it.SectionID, &it.ProductID, &it.OfferID, &it.DisplayOrder); err != nil {
				return err
			}
			list = append(list, &it)
		}
		return rows.Err()
	})
	return list, err
}
