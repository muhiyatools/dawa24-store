package postgres

import (
	"context"

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
			ON CONFLICT (visitor_key, visited_at) DO NOTHING;
		`
		_, err := tx.Exec(txCtx, query, v.VisitorKey, v.IP, v.UserAgent, v.Browser, v.Device, v.OS, v.Country, v.City)
		return err
	})
}

// VisitorAnalytics returns the aggregate traffic view.
func (r *Repository) VisitorAnalytics(ctx context.Context, limit int) (*platformadmin.VisitorAnalytics, error) {
	out := &platformadmin.VisitorAnalytics{
		ByDevice:  map[string]int{},
		ByOS:      map[string]int{},
		ByBrowser: map[string]int{},
		Recent:    []*platformadmin.Visitor{},
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.visitors;`).Scan(&out.Total); err != nil {
			return err
		}
		if err := tx.QueryRow(txCtx, `SELECT COUNT(*) FROM platform_admin.visitors WHERE visited_at = CURRENT_DATE;`).Scan(&out.Today); err != nil {
			return err
		}

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
					key = "غير محدد"
				}
				m[key] = n
			}
			return m, rows.Err()
		}

		var err error
		if out.ByDevice, err = scanGroup(`SELECT device, COUNT(*) FROM platform_admin.visitors GROUP BY device ORDER BY COUNT(*) DESC;`); err != nil {
			return err
		}
		if out.ByOS, err = scanGroup(`SELECT os, COUNT(*) FROM platform_admin.visitors GROUP BY os ORDER BY COUNT(*) DESC;`); err != nil {
			return err
		}
		if out.ByBrowser, err = scanGroup(`SELECT browser, COUNT(*) FROM platform_admin.visitors GROUP BY browser ORDER BY COUNT(*) DESC;`); err != nil {
			return err
		}

		rows, err := tx.Query(txCtx, `
			SELECT id, visitor_key, ip, user_agent, browser, device, os, country, city, visited_at, created_at
			FROM platform_admin.visitors ORDER BY created_at DESC LIMIT $1;`,
			limit)
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
