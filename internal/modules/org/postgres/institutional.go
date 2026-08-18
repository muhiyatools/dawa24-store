package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func ensureInstitutionalTables(ctx context.Context, tx pgx.Tx) error {
	const schema = `
		CREATE TABLE IF NOT EXISTS org.institutional_works (
			id BIGSERIAL PRIMARY KEY,
			public_id UUID NOT NULL DEFAULT gen_random_uuid(),
			title JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
			description JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
			icon TEXT NOT NULL DEFAULT '',
			pricing_type TEXT NOT NULL DEFAULT 'free',
			is_active BOOLEAN NOT NULL DEFAULT true,
			view_type INT NOT NULL DEFAULT 1,
			slug TEXT NOT NULL DEFAULT '',
			parent_id BIGINT REFERENCES org.institutional_works(id) ON DELETE SET NULL,
			sort_order INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			deleted_at TIMESTAMPTZ
		);

		CREATE TABLE IF NOT EXISTS org.branch_institutional_works (
			id BIGSERIAL PRIMARY KEY,
			branch_id BIGINT NOT NULL,
			institutional_work_id BIGINT NOT NULL REFERENCES org.institutional_works(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (branch_id, institutional_work_id)
		);
	`
	_, err := tx.Exec(ctx, schema)
	return err
}

func seedDefaultInstitutionalWorks(ctx context.Context, tx pgx.Tx) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM org.institutional_works WHERE deleted_at IS NULL;`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaults := []struct {
		titleAr     string
		titleEn     string
		descAr      string
		pricingType string
		icon        string
		children    []struct {
			titleAr  string
			titleEn  string
			descAr   string
			pricing  string
			icon     string
		}
	}{
		{
			titleAr:     "مستودعات وشركات التوزيع الدوائي",
			titleEn:     "Pharmaceutical Distributors & Hubs",
			descAr:      "الشركات والمستودعات المرخصة لتوزيع الأدوية والمستلزمات الطبية بالجملة",
			pricingType: "subscription",
			icon:        "truck",
			children: []struct {
				titleAr string
				titleEn string
				descAr  string
				pricing string
				icon    string
			}{
				{titleAr: "مستودع رئيسي مركزي", titleEn: "Central Main Warehouse", descAr: "مستودع التخزين والتوريد الرئيسي المركزي", pricing: "paid", icon: "building"},
				{titleAr: "فرع مستودع إقليمي / محافظات", titleEn: "Regional Distribution Branch", descAr: "فرع توزيع مخصص للمحافظات والمناطق الإقليمية", pricing: "subscription", icon: "layers"},
				{titleAr: "مركز توزيع وتوصيل مبرد فوري", titleEn: "Cold-Chain Rapid Depot", descAr: "مستودع متخصص في الأدوية المبردة (2-8 درجات) والتوصيل السريع", pricing: "monthly", icon: "shield"},
			},
		},
		{
			titleAr:     "الصيدليات ومراكز الصرف الدوائي",
			titleEn:     "Pharmacies & Healthcare Outlets",
			descAr:      "الصيدليات المجتمعية وسلاسل الصيدليات المرخصة بوزارة الصحة وهيئة الدواء",
			pricingType: "free",
			icon:        "pill",
			children: []struct {
				titleAr string
				titleEn string
				descAr  string
				pricing string
				icon    string
			}{
				{titleAr: "صيدلية مجتمعية مستقلة", titleEn: "Community Pharmacy", descAr: "صيدلية فردية أهلية مرخصة", pricing: "free", icon: "pill"},
				{titleAr: "فرع سلسلة صيدليات", titleEn: "Pharmacy Chain Branch", descAr: "فرع تابع لسلسلة صيدليات مركزية", pricing: "subscription", icon: "building"},
				{titleAr: "صيدلية مستشفى ومجمع عيادات", titleEn: "Hospital Pharmacy", descAr: "صيدلية مخصصة للمستشفيات والمراكز التخصصية", pricing: "per_project", icon: "shield"},
			},
		},
		{
			titleAr:     "المصانع والشركات المنتجة والوكالات",
			titleEn:     "Manufacturers & Official Agencies",
			descAr:      "مصانع الأدوية المحلية والوكلاء الرسميون والمكاتب العلمية المعتمدة",
			pricingType: "subscription",
			icon:        "package",
			children: []struct {
				titleAr string
				titleEn string
				descAr  string
				pricing string
				icon    string
			}{
				{titleAr: "مصنع أدوية محلي معتمد EDA", titleEn: "EDA Approved Manufacturer", descAr: "مصنع مرخص لإنتاج المستحضرات الصيدلية محلياً", pricing: "subscription", icon: "building"},
				{titleAr: "وكيل ومستورد رسمي", titleEn: "Official Importer & Agency", descAr: "وكيل معتمد لاستيراد الأدوية والأجهزة الطبية", pricing: "paid", icon: "tag"},
				{titleAr: "مكتب علمي ومبيعات", titleEn: "Scientific Office", descAr: "مكتب علمي مسؤول عن الترويج والتسجيل والدعم الطبي", pricing: "monthly", icon: "file"},
			},
		},
	}

	for _, g := range defaults {
		var parentID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug)
			VALUES ($1, $2, $3, $4, true, 1, $5)
			RETURNING id;
		`, i18n.New(g.titleAr, g.titleEn), i18n.New(g.descAr, ""), g.icon, g.pricingType, g.titleEn).Scan(&parentID)
		if err != nil {
			return err
		}

		for _, c := range g.children {
			_, err = tx.Exec(ctx, `
				INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug, parent_id)
				VALUES ($1, $2, $3, $4, true, 2, $5, $6);
			`, i18n.New(c.titleAr, c.titleEn), i18n.New(c.descAr, ""), c.icon, c.pricing, c.titleEn, parentID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateInstitutionalWork creates a new institutional structure category.
func (r *Repository) CreateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		const query = `
			INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug, parent_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			iw.Title, iw.Description, iw.Icon, string(iw.PricingType), iw.IsActive, iw.ViewType, iw.Slug, iw.ParentID,
		).Scan(&iw.ID, &iw.PublicID, &iw.CreatedAt, &iw.UpdatedAt)
	})
}

// GetInstitutionalWorkByID retrieves an institutional work by its primary key.
func (r *Repository) GetInstitutionalWorkByID(ctx context.Context, id int64) (*org.InstitutionalWork, error) {
	var iw org.InstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		const query = `
			SELECT id, public_id, title, description, icon, pricing_type, is_active, view_type, slug, parent_id, created_at, updated_at
			FROM org.institutional_works
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var pricingStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&iw.ID, &iw.PublicID, &iw.Title, &iw.Description, &iw.Icon, &pricingStr, &iw.IsActive, &iw.ViewType, &iw.Slug, &iw.ParentID, &iw.CreatedAt, &iw.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("institutional_work")
			}
			return err
		}
		iw.PricingType = org.PricingType(pricingStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &iw, nil
}

