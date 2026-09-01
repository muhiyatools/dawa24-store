package layouts

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

func TestVisibleNav_UnapprovedUser_ShowsOnlyDocuments(t *testing.T) {
	actor := authctx.Actor{
		UserID:         42,
		OrganizationID: 10,
		Role:           "customer_owner",
		Scope:          rbac.ScopePharmacy,
		OrgStatus:      "pending",
		Permissions: []string{
			"pharmacy.dashboard.view",
			"pharmacy.purchase_request.view",
			"pharmacy.order.view",
			"pharmacy.invoice.view",
			"pharmacy.documents.view",
		},
	}
	ctx := authctx.WithActor(context.Background(), actor)

	sections := visibleNav(ctx, rbac.ScopePharmacy)
	if len(sections) == 0 {
		t.Fatalf("expected unapproved user to see documents section, got none")
	}

	for _, sec := range sections {
		for _, item := range sec.Items {
			if item.Key != "documents" && item.Href != "/customer/documents" && item.Href != "/documents" {
				t.Errorf("unapproved user saw non-document item: key=%s href=%s", item.Key, item.Href)
			}
		}
	}
}

func TestVisibleNav_ApprovedUser_ShowsFullNavigation(t *testing.T) {
	actor := authctx.Actor{
		UserID:         42,
		OrganizationID: 10,
		Role:           "customer_owner",
		Scope:          rbac.ScopePharmacy,
		OrgStatus:      "approved",
		Permissions: []string{
			"pharmacy.dashboard.view",
			"pharmacy.purchase_request.view",
			"pharmacy.order.view",
		},
	}
	ctx := authctx.WithActor(context.Background(), actor)

	sections := visibleNav(ctx, rbac.ScopePharmacy)
	if len(sections) == 0 {
		t.Fatalf("expected approved user to have visible nav sections, got none")
	}

	foundDashboard := false
	for _, sec := range sections {
		for _, item := range sec.Items {
			if item.Key == "dashboard" {
				foundDashboard = true
			}
		}
	}
	if !foundDashboard {
		t.Errorf("approved user should see dashboard nav item")
	}
}
