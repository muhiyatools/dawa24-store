package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	promoPostgres "github.com/muhiya/dawa24-store/internal/modules/promo/postgres"
	workflowPostgres "github.com/muhiya/dawa24-store/internal/modules/workflow/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// TestCoverageChain_VisibilityRule verifies that vendor weekly coverage coordinates
// and active status directly govern offer visibility for pharmacies (Plan V5 Phase 0 Task 0.1 & T9).
func TestCoverageChain_VisibilityRule(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	promoRepo := promoPostgres.NewRepository(db)
	wfRepo := workflowPostgres.NewRepository(db)

	// Clean up any test fixtures after run
	var orgID, branchID, offerID, covID int64
	defer func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM promo.offers WHERE id = $1", offerID)
			_, _ = tx.Exec(txCtx, "DELETE FROM workflow.weekly_coverages WHERE id = $1", covID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.branches WHERE id = $1", branchID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.organizations WHERE id = $1", orgID)
			return nil
		})
	}()

	// 1. Setup: Create Vendor Org & Branch at Cairo (30.0444, 31.2357)
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx, `
			INSERT INTO org.organizations (
				public_id, legal_name, trade_name, tax_number, commercial_register,
				type, status, created_at, updated_at
			) VALUES (
				'org_test_cov_' || substr(md5(random()::text), 1, 8),
				'Coverage Test Vendor Co', '{"ar":"مورد اختبار التغطية","en":"Coverage Test Vendor"}',
				'TX-99887766', 'CR-99887766', 'vendor', 'approved', now(), now()
			) RETURNING id;
		`).Scan(&orgID)
		if err != nil {
			return err
		}

		lat, lng := 30.0444, 31.2357
		err = tx.QueryRow(txCtx, `
			INSERT INTO org.branches (
				public_id, organization_id, name, code, address,
				latitude, longitude, created_at, updated_at
			) VALUES (
				'br_test_cov_' || substr(md5(random()::text), 1, 8),
				$1, '{"ar":"فرع القاهرة المركزي","en":"Cairo Central Branch"}',
				'CAI-01', '123 Tahrir St, Cairo', $2, $3, now(), now()
			) RETURNING id;
		`, orgID, lat, lng).Scan(&branchID)
		return err
	})
	if err != nil {
		t.Fatalf("failed creating vendor org & branch: %v", err)
	}

	// 2. Vendor creates weekly coverage for Sunday (day 0), radius 25km (25,000m)
	covLat, covLng := 30.0444, 31.2357
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO workflow.weekly_coverages (
				public_id, organization_id, branch_id, day_of_week, coverage_from, coverage_to,
				address, latitude, longitude, distance_meters, is_active, created_at, updated_at
			) VALUES (
				'cov_test_' || substr(md5(random()::text), 1, 8),
				$1, $2, 0, '08:00', '18:00',
				'Cairo Metropolitan Area', $3, $4, 25000, true, now(), now()
			) RETURNING id;
		`, orgID, branchID, covLat, covLng).Scan(&covID)
	})
	if err != nil {
		t.Fatalf("failed creating weekly coverage: %v", err)
	}

	// 3. Vendor creates approved active offer
	startsAt := time.Now().Add(-1 * time.Hour)
	expiresAt := time.Now().Add(24 * time.Hour)
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO promo.offers (
				public_id, organization_id, branch_id, title, description,
				discount_type, discount_value, min_order_amount, admin_status,
				starts_at, expires_at, is_active, created_at, updated_at
			) VALUES (
				'off_test_' || substr(md5(random()::text), 1, 8),
				$1, $2, 'Special Sunday Discount', '10% off for in-range pharmacies',
				'percentage', 10.0, 50000, 'approved',
				$3, $4, true, now(), now()
			) RETURNING id;
		`, orgID, branchID, startsAt, expiresAt).Scan(&offerID)
	})
	if err != nil {
		t.Fatalf("failed creating offer: %v", err)
	}

	// 4. Pharmacy A in Cairo (30.0500, 31.2400) (~1km away) queries visible offers on Sunday (day 0)
	cairoPharmacyLat, cairoPharmacyLng := 30.0500, 31.2400
	visibleCairo, err := promoRepo.ListOffersVisibleTo(ctx, cairoPharmacyLat, cairoPharmacyLng, 0, 10, 0, nil)
	if err != nil {
		t.Fatalf("ListOffersVisibleTo for Cairo pharmacy failed: %v", err)
	}
	found := false
	for _, vo := range visibleCairo {
		if vo.Offer.ID == offerID {
			found = true
			if vo.Metres > 25000 {
				t.Errorf("expected distance <= 25000, got %d", vo.Metres)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected offer %d to be visible to Cairo pharmacy (~1km away), but was not found", offerID)
	}

	// 5. Pharmacy B in Alexandria (31.2001, 29.9187) (~180km away) queries on Sunday (day 0) -> finds 0 offers
	alexPharmacyLat, alexPharmacyLng := 31.2001, 29.9187
	visibleAlex, err := promoRepo.ListOffersVisibleTo(ctx, alexPharmacyLat, alexPharmacyLng, 0, 10, 0, nil)
	if err != nil {
		t.Fatalf("ListOffersVisibleTo for Alexandria pharmacy failed: %v", err)
	}
	for _, vo := range visibleAlex {
		if vo.Offer.ID == offerID {
			t.Errorf("offer %d should NOT be visible to Alexandria pharmacy (~180km away, radius is 25km)", offerID)
		}
	}

	// 6. Vendor toggles coverage to inactive
	err = wfRepo.ToggleWeeklyCoverage(ctx, covID, false)
	if err != nil {
		t.Fatalf("ToggleWeeklyCoverage failed: %v", err)
	}

	// Cairo pharmacy queries again -> finds 0 offers because coverage is inactive
	visibleCairoAfterDisable, err := promoRepo.ListOffersVisibleTo(ctx, cairoPharmacyLat, cairoPharmacyLng, 0, 10, 0, nil)
	if err != nil {
		t.Fatalf("ListOffersVisibleTo after disabling coverage failed: %v", err)
	}
	for _, vo := range visibleCairoAfterDisable {
		if vo.Offer.ID == offerID {
			t.Errorf("offer %d should NOT be visible after coverage is disabled", offerID)
		}
	}
}