// UpdateInstitutionalWork updates an existing institutional category.
func (r *Repository) UpdateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		const query = `
			UPDATE org.institutional_works
			SET title = $2, description = $3, icon = $4, pricing_type = $5,
			    is_active = $6, view_type = $7, slug = $8, parent_id = $9, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, query,
			iw.ID, iw.Title, iw.Description, iw.Icon, string(iw.PricingType), iw.IsActive, iw.ViewType, iw.Slug, iw.ParentID,
		)
		if err != nil {
			return fmt.Errorf("update institutional work: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("institutional_work")
		}
		return nil
	})
}

// DeleteInstitutionalWork soft-deletes an institutional category.
func (r *Repository) DeleteInstitutionalWork(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		const query = `UPDATE org.institutional_works SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		tag, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("institutional_work")
		}
		return nil
	})
}

// ToggleInstitutionalWorkStatus toggles the active state of an institutional category.
func (r *Repository) ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		const query = `UPDATE org.institutional_works SET is_active = NOT is_active, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		tag, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("institutional_work")
		}
		return nil
	})
}

// ListInstitutionalWorks returns all institutional categories formatted with parent-child hierarchy.
func (r *Repository) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	var all []*org.InstitutionalWork
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		if err := seedDefaultInstitutionalWorks(txCtx, tx); err != nil {
			return err
		}

		query := `
			SELECT iw.id, iw.public_id, iw.title, iw.description, iw.icon, iw.pricing_type,
			       iw.is_active, iw.view_type, iw.slug, iw.parent_id,
			       COALESCE(p.title->>'ar', p.title->>'en', '') AS parent_title,
			       (SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id = iw.id) AS branch_count,
			       iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			LEFT JOIN org.institutional_works p ON iw.parent_id = p.id
			WHERE iw.deleted_at IS NULL
		`
		if onlyActive {
			query += " AND iw.is_active = true"
		}
		query += " ORDER BY iw.parent_id NULLS FIRST, iw.sort_order ASC, iw.id ASC;"

		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item org.InstitutionalWork
			var pricingStr string
			if err := rows.Scan(
				&item.ID, &item.PublicID, &item.Title, &item.Description, &item.Icon,
				&pricingStr, &item.IsActive, &item.ViewType, &item.Slug, &item.ParentID,
				&item.ParentTitle, &item.BranchCount, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			item.PricingType = org.PricingType(pricingStr)
			all = append(all, &item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	// Organize into Parents with Children
	parentMap := make(map[int64]*org.InstitutionalWork)
	var rootList []*org.InstitutionalWork

	for _, item := range all {
		if item.ParentID == nil || *item.ParentID == 0 {
			parentMap[item.ID] = item
			rootList = append(rootList, item)
		}
	}

	for _, item := range all {
		if item.ParentID != nil && *item.ParentID > 0 {
			if parent, exists := parentMap[*item.ParentID]; exists {
				parent.Children = append(parent.Children, item)
			} else {
				rootList = append(rootList, item)
			}
		}
	}

	return rootList, nil
}

// AssignBranchInstitutionalWorks sets the institutional work categories for a branch.
func (r *Repository) AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM org.branch_institutional_works WHERE branch_id = $1;`, branchID); err != nil {
			return err
		}
		for _, wID := range workIDs {
			if wID > 0 {
				if _, err := tx.Exec(txCtx, `
					INSERT INTO org.branch_institutional_works (branch_id, institutional_work_id)
					VALUES ($1, $2) ON CONFLICT (branch_id, institutional_work_id) DO NOTHING;
				`, branchID, wID); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// GetBranchInstitutionalWorks retrieves all institutional works assigned to a branch.
func (r *Repository) GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	var list []*org.InstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		const query = `
			SELECT iw.id, iw.public_id, iw.title, iw.description, iw.icon, iw.pricing_type,
			       iw.is_active, iw.view_type, iw.slug, iw.parent_id, '', 0, iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			JOIN org.branch_institutional_works biw ON iw.id = biw.institutional_work_id
			WHERE biw.branch_id = $1 AND iw.deleted_at IS NULL;
		`
		rows, err := tx.Query(txCtx, query, branchID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item org.InstitutionalWork
			var pricingStr string
			if err := rows.Scan(
				&item.ID, &item.PublicID, &item.Title, &item.Description, &item.Icon,
				&pricingStr, &item.IsActive, &item.ViewType, &item.Slug, &item.ParentID,
				&item.ParentTitle, &item.BranchCount, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			item.PricingType = org.PricingType(pricingStr)
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}
