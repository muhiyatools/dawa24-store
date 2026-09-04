package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Coord represents geographic latitude and longitude, with optional CityID.
type Coord struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	CityID *int64  `json:"city_id,omitempty"`
}

// CoverageService evaluates spatial reachability between suppliers and pharmacies.
type CoverageService struct {
	db *database.DB
}

// NewCoverageService constructs a new CoverageService.
func NewCoverageService(db *database.DB) *CoverageService {
	return &CoverageService{db: db}
}

// ServesPoint checks whether an organization covers the given location coordinates or city on a specified weekday and optional time.
// Returns (serves bool, distanceMeters int, err error).
func (cs *CoverageService) ServesPoint(ctx context.Context, orgID int64, day time.Weekday, target Coord, optWhen ...time.Time) (bool, int, error) {
	dayInt := int(day) // 0 = Sunday, 1 = Monday ...
	var distanceMeters int
	var actualMeters *int

	var targetCityID int64
	if target.CityID != nil && *target.CityID > 0 {
		targetCityID = *target.CityID
	}
	hasCoords := target.Lat != 0 || target.Lon != 0

	var timeParam *string
	if len(optWhen) > 0 && !optWhen[0].IsZero() {
		tStr := optWhen[0].Format("15:04:05")
		timeParam = &tStr
	}

	err := cs.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Strict check: current day of week (or explicit daily coverage) and valid operating time window
		query := `
			SELECT GREATEST(COALESCE(c.coverage_radius_meters, 0), COALESCE(wc.distance_meters, 0), 3000) AS allowed_radius,
			       CASE 
			           WHEN COALESCE(wc.latitude, c.latitude, b.latitude) IS NOT NULL AND $5::boolean = true THEN
			               platform.distance_meters(
			                   COALESCE(wc.latitude, c.latitude, b.latitude)::numeric,
			                   COALESCE(wc.longitude, c.longitude, b.longitude)::numeric,
			                   $2::numeric,
			                   $3::numeric
			               )::integer
			           ELSE 0
			       END AS actual_meters
			FROM workflow.weekly_coverages wc
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			LEFT JOIN org.branches b ON b.id = wc.branch_id
			WHERE wc.organization_id = $1::bigint
			  AND (wc.day_of_week = $4::integer OR wc.day_of_week IS NULL)
			  AND wc.is_active = true
			  AND (
			      $7::text IS NULL
			      OR (wc.coverage_from IS NULL AND wc.coverage_to IS NULL)
			      OR (
			          wc.coverage_from <= wc.coverage_to
			          AND $7::time >= wc.coverage_from
			          AND $7::time <= wc.coverage_to
			      )
			      OR (
			          wc.coverage_from > wc.coverage_to
			          AND ($7::time >= wc.coverage_from OR $7::time <= wc.coverage_to)
			      )
			  )
			  AND (
			      (
			          $5::boolean = true
			          AND COALESCE(wc.latitude, c.latitude, b.latitude) IS NOT NULL
			          AND COALESCE(wc.longitude, c.longitude, b.longitude) IS NOT NULL
			          AND platform.distance_meters(
			              COALESCE(wc.latitude, c.latitude, b.latitude)::numeric,
			              COALESCE(wc.longitude, c.longitude, b.longitude)::numeric,
			              $2::numeric,
			              $3::numeric
			          )::integer <= GREATEST(COALESCE(c.coverage_radius_meters, 0), COALESCE(wc.distance_meters, 0), 3000)
			      )
			      OR (
			          ($5::boolean = false OR (COALESCE(wc.latitude, c.latitude, b.latitude) IS NULL AND COALESCE(wc.longitude, c.longitude, b.longitude) IS NULL))
			          AND $6::bigint > 0
			          AND wc.city_id = $6::bigint
			      )
			  )
			ORDER BY (wc.day_of_week = $4::integer) DESC, actual_meters ASC
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, orgID, target.Lat, target.Lon, dayInt, hasCoords, targetCityID, timeParam).Scan(&distanceMeters, &actualMeters)
		if err == nil {
			return nil
		}
		if !database.IsNotFound(err) {
			return err
		}

		// If vendor has no active coverage for today's weekday or operating hours, fail closed.
		return pgx.ErrNoRows
	})

	if err != nil {
		if database.IsNotFound(err) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("coverage.ServesPoint: %w", err)
	}

	retMeters := 0
	if actualMeters != nil {
		retMeters = *actualMeters
	}
	return true, retMeters, nil
}
