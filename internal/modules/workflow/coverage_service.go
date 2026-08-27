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
			  AND wc.day_of_week = $4::integer
			  AND wc.is_active = true
			  AND COALESCE(wc.latitude, c.latitude, b.latitude) IS NOT NULL
			  AND COALESCE(wc.longitude, c.longitude, b.longitude) IS NOT NULL
			ORDER BY actual_meters ASC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, orgID, target.Lat, target.Lon, dayInt).Scan(&distanceMeters, &actualMeters)
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
