package ui_test

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockCompareRepoE2E struct {
	files    map[int64]*compare.CompareFile
	fileRows map[int64][]*compare.CompareFileRow
	nextID   int64
}

func newMockCompareRepoE2E() *mockCompareRepoE2E {
	return &mockCompareRepoE2E{
		files:    make(map[int64]*compare.CompareFile),
		fileRows: make(map[int64][]*compare.CompareFileRow),
		nextID:   1,
	}
}

func (m *mockCompareRepoE2E) ListPlans(ctx context.Context, onlyPublic bool) ([]*compare.Plan, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) GetPlanByID(ctx context.Context, id int64) (*compare.Plan, error) {
	return nil, apperr.NotFound("plan")
}
func (m *mockCompareRepoE2E) GetPlanBySlug(ctx context.Context, slug string) (*compare.Plan, error) {
	return nil, apperr.NotFound("plan")
}
func (m *mockCompareRepoE2E) CreatePlan(ctx context.Context, p *compare.Plan) error { return nil }
func (m *mockCompareRepoE2E) UpdatePlan(ctx context.Context, p *compare.Plan) error { return nil }
func (m *mockCompareRepoE2E) DeletePlan(ctx context.Context, id int64) error        { return nil }
func (m *mockCompareRepoE2E) ListPlanFeatures(ctx context.Context, planID int64) ([]*compare.PlanFeature, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) SetPlanFeature(ctx context.Context, feature *compare.PlanFeature) error {
	return nil
}
func (m *mockCompareRepoE2E) DeletePlanFeature(ctx context.Context, id int64) error { return nil }
func (m *mockCompareRepoE2E) CreatePlanRequest(ctx context.Context, r *compare.PlanRequest) error {
	return nil
}
func (m *mockCompareRepoE2E) GetPlanRequestByID(ctx context.Context, id int64) (*compare.PlanRequest, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ListPlanRequestsByOrg(ctx context.Context, orgID int64) ([]*compare.PlanRequest, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ListPendingPlanRequests(ctx context.Context) ([]*compare.PlanRequest, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ReviewPlanRequest(ctx context.Context, id int64, status compare.PlanRequestStatus, reviewerID int64, notes string) error {
	return nil
}
func (m *mockCompareRepoE2E) CreateSubscription(ctx context.Context, s *compare.Subscription) error {
	return nil
}
func (m *mockCompareRepoE2E) GetActiveSubscription(ctx context.Context, userID int64, orgID *int64) (*compare.Subscription, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) UpdateSubscription(ctx context.Context, s *compare.Subscription) error {
	return nil
}
func (m *mockCompareRepoE2E) AssignSubscriptionUser(ctx context.Context, subID, userID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) RevokeSubscriptionUser(ctx context.Context, subID, userID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) ListSubscriptionUsers(ctx context.Context, subID int64) ([]*compare.SubscriptionUser, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) UpsertUserSession(ctx context.Context, s *compare.UserSession) error {
	return nil
}
func (m *mockCompareRepoE2E) CountActiveUserSessions(ctx context.Context, userID int64) (int, error) {
	return 1, nil
}
func (m *mockCompareRepoE2E) EvictOldestSessions(ctx context.Context, userID int64, keep int) error {
	return nil
}
func (m *mockCompareRepoE2E) ListActiveSessions(ctx context.Context, userID int64) ([]*compare.UserSession, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) TerminateSession(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockCompareRepoE2E) DeactivateUserSession(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockCompareRepoE2E) TerminateUserSessions(ctx context.Context, userID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) CreateFile(ctx context.Context, f *compare.CompareFile) error {
	f.ID = m.nextID
	m.nextID++
	m.files[f.ID] = f
	return nil
}
func (m *mockCompareRepoE2E) GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error) {
	if f, ok := m.files[id]; ok {
		return f, nil
	}
	return nil, apperr.NotFound("compare file")
}
func (m *mockCompareRepoE2E) GetFileByPublicID(ctx context.Context, pid string) (*compare.CompareFile, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ListFiles(ctx context.Context, userID int64, orgID *int64, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	for _, f := range m.files {
		list = append(list, f)
	}
	return list, nil
}
func (m *mockCompareRepoE2E) ListAllFiles(ctx context.Context, search string, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	for _, f := range m.files {
		list = append(list, f)
	}
	return list, nil
}
func (m *mockCompareRepoE2E) ListAdminTempWarehouses(ctx context.Context, filter compare.AdminTempWarehouseFilter) ([]*compare.AdminTempWarehouse, error) {
	var list []*compare.AdminTempWarehouse
	for _, f := range m.files {
		list = append(list, &compare.AdminTempWarehouse{CompareFile: f})
	}
	return list, nil
}
func (m *mockCompareRepoE2E) ListAdminTempWarehousesWithTotal(ctx context.Context, filter compare.AdminTempWarehouseFilter, _, _ int) ([]*compare.AdminTempWarehouse, int, error) {
	list, err := m.ListAdminTempWarehouses(ctx, filter)
	return list, len(list), err
}
func (m *mockCompareRepoE2E) AdminTempWarehouseStats(ctx context.Context, filter compare.AdminTempWarehouseFilter) (int64, int, int, error) {
	var totalRows int64
	var activeCount, archivedCount int
	for _, f := range m.files {
		totalRows += int64(f.RowCount)
		if f.Status == compare.FileReady {
			activeCount++
		} else if f.Status == compare.FileArchived {
			archivedCount++
		}
	}
	return totalRows, activeCount, archivedCount, nil
}
func (m *mockCompareRepoE2E) ListTempWarehouseUploaders(ctx context.Context) ([]compare.FileUploader, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) SetFileVisibility(ctx context.Context, id int64, visibility string) error {
	if f := m.files[id]; f != nil {
		f.Visibility = visibility
	}
	return nil
}
func (m *mockCompareRepoE2E) CountActiveFiles(ctx context.Context, userID int64, orgID *int64) (int, error) {
	return len(m.files), nil
}
func (m *mockCompareRepoE2E) UpdateFile(ctx context.Context, f *compare.CompareFile) error {
	m.files[f.ID] = f
	return nil
}
func (m *mockCompareRepoE2E) RenameFile(ctx context.Context, id int64, name string) error {
	if f, ok := m.files[id]; ok {
		f.SupplierName = name
	}
	return nil
}
func (m *mockCompareRepoE2E) ArchiveOldestFiles(ctx context.Context, userID int64, orgID *int64, keep int, reason string) ([]string, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ArchiveFile(ctx context.Context, id int64, reason string) error {
	return nil
}
func (m *mockCompareRepoE2E) UnarchiveFile(ctx context.Context, id int64) error { return nil }
func (m *mockCompareRepoE2E) DeleteFile(ctx context.Context, id int64) error    { return nil }
func (m *mockCompareRepoE2E) BulkDeleteFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error) {
	return int64(len(ids)), nil
}
func (m *mockCompareRepoE2E) BulkArchiveFiles(ctx context.Context, ids []int64, ownerID *int64, reason string) (int64, error) {
	return int64(len(ids)), nil
}
func (m *mockCompareRepoE2E) BulkUnarchiveFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error) {
	return int64(len(ids)), nil
}
func (m *mockCompareRepoE2E) PurgeExpiredCompareFiles(ctx context.Context, defaultRetentionDays int) (int64, error) {
	return 0, nil
}
func (m *mockCompareRepoE2E) InsertFileRows(ctx context.Context, rows []*compare.CompareFileRow) error {
	for _, r := range rows {
		m.fileRows[r.FileID] = append(m.fileRows[r.FileID], r)
	}
	return nil
}
func (m *mockCompareRepoE2E) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*compare.CompareFileRow, error) {
	return m.fileRows[fileID], nil
}
func (m *mockCompareRepoE2E) GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*compare.CompareFileRow, int64, error) {
	rows := m.fileRows[fileID]
	return rows, int64(len(rows)), nil
}
func (m *mockCompareRepoE2E) DeleteFileRows(ctx context.Context, fileID int64) error {
	delete(m.fileRows, fileID)
	return nil
}
func (m *mockCompareRepoE2E) DeleteFileRowOwnedBy(ctx context.Context, rowID int64, ownerUserID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) DeleteFileRow(ctx context.Context, rowID int64) error {
	for fileID, rows := range m.fileRows {
		for i, r := range rows {
			if r.ID == rowID {
				m.fileRows[fileID] = append(rows[:i], rows[i+1:]...)
				return nil
			}
		}
	}
	return nil
}
func (m *mockCompareRepoE2E) GetSubscriptionByID(ctx context.Context, id int64) (*compare.Subscription, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) ListSubscriptionsByOrg(ctx context.Context, orgID int64) ([]*compare.Subscription, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) UpdateSubscriptionStatus(ctx context.Context, id int64, status compare.SubscriptionStatus) error {
	return nil
}
func (m *mockCompareRepoE2E) RemoveSubscriptionUser(ctx context.Context, subID int64, userID int64) error {
	return nil
}
func (m *mockCompareRepoE2E) IsUserAssignedToSubscription(ctx context.Context, subID int64, userID int64) (bool, error) {
	return true, nil
}
func (m *mockCompareRepoE2E) TouchUserSession(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockCompareRepoE2E) ListActiveUserSessions(ctx context.Context, userID int64) ([]*compare.UserSession, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) BulkUpdateFileRowMatches(ctx context.Context, fileID int64, matches []compare.RowMatch) error {
	return nil
}

