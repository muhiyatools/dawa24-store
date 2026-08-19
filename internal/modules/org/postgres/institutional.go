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
			icon TEXT NOT NULL DEFAULT 'building',
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

		CREATE TABLE IF NOT EXISTS org.institutional_work_connections (
			id BIGSERIAL PRIMARY KEY,
			from_institutional_work_id BIGINT NOT NULL REFERENCES org.institutional_works(id) ON DELETE CASCADE,
			to_institutional_work_id BIGINT NOT NULL REFERENCES org.institutional_works(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			CONSTRAINT uq_inst_work_conn UNIQUE (from_institutional_work_id, to_institutional_work_id)
		);

		CREATE INDEX IF NOT EXISTS idx_inst_work_conn_from ON org.institutional_work_connections (from_institutional_work_id);
		CREATE INDEX IF NOT EXISTS idx_inst_work_conn_to ON org.institutional_work_connections (to_institutional_work_id);

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_schema = 'org' AND table_name = 'branch_institutional_works' AND column_name = 'institutional_work_id'
			) THEN
				ALTER TABLE org.branch_institutional_works ADD COLUMN institutional_work_id BIGINT REFERENCES org.institutional_works(id) ON DELETE CASCADE;
			END IF;
		END $$;
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
		descEn      string
		pricingType string
		icon        string
		viewType    int
		children    []struct {
			titleAr  string
			titleEn  string
			descAr   string
			descEn   string
			pricing  string
			icon     string
			viewType int
		}
	}{
		{
			titleAr:     "جملة جملة",
			titleEn:     "Wholesale - Wholesale",
			descAr:      "كبار المستودعات وشركات التوزيع الدوائي المركزية للتوريد بالجملة الكبرى",
			descEn:      "Primary large-scale pharmaceutical wholesalers and master hubs",
			pricingType: "subscription",
			icon:        "truck",
			viewType:    1,
			children: []struct {
				titleAr  string
				titleEn  string
				descAr   string
				descEn   string
				pricing  string
				icon     string
				viewType int
			}{
				{
					titleAr:  "قطاع",
					titleEn:  "Sector",
					descAr:   "القطاعات الجغرافية والتخصصية لتوزيع المستحضرات",
					descEn:   "Specialized geographical distribution sector",
					pricing:  "subscription",
					icon:     "layers",
					viewType: 1,
				},
				{
					titleAr:  "مصنع",
					titleEn:  "Factory",
					descAr:   "مصانع الأدوية والشركات المصنعة المحلية والدولية",
					descEn:   "Pharmaceutical manufacturing plants and laboratories",
					pricing:  "paid",
					icon:     "package",
					viewType: 1,
				},
			},
		},
		{
			titleAr:     "تجزئة",
			titleEn:     "Retail",
			descAr:      "صيدليات ومنافذ البيع والتوزيع المباشر للجمهور",
			descEn:      "Retail pharmacies and direct consumer healthcare outlets",
			pricingType: "free",
			icon:        "pill",
			viewType:    1,
			children: []struct {
				titleAr  string
				titleEn  string
				descAr   string
				descEn   string
				pricing  string
				icon     string
				viewType int
			}{
				{
					titleAr:  "صيدلية",
					titleEn:  "Pharmacy",
					descAr:   "صيدلية مرخصة لصرف الأدوية والمستلزمات الطبية",
					descEn:   "Licensed community dispensing pharmacy",
					pricing:  "free",
					icon:     "pill",
					viewType: 2,
				},
				{
					titleAr:  "فئة الجمهور",
					titleEn:  "Audience Category",
					descAr:   "منافذ البيع المباشر المخصصة لخدمة شرائح الجمهور",
					descEn:   "Direct customer retail category",
					pricing:  "free",
					icon:     "cart",
					viewType: 1,
				},
			},
		},
		{
			titleAr:     "خدمات",
			titleEn:     "Services",
			descAr:      "الخدمات اللوجستية، سلاسل التبريد، الاستشارات والحلول الصيدلانية",
			descEn:      "Pharma cold-chain logistics, technical consultancy, and enterprise services",
			pricingType: "monthly",
			icon:        "shield",
			viewType:    1,
			children: []struct {
				titleAr  string
				titleEn  string
				descAr   string
				descEn   string
				pricing  string
				icon     string
				viewType int
			}{
				{
					titleAr:  "شركة مساهمة",
					titleEn:  "Joint-Stock Company",
					descAr:   "الشركات المساهمة الكبرى والمؤسسات الاعتبارية",
					descEn:   "Corporate joint-stock healthcare enterprises",
					pricing:  "subscription",
					icon:     "briefcase",
					viewType: 1,
				},
				{
					titleAr:  "مؤسسة فردية",
					titleEn:  "Sole Proprietorship",
					descAr:   "المؤسسات الفردية والمكاتب العلمية التخصصية",
					descEn:   "Single proprietor medical agencies and scientific offices",
					pricing:  "paid",
					icon:     "building",
					viewType: 1,
				},
				{
					titleAr:  "تعاونيات",
					titleEn:  "Cooperatives",
					descAr:   "الجمعيات التعاونية الصيدلانية والمهنية",
					descEn:   "Healthcare & pharmaceutical cooperatives",
					pricing:  "per_project",
					icon:     "users",
					viewType: 1,
				},
				{
					titleAr:  "شركات ناشئة",
					titleEn:  "Startups",
					descAr:   "الشركات الناشئة في التكنولوجيا الطبية والرعاية الصحية",
					descEn:   "HealthTech and pharmacy supply chain startups",
					pricing:  "free",
					icon:     "tag",
					viewType: 1,
				},
				{
					titleAr:  "منظمة غير ربحية",
					titleEn:  "Non-profit Organization",
					descAr:   "الجمعيات الخيرية والمنظمات الإغاثية والصحية غير الهادفة للربح",
					descEn:   "Non-profit healthcare and humanitarian medical organizations",
					pricing:  "free",
					icon:     "shield",
					viewType: 1,
				},
			},
		},
	}

	idMap := make(map[string]int64)

	for _, g := range defaults {
		var parentID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug)
			VALUES ($1, $2, $3, $4, true, $5, $6)
			RETURNING id;
		`, i18n.New(g.titleAr, g.titleEn), i18n.New(g.descAr, g.descEn), g.icon, g.pricingType, g.viewType, g.titleEn).Scan(&parentID)
		if err != nil {
			return err
		}
		idMap[g.titleEn] = parentID

		for _, c := range g.children {
			var childID int64
			err = tx.QueryRow(ctx, `
				INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug, parent_id)
				VALUES ($1, $2, $3, $4, true, $5, $6, $7)
				RETURNING id;
			`, i18n.New(c.titleAr, c.titleEn), i18n.New(c.descAr, c.descEn), c.icon, c.pricing, c.viewType, c.titleEn, parentID).Scan(&childID)
			if err != nil {
				return err
			}
			idMap[c.titleEn] = childID
		}
	}

	// Seed baseline connections:
	// Factory -> Wholesale, Retail
	// Wholesale -> Factory, Retail, Pharmacy
	// Pharmacy -> Wholesale
	// Services -> Wholesale
	defaultConnections := [][2]string{
		{"Factory", "Wholesale - Wholesale"},
		{"Factory", "Retail"},
		{"Wholesale - Wholesale", "Factory"},
		{"Wholesale - Wholesale", "Retail"},
		{"Wholesale - Wholesale", "Pharmacy"},
		{"Pharmacy", "Wholesale - Wholesale"},
		{"Services", "Wholesale - Wholesale"},
		{"Sector", "Factory"},
		{"Sector", "Wholesale - Wholesale"},
	}

	for _, pair := range defaultConnections {
		fromID, ok1 := idMap[pair[0]]
		toID, ok2 := idMap[pair[1]]
		if ok1 && ok2 && fromID > 0 && toID > 0 {
			_, _ = tx.Exec(ctx, `
				INSERT INTO org.institutional_work_connections (from_institutional_work_id, to_institutional_work_id)
				VALUES ($1, $2) ON CONFLICT (from_institutional_work_id, to_institutional_work_id) DO NOTHING;
			`, fromID, toID)
		}
	}

	return nil
}

// CreateInstitutionalWork creates a new institutional structure category and syncs allowed connections.
func (r *Repository) CreateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		var parentID *int64
		if iw.ParentID != nil && *iw.ParentID > 0 {
			parentID = iw.ParentID
		}
		const query = `
			INSERT INTO org.institutional_works (title, description, icon, pricing_type, is_active, view_type, slug, parent_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query,
			iw.Title, iw.Description, iw.Icon, string(iw.PricingType), iw.IsActive, iw.ViewType, iw.Slug, parentID,
		).Scan(&iw.ID, &iw.PublicID, &iw.CreatedAt, &iw.UpdatedAt); err != nil {
			return err
		}

		// Sync connections
		if len(iw.AllowedConnections) > 0 {
			for _, toID := range iw.AllowedConnections {
				if toID > 0 && toID != iw.ID {
					if _, err := tx.Exec(txCtx, `
						INSERT INTO org.institutional_work_connections (from_institutional_work_id, to_institutional_work_id)
						VALUES ($1, $2) ON CONFLICT (from_institutional_work_id, to_institutional_work_id) DO NOTHING;
					`, iw.ID, toID); err != nil {
						return err
					}
				}
			}
		}

		return nil
	})
}

