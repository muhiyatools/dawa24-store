package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Coord represents geographic latitude and longitude.
type Coord struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// CoverageService evaluates spatial reachability between suppliers and pharmacies.
type CoverageService struct {
	db *database.DB
}

// NewCoverageService constructs a new CoverageService.
func NewCoverageService(db *database.DB) *CoverageService {
	return &CoverageService{db: db}
}

// ServesPoint checks whether an organization covers the given location coordinates on a specified weekday.
// Returns (serves bool, distanceMeters int, err error).
func (cs *CoverageService) ServesPoint(ctx context.Context, orgID int64, day time.Weekday, target Coord) (bool, int, error) {
	dayInt := int(day) // 0 = Sunday, 1 = Monday ...
	var distanceMeters int
	var actualMeters *int

	err := cs.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// First try: exact day of week
		query := `
			SELECT wc.distance_meters,
			       platform.distance_meters(
			           COALESCE(wc.latitude, c.latitude, b.latitude)::numeric,
			           COALESCE(wc.longitude, c.longitude, b.longitude)::numeric,
			           $2::numeric,
			           $3::numeric
			       )::integer AS actual_meters
			FROM workflow.weekly_coverages wc
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			LEFT JOIN org.branches b ON b.id = wc.branch_id
			WHERE wc.organization_id = $1::bigint
			  AND (wc.day_of_week = $4::integer OR wc.day_of_week IS NULL)
			  AND wc.is_active = true
			  AND COALESCE(wc.latitude, c.latitude, b.latitude) IS NOT NULL
			  AND COALESCE(wc.longitude, c.longitude, b.longitude) IS NOT NULL
			ORDER BY (wc.day_of_week = $4::integer) DESC, actual_meters ASC
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, orgID, target.Lat, target.Lon, dayInt).Scan(&distanceMeters, &actualMeters)
		if err == nil {
			return nil
		}
		if !database.IsNotFound(err) {
			return err
		}

		// Second try: any active coverage day (advance orders for scheduled delivery)
		queryAny := `
			SELECT wc.distance_meters,
			       platform.distance_meters(
			           COALESCE(wc.latitude, c.latitude, b.latitude)::numeric,
			           COALESCE(wc.longitude, c.longitude, b.longitude)::numeric,
			           $2::numeric,
			           $3::numeric
			       )::integer AS actual_meters
			FROM workflow.weekly_coverages wc
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			LEFT JOIN org.branches b ON b.id = wc.branch_id
			WHERE wc.organization_id = $1::bigint
			  AND wc.is_active = true
			  AND COALESCE(wc.latitude, c.latitude, b.latitude) IS NOT NULL
			  AND COALESCE(wc.longitude, c.longitude, b.longitude) IS NOT NULL
			ORDER BY actual_meters ASC
			LIMIT 1;
		`
		err = tx.QueryRow(txCtx, queryAny, orgID, target.Lat, target.Lon).Scan(&distanceMeters, &actualMeters)
		if err == nil {
			return nil
		}
		if !database.IsNotFound(err) {
			return err
		}

		// Third try: if vendor has no weekly_coverages rows at all, check if vendor exists and is approved
		var vendorExists bool
		err = tx.QueryRow(txCtx, `SELECT EXISTS(SELECT 1 FROM org.organizations WHERE id = $1 AND status = 'approved')`, orgID).Scan(&vendorExists)
		if err == nil && vendorExists {
			defDist := 50000
			distanceMeters = defDist
			defAct := 1000
			actualMeters = &defAct
			return nil
		}
		return pgx.ErrNoRows
	})

	if err != nil {
		if database.IsNotFound(err) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("coverage.ServesPoint: %w", err)
	}

	allowedRadius := distanceMeters
	if allowedRadius < 35000 {
		allowedRadius = 35000
	}

	if actualMeters != nil && *actualMeters <= allowedRadius {
		return true, *actualMeters, nil
	}

	if actualMeters != nil {
		return false, *actualMeters, nil
	}

	return false, 0, nil
}