func (m *mockCompareRepoE2E) UpdateFileRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, method compare.MatchMethod, confidence float64) error {
	return nil
}
func (m *mockCompareRepoE2E) SaveCustomerProductMapping(ctx context.Context, orgID *int64, rawName string, productID int64, source string) error {
	return nil
}
func (m *mockCompareRepoE2E) GetSavedProductMapping(ctx context.Context, orgID *int64, rawName string) (*int64, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) FindCandidateProducts(ctx context.Context, orgID *int64, query, sku string, limit int) ([]*compare.CandidateProduct, error) {
	return nil, nil
}
func (m *mockCompareRepoE2E) SearchFileRows(ctx context.Context, userID int64, orgID *int64, query string, limit int) ([]*compare.CompareFileRowWithSupplier, error) {
	var results []*compare.CompareFileRowWithSupplier
	cleanQ := strings.ToLower(strings.TrimSpace(query))
	for fileID, rows := range m.fileRows {
		file, ok := m.files[fileID]
		if !ok || file.DeletedAt != nil || file.Status != compare.FileReady {
			continue
		}
		for _, r := range rows {
			if cleanQ == "" || strings.Contains(strings.ToLower(r.RawName), cleanQ) || strings.Contains(strings.ToLower(r.NormalizedName), cleanQ) || strings.Contains(strings.ToLower(r.SKU), cleanQ) || strings.Contains(strings.ToLower(file.SupplierName), cleanQ) {
				results = append(results, &compare.CompareFileRowWithSupplier{
					CompareFileRow: *r,
					SupplierName:   file.SupplierName,
				})
			}
		}
	}
	return results, nil
}
func (m *mockCompareRepoE2E) ListDistinctSuppliers(ctx context.Context) ([]string, error) {
	return []string{"شركة الفتح", "شركة النصر للأدوية"}, nil
}
func (m *mockCompareRepoE2E) ListMarketDiscounts(ctx context.Context, filter compare.MarketDiscountsFilter) (*compare.MarketDiscountsResult, error) {
	var items []*compare.MarketDiscountRow
	for fileID, rows := range m.fileRows {
		file := m.files[fileID]
		for _, r := range rows {
			items = append(items, &compare.MarketDiscountRow{
				ID:                 r.ID,
				FileID:             r.FileID,
				SupplierName:       file.SupplierName,
				ProductName:        r.RawName,
				SKU:                r.SKU,
				OriginalPrice:      r.Price,
				DiscountPercent:    r.Discount,
				PriceAfterDiscount: r.PriceAfterDiscount,
				MatchedProductID:   r.MatchedProductID,
				InCatalog:          r.MatchedProductID != nil && *r.MatchedProductID > 0,
			})
		}
	}
	return &compare.MarketDiscountsResult{
		Items:              items,
		TotalCount:         int64(len(items)),
		AvailableSuppliers: []string{"شركة الفتح", "شركة النصر للأدوية"},
		Page:               1,
		Limit:              24,
		TotalPages:         1,
	}, nil
}

func (m *mockCompareRepoE2E) LoadMarketOffers(
	_ context.Context, opts compare.MarketScanOptions,
) ([]compare.MarketOffer, error) {
	var out []compare.MarketOffer
	for fileID, rows := range m.fileRows {
		if opts.ExcludeFileID > 0 && fileID == opts.ExcludeFileID {
			continue
		}
		supplier := ""
		for _, f := range m.files {
			if f != nil && f.ID == fileID {
				supplier = f.SupplierName
			}
		}
		for _, r := range rows {
			if r == nil || !r.Price.IsPositive() {
				continue
			}
			net := r.PriceAfterDiscount
			if net.IsZero() {
				net = compare.CalculatePriceAfterDiscount(r.Price, r.Discount)
			}
			out = append(out, compare.MarketOffer{
				RowID: r.ID, FileID: fileID, SupplierName: supplier,
				ProductName: r.RawName, SKU: r.SKU, ProductID: r.MatchedProductID,
				Price: r.Price, Discount: r.Discount, NetPrice: net,
			})
		}
	}
	return out, nil
}
