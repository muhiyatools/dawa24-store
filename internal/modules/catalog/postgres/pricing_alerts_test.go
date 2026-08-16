package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogPostgres "github.com/muhiya/dawa24-store/internal/modules/catalog/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestCustomerPricingAndAlerts(t *testing.T) {
	db := getTestDB(t)
	repo := catalogPostgres.NewRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orgID := int64(1001)
	customerOrgID := int64(1002)
	userID := int64(2001)
	ctx = database.WithTenant(ctx, orgID)

	prod := &catalog.Product{
		OrganizationID: orgID,
		Name:           i18n.New("بانادول أزرق تسعير", "Panadol Blue Pricing"),
		Status:         catalog.StatusActive,
	}
	if err := repo.CreateProduct(ctx, prod); err != nil {
		t.Fatalf("CreateProduct failed: %v", err)
	}
	defer func() { _ = repo.DeleteProduct(ctx, prod.ID) }()

	t.Run("CustomerPricing", func(t *testing.T) {
		m := &catalog.CustomerProductMapping{
			OrganizationID: orgID,
			CustomerOrgID:  customerOrgID,
			ProductID:      prod.ID,
			CustomPrice:    money.FromMinor(800),
			IsActive:       true,
		}
		if err := repo.SetCustomerPricing(ctx, m); err != nil {
			t.Fatalf("SetCustomerPricing failed: %v", err)
		}

		got, err := repo.GetCustomerPricing(ctx, orgID, customerOrgID, prod.ID)
		if err != nil {
			t.Fatalf("GetCustomerPricing failed: %v", err)
		}
		if got.CustomPrice.Minor() != 800 {
			t.Errorf("money round-trip failed: got %v", got.CustomPrice)
		}
	})

	t.Run("ProductAlerts", func(t *testing.T) {
		a := &catalog.ProductAlert{
			UserID:      userID,
			ProductID:   prod.ID,
			AlertType:   "price_drop",
			TargetPrice: money.FromMinor(900),
		}
		if err := repo.CreateProductAlert(ctx, a); err != nil {
			t.Fatalf("CreateProductAlert failed: %v", err)
		}

		alerts, err := repo.ListProductAlertsByUser(ctx, userID)
		if err != nil {
			t.Fatalf("ListProductAlertsByUser failed: %v", err)
		}
		if len(alerts) == 0 {
			t.Error("expected at least 1 alert")
		}
	})
}
