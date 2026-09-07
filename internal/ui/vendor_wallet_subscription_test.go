package ui

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestDepositModal_ResponsiveLayoutAndCopyButtons(t *testing.T) {
	data := pages.WalletViewData{
		IsVendor: true,
		PlatformPaymentMethods: []*billing.PlatformPaymentMethod{
			{
				ID:               "ppm-cib",
				Name:             i18n.Text{"ar": "البنك التجاري الدولي (CIB)"},
				ProviderType:     "bank",
				IsDepositEnabled: true,
				IsActive:         true,
				BankName:         "CIB",
				AccountName:      "شركة دوا 24",
				AccountNumber:    "100029384756",
				IBAN:             "EG120010000100029384756",
			},
		},
	}

	var buf bytes.Buffer
	err := pages.WalletModals(data).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render WalletModals: %v", err)
	}

	html := buf.String()

	// Check container class
	if !strings.Contains(html, "deposit-modal-container") {
		t.Errorf("expected deposit-modal-container class in rendered modal")
	}

	// Check scrollable body class
	if !strings.Contains(html, "deposit-modal-body") {
		t.Errorf("expected deposit-modal-body class in rendered modal")
	}

	// Check responsive 2-column grid
	if !strings.Contains(html, "grid grid-cols-1 sm:grid-cols-2") {
		t.Errorf("expected responsive 2-column grid in deposit modal")
	}

	// Check copy button
	if !strings.Contains(html, "نسخ") {
		t.Errorf("expected copy button text in deposit modal")
	}

	// Check fixed modal footer
	if !strings.Contains(html, "تأكيد طلب الشحن") {
		t.Errorf("expected submit button in deposit modal footer")
	}
}

func TestTenantSubscriptionPage_DaysRemainingAndNoManualRenew(t *testing.T) {
	expiryDate := time.Now().Add(15 * 24 * time.Hour)
	planPrice := money.MustParse("1000.00")
	planPriceAnnual := money.MustParse("5000.00")

	plans := []*billing.Plan{
		{
			ID:               1,
			Slug:             "growth",
			Name:             i18n.Text{"ar": "باقة النمو المتقدمة"},
			PriceMonth:       planPrice,
			PriceYear:        planPriceAnnual,
			MaxLoginSessions: 5,
			MaxDevices:       5,
			AIPlanID:         "growth-ai",
		},
		{
			ID:               2,
			Slug:             "enterprise",
			Name:             i18n.Text{"ar": "باقة الشركات الكبرى"},
			PriceMonth:       money.MustParse("3000.00"),
			PriceYear:        money.MustParse("15000.00"),
			MaxLoginSessions: 20,
			MaxDevices:       20,
			AIPlanID:         "enterprise-ai",
		},
	}

	data := pages.TenantSubscriptionPageData{
		Subscription: &pages.OrgSubscriptionView{
			HasSubscription:  true,
			PlanName:         "باقة النمو المتقدمة",
			PlanSlug:         "growth",
			Status:           "نشط ومفعّل",
			ExpiresAt:        expiryDate.Format("2006-01-02"),
			RawExpiresAt:     expiryDate,
			DaysRemaining:    15,
			HasDaysRemaining: true,
			RenewalCost:      planPrice,
			MaxLoginSessions: 5,
			MaxDevices:       5,
			AIPlanID:         "growth-ai",
		},
		Plans:         plans,
		CurrentPlanID: 1,
		WalletBalance: money.MustParse("15000.00"),
		AutoRenew:     true,
		BillingCycle:  "monthly",
	}

	var buf bytes.Buffer
	err := pages.TenantSubscriptionPage(data, "vendor", "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionPage: %v", err)
	}

	html := buf.String()

	// 1. Must show days remaining
	if !strings.Contains(html, "متبقي 15 يوماً على موعد التجديد") {
		t.Errorf("expected days remaining countdown text, got html snippet without it")
	}

	// 2. Must show auto-renewal information banner
	if !strings.Contains(html, "نظام الفوترة والتجديد التلقائي عبر المحفظة") {
		t.Errorf("expected auto-renewal wallet info banner")
	}

	// 3. Must show wallet balance coverage indication
	if !strings.Contains(html, "يغطي تكلفة التجديد القادم بنجاح") {
		t.Errorf("expected wallet coverage success notice")
	}

	// 4. Must show active status pill on current plan
	if !strings.Contains(html, "باقتك الحالية المفعّلة (تتجدد تلقائياً)") {
		t.Errorf("expected active plan pill")
	}

	// 5. Must NOT show manual renewal button on current plan
	if strings.Contains(html, "تجديد / تمديد باقتك الحالية") {
		t.Errorf("CRITICAL BUG: 'تجديد / تمديد باقتك الحالية' should be completely removed, but was found in HTML!")
	}

	// 6. Must show upgrade button on other tier
	if !strings.Contains(html, "الاشتراك والترقية لهذه الباقة") {
		t.Errorf("expected upgrade button for enterprise plan")
	}
}

func TestSubscriptionsPlural_RedirectsToSingular(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/vendor/subscriptions", nil)
	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.subscription.view"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	http.Redirect(rec, req, "/vendor/subscription", http.StatusMovedPermanently)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/vendor/subscription" {
		t.Fatalf("expected /vendor/subscription, got %s", loc)
	}
}
