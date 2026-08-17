package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListContentBlocks returns all CMS blocks ordered by sort order.
func (r *Repository) ListContentBlocks(ctx context.Context) ([]*platformadmin.ContentBlock, error) {
	var list []*platformadmin.ContentBlock
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, key, title, body, position, sort_order, is_active, updated_at
			FROM platform_admin.content_blocks
			ORDER BY sort_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b platformadmin.ContentBlock
			if err := rows.Scan(&b.ID, &b.Key, &b.Title, &b.Body, &b.Position, &b.SortOrder, &b.IsActive, &b.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &b)
		}
		return rows.Err()
	})
	return list, err
}

// GetContentBlockByKey returns one active CMS block by key.
func (r *Repository) GetContentBlockByKey(ctx context.Context, key string) (*platformadmin.ContentBlock, error) {
	var b platformadmin.ContentBlock
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, key, title, body, position, sort_order, is_active, updated_at
			FROM platform_admin.content_blocks
			WHERE key = $1 AND is_active = true
			ORDER BY id ASC LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, key).Scan(&b.ID, &b.Key, &b.Title, &b.Body, &b.Position, &b.SortOrder, &b.IsActive, &b.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("content_block")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// UpsertContentBlock creates or updates a CMS block by key.
func (r *Repository) UpsertContentBlock(ctx context.Context, b *platformadmin.ContentBlock) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO platform_admin.content_blocks (key, title, body, position, sort_order, is_active, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (key) DO UPDATE SET
				title = EXCLUDED.title, body = EXCLUDED.body, position = EXCLUDED.position,
				sort_order = EXCLUDED.sort_order, is_active = EXCLUDED.is_active, updated_at = now()
			RETURNING id, updated_at;
		`
		return tx.QueryRow(txCtx, query, b.Key, b.Title, b.Body, b.Position, b.SortOrder, b.IsActive).Scan(&b.ID, &b.UpdatedAt)
	})
}

// GetPublishedPolicy returns the newest published policy for a slug.
func (r *Repository) GetPublishedPolicy(ctx context.Context, slug string) (*platformadmin.PrivacyPolicy, error) {
	var p platformadmin.PrivacyPolicy
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, slug, title, content, is_published, version, published_at, updated_at
			FROM platform_admin.privacy_policies
			WHERE slug = $1 AND is_published = true
			ORDER BY version DESC LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, slug).Scan(&p.ID, &p.Slug, &p.Title, &p.Content, &p.IsPublished, &p.Version, &p.PublishedAt, &p.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("policy")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}
