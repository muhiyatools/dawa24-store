package postgres

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// RecordVisitor inserts one visitor-session-day, ignoring repeats.
func (r *Repository) RecordVisitor(ctx context.Context, v *platformadmin.Visitor) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO platform_admin.visitors (visitor_key, ip, user_agent, browser, device, os, country, city, visited_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_DATE)
			ON CONFLICT (visitor_key, visited_at) DO UPDATE 
			SET country = EXCLUDED.country, city = EXCLUDED.city, ip = EXCLUDED.ip;
		`
		_, err := tx.Exec(txCtx, query, v.VisitorKey, v.IP, v.UserAgent, v.Browser, v.Device, v.OS, v.Country, v.City)
		return err
	})
}

// VisitorAnalytics returns the aggregate traffic and platform health view.
func (r *Repository) VisitorAnalytics(ctx context.Context, limit int) (*platformadmin.VisitorAnalytics, error) {
	return r.VisitorAnalyticsWithTotal(ctx, limit, 0)
}

// VisitorAnalyticsWithTotal returns the aggregate traffic and platform health view with pagination.
func (r *Repository) VisitorAnalyticsWithTotal(ctx context.Context, limit, offset int) (*platformadmin.VisitorAnalytics, error) {
	out := &platformadmin.VisitorAnalytics{
		ByCountry: map[string]int{},
		ByCity:    map[string]int{},
		ByDevice:  map[string]int{},
		ByOS:      map[string]int{},
		ByBrowser: map[string]int{},
		Recent:    []*platformadmin.Visitor{},
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.visitors;`).Scan(&out.Total)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.visitors WHERE visited_at = CURRENT_DATE;`).Scan(&out.Today)

		// Platform business summary stats (strictly from live database tables)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM org.organizations WHERE type IN ('pharmacy', 'customer') AND deleted_at IS NULL;`).Scan(&out.TotalPharmacies)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM org.organizations WHERE type IN ('supplier', 'vendor') AND deleted_at IS NULL;`).Scan(&out.TotalSuppliers)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM catalog.products WHERE deleted_at IS NULL;`).Scan(&out.TotalProducts)
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.orders;`).Scan(&out.TotalOrders)

		var totalGMVCents int64
		_ = tx.QueryRow(txCtx, `SELECT COALESCE(SUM(total_amount), 0) FROM commerce.orders WHERE status NOT IN ('cancelled', 'rejected');`).Scan(&totalGMVCents)
		out.TotalGMV = fmt.Sprintf(i18n.TDefault("w4_mod.2f_425"), float64(totalGMVCents)/100.0)

		scanGroup := func(query string) (map[string]int, error) {
			m := map[string]int{}
			rows, err := tx.Query(txCtx, query)
			if err != nil {
				return m, err
			}
			defer rows.Close()
			for rows.Next() {
				var key string
				var n int
				if err := rows.Scan(&key, &n); err != nil {
					return m, err
				}
				if key == "" {
					key = i18n.TDefault("w4_ui.s_178_178")
				}
				m[key] = n
			}
			return m, rows.Err()
		}

		var err error
		if out.ByCountry, err = scanGroup(`SELECT CASE WHEN country = 'شبكة داخلية 🖥️' OR country = 'غير محدد' OR country = '' THEN 'مصر 🇪🇬' ELSE country END, COUNT(*) FROM platform_admin.visitors GROUP BY 1 ORDER BY COUNT(*) DESC;`); err != nil {
			return err
		}
		if out.ByCity, err = scanGroup(`SELECT CASE WHEN city = 'بيئة التطوير (Local)' OR city = 'غير محدد' OR city = '' THEN 'القاهرة' ELSE city END, COUNT(*) FROM platform_admin.visitors GROUP BY 1 ORDER BY COUNT(*) DESC;`); err != nil {
			return err
		}
		if out.ByDevice, err = scanGroup(`SELECT COALESCE(NULLIF(device, ''), 'كمبيوتر مكتب (Desktop)'), COUNT(*) FROM platform_admin.visitors GROUP BY 1 ORDER BY COUNT(*) DESC;`); err != nil {
			return err
		}
		if out.ByOS, err = scanGroup(`SELECT COALESCE(NULLIF(os, ''), 'Windows'), COUNT(*) FROM platform_admin.visitors GROUP BY 1 ORDER BY COUNT(*) DESC;`); err != nil {
			return err
		}
		if out.ByBrowser, err = scanGroup(`SELECT COALESCE(NULLIF(browser, ''), 'Chrome'), COUNT(*) FROM platform_admin.visitors GROUP BY 1 ORDER BY COUNT(*) DESC;`); err != nil {
			return err
		}

		rows, err := tx.Query(txCtx, `
			SELECT id, visitor_key, ip, user_agent, browser, device, os,
			       CASE WHEN country = 'شبكة داخلية 🖥️' OR country = 'غير محدد' OR country = '' THEN 'مصر 🇪🇬' ELSE country END AS country,
			       CASE WHEN city = 'بيئة التطوير (Local)' OR city = 'غير محدد' OR city = '' THEN 'القاهرة' ELSE city END AS city,
			       visited_at, created_at
			FROM platform_admin.visitors ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2;`,
			limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v platformadmin.Visitor
			if err := rows.Scan(&v.ID, &v.VisitorKey, &v.IP, &v.UserAgent, &v.Browser, &v.Device, &v.OS, &v.Country, &v.City, &v.VisitedAt, &v.CreatedAt); err != nil {
				return err
			}
			out.Recent = append(out.Recent, &v)
		}
		return rows.Err()
	})
	return out, err
}
