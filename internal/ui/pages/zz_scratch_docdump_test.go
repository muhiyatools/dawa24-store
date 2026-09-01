package pages

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

func renderDump(t *testing.T, name string, comp interface{ Render(context.Context, *strings.Builder) error }) {
}

func dumpStats(t *testing.T, label, html string) {
	t.Logf("[%s] len=%d html=%d app-shell=%d <aside=%d main-content=%d sidebar-link=%d id=capsule=%d class=capsule-host=%d catalog-sidebar-filter=%d dashboard-topbar=%d site-header=%d",
		label, len(html),
		strings.Count(html, "<html"),
		strings.Count(html, `class="app-shell"`),
		strings.Count(html, "<aside"),
		strings.Count(html, "main-content"),
		strings.Count(html, "sidebar-link"),
		strings.Count(html, `id="capsule-assistant-host"`),
		strings.Count(html, `class="capsule-assistant-host"`),
		strings.Count(html, "catalog-sidebar-filter"),
		strings.Count(html, `class="dashboard-topbar`),
		strings.Count(html, `class="site-header`),
	)
}

func pendingActor() authctx.Actor {
	return authctx.Actor{
		UserID: 42, OrganizationID: 10, OrgType: "customer", OrgStatus: "under_review",
		Role: "customer_owner", Scope: rbac.ScopePharmacy,
		Permissions: []string{
			"pharmacy.dashboard.view", "pharmacy.document.view", "pharmacy.purchase_request.view",
			"pharmacy.order.view", "pharmacy.branch.view", "pharmacy.team.view",
		},
	}
}

func TestScratchDumpPages(t *testing.T) {
	act := pendingActor()
	ctx := authctx.WithActor(context.Background(), act)

	// 1. Organization documents
	{
		var sb strings.Builder
		if err := OrganizationDocuments("ar", "rtl", &OrganizationDocumentsData{}, act.Permissions).Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		dumpStats(t, "orgdocs", sb.String())
		_ = os.WriteFile("../../../scratch/dump_orgdocs.html", []byte(sb.String()), 0644)
	}

	// 2. Catalog full page
	{
		var sb strings.Builder
		if err := CustomerCatalog(CatalogPageData{}, "ar", "rtl", false).Render(ctx, &sb); err != nil {
			t.Fatal(err)
		}
		dumpStats(t, "catalog", sb.String())
		_ = os.WriteFile("../../../scratch/dump_catalog.html", []byte(sb.String()), 0644)
	}
}
