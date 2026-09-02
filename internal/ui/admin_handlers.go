package ui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) AdminDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	stats := pages.AdminDashboardStats{
		TopDevices:   make(map[string]int),
		TopBrowsers:  make(map[string]int),
		TopLocations: make(map[string]int),
	}
	var pendingOrgs []*org.Organization
	var recentOrganizations []*org.Organization

	if h.idSvc != nil {
		if n, err := h.idSvc.AdminCountUsers(ctx); err == nil {
			stats.TotalUsers = n
		}
	}
	if h.orgSvc != nil {
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, nil); err == nil {
			stats.TotalOrganizations = n
		}
		pharmacyType := org.TypeCustomer
		if n, err := h.orgSvc.CountOrganizations(ctx, &pharmacyType, nil); err == nil {
			stats.TotalPharmacies = n
		}
		vendorType := org.TypeVendor
		if n, err := h.orgSvc.CountOrganizations(ctx, &vendorType, nil); err == nil {
			stats.TotalVendors = n
		}
		pending := org.StatusPending
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, &pending); err == nil {
			stats.PendingApprovals = n
		}
		if list, err := h.orgSvc.ListOrganizations(ctx, nil, &pending, 6, 0); err == nil {
			pendingOrgs = list
		}
		if recentList, err := h.orgSvc.ListOrganizations(ctx, nil, nil, 6, 0); err == nil {
			recentOrganizations = recentList
		}
		// AdminBranchStats and not ListBranches(0): the latter selects every
		// branch on the platform, joined to identity.users with a correlated
		// array_agg per row, and materialises the lot in Go so that len() can
		// be taken of it. The count is a count.
		if bs, err := h.orgSvc.AdminBranchStats(sysCtx); err == nil {
			stats.TotalBranches = bs.TotalBranches
		}
	}
	if h.commSvc != nil {
		if n, err := h.commSvc.CountOrders(ctx); err == nil {
			stats.TotalOrders = n
		}
		if recent, err := h.commSvc.AdminSearchOrders(ctx, "", 8, 0); err == nil {
			stats.RecentOrders = recent
			for _, o := range recent {
				if o.Status == commerce.StatusDelivered || o.Status == commerce.StatusCompleted {
					stats.CompletedOrdersCount++
				} else if o.Status != commerce.StatusCancelled && o.Status != commerce.StatusFailed {
					stats.ActiveOrdersCount++
				}
			}
		}
	}
	if h.billSvc != nil {
		if deps, total, err := h.billSvc.AdminListDetailedDeposits(sysCtx, billing.DepositFilter{Status: "pending", Limit: 50}); err == nil {
			stats.PendingDepositsCount = total
			var depTotal money.Amount
			for _, d := range deps {
				depTotal, _ = depTotal.Add(d.Amount)
			}
			stats.PendingDepositsAmount = fmt.Sprintf("%s %s", depTotal.String(), i18n.T(lang, "common.currency_egp"))
		}
		if wallets, _, err := h.billSvc.AdminListDetailedWallets(sysCtx, billing.WalletFilter{Limit: 100}); err == nil {
			var totalHeldMinor int64
			for _, w := range wallets {
				totalHeldMinor += w.Balance.Minor()
			}
			stats.TotalHeldInWallets = fmt.Sprintf("%s %s", money.FromMinor(totalHeldMinor).String(), i18n.T(lang, "common.currency_egp"))
		}
	}
	if h.catSvc != nil {
		if _, savingStats, err := h.catSvc.ListAllSavingProductsAdmin(sysCtx, nil, nil, "", "all", 1, 0); err == nil && savingStats != nil {
			stats.TotalSavingProducts = savingStats.TotalProducts
		}
	}
	if h.adminSvc != nil {
		if va, err := h.adminSvc.VisitorAnalytics(ctx, 10); err == nil && va != nil {
			stats.TotalVisitors = va.Total
			stats.TodayVisitors = va.Today
			stats.TopDevices = va.ByDevice
			stats.TopBrowsers = va.ByBrowser
			stats.TopLocations = va.ByCity
			stats.TotalGMV = va.TotalGMV
			if stats.TotalProducts == 0 {
				stats.TotalProducts = va.TotalProducts
			}
		}
	}
	if stats.TotalGMV != "" && !strings.HasPrefix(stats.TotalGMV, "0.00") && stats.TotalGMV != "0" {
		stats.TotalCommission = i18n.T(lang, "admin.dashboard.commission_5pct")
	} else {
		stats.TotalCommission = fmt.Sprintf("0.00 %s", i18n.T(lang, "common.currency_egp"))
	}
	if gwAdmin, _, ok := h.getGatewayAdminClient(ctx); ok && gwAdmin != nil {
		stats.GatewayOnline = true
	}
	stats.RecentOrganizations = recentOrganizations

	h.renderPage(ctx, w, "render admin dashboard page", pages.AdminDashboard(stats, pendingOrgs, lang, dir))
}