// GetInstitutionalWorkByID retrieves an institutional work by its primary key along with connected entities.
func (r *Repository) GetInstitutionalWorkByID(ctx context.Context, id int64) (*org.InstitutionalWork, error) {
	var iw org.InstitutionalWork
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		const query = `
			SELECT iw.id, iw.public_id, iw.title, iw.description, iw.icon, iw.pricing_type,
			       iw.is_active, iw.view_type, iw.slug, iw.parent_id,
			       COALESCE(p.title->>'ar', p.title->>'en', '') AS parent_title,
			       COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id = iw.id), 0) AS branch_count,
			       iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			LEFT JOIN org.institutional_works p ON iw.parent_id = p.id
			WHERE iw.id = $1 AND iw.deleted_at IS NULL;
		`
		var pricingStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&iw.ID, &iw.PublicID, &iw.Title, &iw.Description, &iw.Icon, &pricingStr,
			&iw.IsActive, &iw.ViewType, &iw.Slug, &iw.ParentID, &iw.ParentTitle,
			&iw.BranchCount, &iw.CreatedAt, &iw.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("institutional_work")
			}
			return err
		}
		iw.PricingType = org.PricingType(pricingStr)

		// Load connections
		connRows, err := tx.Query(txCtx, `
			SELECT iwc.to_institutional_work_id,
			       COALESCE(target.title->>'ar', target.title->>'en', '')
			FROM org.institutional_work_connections iwc
			JOIN org.institutional_works target ON iwc.to_institutional_work_id = target.id
			WHERE iwc.from_institutional_work_id = $1 AND target.deleted_at IS NULL
			ORDER BY target.id ASC;
		`, id)
		if err == nil {
			defer connRows.Close()
			for connRows.Next() {
				var toID int64
				var toName string
				if err := connRows.Scan(&toID, &toName); err == nil {
					iw.AllowedConnections = append(iw.AllowedConnections, toID)
					if toName != "" {
						iw.AllowedConnectionNames = append(iw.AllowedConnectionNames, toName)
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &iw, nil
}

// UpdateInstitutionalWork updates an existing institutional category and synchronizes connections.
func (r *Repository) UpdateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		var parentID *int64
		if iw.ParentID != nil && *iw.ParentID > 0 && *iw.ParentID != iw.ID {
			parentID = iw.ParentID
		}
		const query = `
			UPDATE org.institutional_works
			SET title = $2, description = $3, icon = $4, pricing_type = $5,
			    is_active = $6, view_type = $7, slug = COALESCE(NULLIF($8, ''), slug), parent_id = $9, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		tag, err := tx.Exec(txCtx, query,
			iw.ID, iw.Title, iw.Description, iw.Icon, string(iw.PricingType), iw.IsActive, iw.ViewType, iw.Slug, parentID,
		)
		if err != nil {
			return fmt.Errorf("update institutional work: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("institutional_work")
		}

		// Synchronize connections: delete old and insert new
		if _, err := tx.Exec(txCtx, `DELETE FROM org.institutional_work_connections WHERE from_institutional_work_id = $1;`, iw.ID); err != nil {
			return err
		}
		for _, toID := range iw.AllowedConnections {
			if toID > 0 && toID != iw.ID {
				if _, err := tx.Exec(txCtx, `
					INSERT INTO org.institutional_work_connections (from_institutional_work_id, to_institutional_work_id)
					VALUES ($1, $2) ON CONFLICT (from_institutional_work_id, to_institutional_work_id) DO NOTHING;
				`, iw.ID, toID); err != nil {
					return err
				}
			}
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

// CanConnectInstitutionalWorks checks if entity fromID is permitted to connect to entity toID.
func (r *Repository) CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error) {
	var allowed bool
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := ensureInstitutionalTables(txCtx, tx); err != nil {
			return err
		}
		const query = `
			SELECT EXISTS (
				SELECT 1 FROM org.institutional_work_connections
				WHERE from_institutional_work_id = $1 AND to_institutional_work_id = $2
			);
		`
		return tx.QueryRow(txCtx, query, fromID, toID).Scan(&allowed)
	})
	return allowed, err
}

// ListAllFlatInstitutionalWorks returns all institutional categories in a flat list with hierarchy levels and paths.
func (r *Repository) ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
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
			       COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id = iw.id), 0) AS branch_count,
			       iw.created_at, iw.updated_at
			FROM org.institutional_works iw
			LEFT JOIN org.institutional_works p ON iw.parent_id = p.id
			WHERE iw.deleted_at IS NULL
		`
		if onlyActive {
			query += " AND iw.is_active = true"
		}
		query += " ORDER BY iw.id ASC;"

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

		// Load connections for each
		connRows, err := tx.Query(txCtx, `
			SELECT iwc.from_institutional_work_id, iwc.to_institutional_work_id,
			       COALESCE(target.title->>'ar', target.title->>'en', '')
			FROM org.institutional_work_connections iwc
			JOIN org.institutional_works target ON iwc.to_institutional_work_id = target.id
			WHERE target.deleted_at IS NULL
			ORDER BY iwc.from_institutional_work_id, target.id ASC;
		`)
		if err == nil {
			defer connRows.Close()
			connMap := make(map[int64][]int64)
			connNamesMap := make(map[int64][]string)
			for connRows.Next() {
				var fromID, toID int64
				var name string
				if err := connRows.Scan(&fromID, &toID, &name); err == nil {
					connMap[fromID] = append(connMap[fromID], toID)
					if name != "" {
						connNamesMap[fromID] = append(connNamesMap[fromID], name)
					}
				}
			}
			for _, item := range all {
				item.AllowedConnections = connMap[item.ID]
				item.AllowedConnectionNames = connNamesMap[item.ID]
			}
		}

		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	return buildFlatHierarchy(all), nil
}

// ListInstitutionalWorks returns the full multi-level tree hierarchy of institutional categories.
func (r *Repository) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	flat, err := r.ListAllFlatInstitutionalWorks(ctx, onlyActive)
	if err != nil {
		return nil, err
	}

	// Build tree from flat items
	itemMap := make(map[int64]*org.InstitutionalWork)
	for _, item := range flat {
		item.Children = nil // clear for tree building
		itemMap[item.ID] = item
	}

	var rootList []*org.InstitutionalWork
	for _, item := range flat {
		if item.ParentID == nil || *item.ParentID <= 0 {
			rootList = append(rootList, item)
		} else if parent, exists := itemMap[*item.ParentID]; exists {
			parent.Children = append(parent.Children, item)
		} else {
			rootList = append(rootList, item)
		}
	}

	return rootList, nil
}

// buildFlatHierarchy computes recursive depth levels and ordered flat traversal.
func buildFlatHierarchy(items []*org.InstitutionalWork) []*org.InstitutionalWork {
	itemMap := make(map[int64]*org.InstitutionalWork)
	childrenMap := make(map[int64][]*org.InstitutionalWork)
	var roots []*org.InstitutionalWork

	for _, item := range items {
		itemMap[item.ID] = item
		if item.ParentID == nil || *item.ParentID <= 0 {
			roots = append(roots, item)
		} else {
			childrenMap[*item.ParentID] = append(childrenMap[*item.ParentID], item)
		}
	}

	var flat []*org.InstitutionalWork
	var traverse func(node *org.InstitutionalWork, level int)
	traverse = func(node *org.InstitutionalWork, level int) {
		node.Level = level
		flat = append(flat, node)
		for _, child := range childrenMap[node.ID] {
			traverse(child, level+1)
		}
	}

	for _, root := range roots {
		traverse(root, 0)
	}

	// Add any orphaned nodes that were not reached
	visited := make(map[int64]bool)
	for _, f := range flat {
		visited[f.ID] = true
	}
	for _, item := range items {
		if !visited[item.ID] {
			item.Level = 0
			flat = append(flat, item)
		}
	}

	return flat
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
