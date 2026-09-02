package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListCoverageForOrganizationWithTotal retrieves paginated weekly coverage records for an organization with total count.
func (r *Repository) ListCoverageForOrganizationWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*workflow.CoverageView, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	var list []*workflow.CoverageView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countQuery = `SELECT count(*) FROM workflow.weekly_coverages wc WHERE ($1 = 0 OR wc.organization_id = $1);`
		if err := tx.QueryRow(txCtx, countQuery, orgID).Scan(&total); err != nil {
			return err
		}

		query := `
			SELECT wc.id, wc.public_id, wc.organization_id, wc.branch_id, wc.governorate_id, wc.city_id,
			       wc.day_of_week,
			       to_char(wc.coverage_from, 'HH24:MI') AS coverage_from,
			       to_char(wc.coverage_to,   'HH24:MI') AS coverage_to,
			       wc.address,
			       COALESCE(wc.latitude, c.latitude) AS latitude,
			       COALESCE(wc.longitude, c.longitude) AS longitude,
			       wc.distance_meters, wc.is_active,
			       wc.created_at, wc.updated_at,
			       COALESCE(b.name->>'ar', b.name->>'en', b.name::text, '') AS branch_name,
			       COALESCE(g.name->>'ar', g.name->>'en', '') AS gov_name_ar,
			       COALESCE(g.name->>'en', g.name->>'ar', '') AS gov_name_en,
			       COALESCE(c.name->>'ar', c.name->>'en', '') AS city_name_ar,
			       COALESCE(c.name->>'en', c.name->>'ar', '') AS city_name_en,
			       COALESCE(o.legal_name, o.name->>'ar', o.name->>'en', '') AS org_name
			FROM workflow.weekly_coverages wc
			LEFT JOIN org.organizations o ON o.id = wc.organization_id
			LEFT JOIN org.branches b ON b.id = wc.branch_id AND b.deleted_at IS NULL
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			LEFT JOIN platform_admin.governorates g ON g.id = COALESCE(wc.governorate_id, c.governorate_id)
			WHERE ($1 = 0 OR wc.organization_id = $1)
			ORDER BY wc.day_of_week ASC, wc.id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v workflow.CoverageView
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.BranchID, &v.GovernorateID, &v.CityID,
				&v.DayOfWeek, &v.CoverageFrom, &v.CoverageTo, &v.Address,
				&v.Latitude, &v.Longitude, &v.DistanceMeters, &v.IsActive,
				&v.CreatedAt, &v.UpdatedAt,
				&v.BranchName, &v.GovernorateNameAr, &v.GovernorateName, &v.CityNameAr, &v.CityNameEn, &v.OrgName,
			); err != nil {
				return err
			}
			v.CityName = v.CityNameAr
			if v.CityName == "" {
				v.CityName = v.CityNameEn
			}
			list = append(list, &v)
		}
		return rows.Err()
	})
	return list, total, err
}
