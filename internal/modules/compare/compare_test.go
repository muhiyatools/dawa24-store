package compare_test

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockCompareRepo struct {
	plans         map[int64]*compare.Plan
	plansBySlug   map[string]*compare.Plan
	features      map[int64][]*compare.PlanFeature
	requests      map[int64]*compare.PlanRequest
	subscriptions map[int64]*compare.Subscription
	subUsers      map[int64][]*compare.SubscriptionUser
	sessions      map[int64][]*compare.UserSession
	files         map[int64]*compare.CompareFile
	fileRows      map[int64][]*compare.CompareFileRow
	savedMappings map[string]int64
	nextID        int64
}

func newMockCompareRepo() *mockCompareRepo {
	r := &mockCompareRepo{
		plans:         make(map[int64]*compare.Plan),
		plansBySlug:   make(map[string]*compare.Plan),
		features:      make(map[int64][]*compare.PlanFeature),
		requests:      make(map[int64]*compare.PlanRequest),
		subscriptions: make(map[int64]*compare.Subscription),
		subUsers:      make(map[int64][]*compare.SubscriptionUser),
		sessions:      make(map[int64][]*compare.UserSession),
		files:         make(map[int64]*compare.CompareFile),
		fileRows:      make(map[int64][]*compare.CompareFileRow),
		savedMappings: make(map[string]int64),
		nextID:        1,
	}

	// Seed basic plans
	basicPlan := &compare.Plan{
		ID:        1,
		Slug:      "compare-customer-basic",
		Name:      i18n.Text{"ar": "باقة الصيدليات", "en": "Pharmacy Basic"},
		IsActive:  true,
		IsPublic:  true,
		TrialDays: 7,
	}
	r.plans[1] = basicPlan
	r.plansBySlug[basicPlan.Slug] = basicPlan
	r.features[1] = []*compare.PlanFeature{
		{PlanID: 1, Key: "max_active_files", Value: "8", ValueType: "integer", IsActive: true},
		{PlanID: 1, Key: "max_concurrent_sessions", Value: "1", ValueType: "integer", IsActive: true},
		{PlanID: 1, Key: "ai_matching_enabled", Value: "true", ValueType: "boolean", IsActive: true},
	}
	basicPlan.Features = r.features[1]

	proPlan := &compare.Plan{
		ID:        2,
		Slug:      "compare-vendor-pro",
		Name:      i18n.Text{"ar": "باقة الموردين", "en": "Vendor Pro"},
		IsActive:  true,
		IsPublic:  true,
		TrialDays: 14,
	}
	r.plans[2] = proPlan
	r.plansBySlug[proPlan.Slug] = proPlan
	r.features[2] = []*compare.PlanFeature{
		{PlanID: 2, Key: "max_active_files", Value: "22", ValueType: "integer", IsActive: true},
		{PlanID: 2, Key: "max_concurrent_sessions", Value: "5", ValueType: "integer", IsActive: true},
		{PlanID: 2, Key: "ai_matching_enabled", Value: "true", ValueType: "boolean", IsActive: true},
	}
	proPlan.Features = r.features[2]

	r.nextID = 10
	return r
}

func (m *mockCompareRepo) ListPlans(ctx context.Context, onlyPublic bool) ([]*compare.Plan, error) {
	var list []*compare.Plan
	for _, p := range m.plans {
		if !onlyPublic || (p.IsActive && p.IsPublic) {
			list = append(list, p)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) GetPlanByID(ctx context.Context, id int64) (*compare.Plan, error) {
	if p, ok := m.plans[id]; ok {
		return p, nil
	}
	return nil, apperr.NotFound("plan")
}

func (m *mockCompareRepo) GetPlanBySlug(ctx context.Context, slug string) (*compare.Plan, error) {
	if p, ok := m.plansBySlug[slug]; ok {
		return p, nil
	}
	return nil, apperr.NotFound("plan")
}

func (m *mockCompareRepo) CreatePlan(ctx context.Context, plan *compare.Plan) error {
	m.nextID++
	plan.ID = m.nextID
	m.plans[plan.ID] = plan
	m.plansBySlug[plan.Slug] = plan
	return nil
}

func (m *mockCompareRepo) UpdatePlan(ctx context.Context, plan *compare.Plan) error {
	m.plans[plan.ID] = plan
	m.plansBySlug[plan.Slug] = plan
	return nil
}

func (m *mockCompareRepo) DeletePlan(ctx context.Context, id int64) error {
	delete(m.plans, id)
	return nil
}

func (m *mockCompareRepo) ListPlanFeatures(ctx context.Context, planID int64) ([]*compare.PlanFeature, error) {
	return m.features[planID], nil
}

func (m *mockCompareRepo) SetPlanFeature(ctx context.Context, feature *compare.PlanFeature) error {
	m.features[feature.PlanID] = append(m.features[feature.PlanID], feature)
	return nil
}

func (m *mockCompareRepo) DeletePlanFeature(ctx context.Context, id int64) error {
	return nil
}

func (m *mockCompareRepo) CreatePlanRequest(ctx context.Context, req *compare.PlanRequest) error {
	m.nextID++
	req.ID = m.nextID
	m.requests[req.ID] = req
	return nil
}

func (m *mockCompareRepo) GetPlanRequestByID(ctx context.Context, id int64) (*compare.PlanRequest, error) {
	if r, ok := m.requests[id]; ok {
		return r, nil
	}
	return nil, apperr.NotFound("plan request")
}

func (m *mockCompareRepo) ListPlanRequestsByOrg(ctx context.Context, orgID int64) ([]*compare.PlanRequest, error) {
	var list []*compare.PlanRequest
	for _, r := range m.requests {
		if r.OrganizationID == orgID {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) ListPendingPlanRequests(ctx context.Context) ([]*compare.PlanRequest, error) {
	var list []*compare.PlanRequest
	for _, r := range m.requests {
		if r.Status == compare.RequestPending {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) ReviewPlanRequest(ctx context.Context, id int64, status compare.PlanRequestStatus, reviewerID int64, reason string) error {
	if r, ok := m.requests[id]; ok {
		r.Status = status
		r.ReviewedBy = &reviewerID
		now := time.Now().UTC()
		r.ReviewedAt = &now
		r.RejectionReason = reason
		return nil
	}
	return apperr.NotFound("plan request")
}

func (m *mockCompareRepo) CreateSubscription(ctx context.Context, sub *compare.Subscription) error {
	m.nextID++
	sub.ID = m.nextID
	m.subscriptions[sub.ID] = sub
	m.subUsers[sub.ID] = append(m.subUsers[sub.ID], &compare.SubscriptionUser{
		ID:             m.nextID,
		SubscriptionID: sub.ID,
		UserID:         sub.UserID,
		IsActive:       true,
	})
	return nil
}

func (m *mockCompareRepo) GetSubscriptionByID(ctx context.Context, id int64) (*compare.Subscription, error) {
	if s, ok := m.subscriptions[id]; ok {
		return s, nil
	}
	return nil, apperr.NotFound("subscription")
}

func (m *mockCompareRepo) GetActiveSubscription(ctx context.Context, userID int64, orgID *int64) (*compare.Subscription, error) {
	for _, s := range m.subscriptions {
		if s.Status == compare.SubActive && (s.EndsAt == nil || s.EndsAt.After(time.Now().UTC())) {
			if s.UserID == userID || (orgID != nil && s.OrganizationID != nil && *s.OrganizationID == *orgID) {
				if s.Plan == nil {
					s.Plan, _ = m.GetPlanByID(ctx, s.PlanID)
				}
				return s, nil
			}
		}
	}
	return nil, apperr.NotFound("active subscription")
}

func (m *mockCompareRepo) ListSubscriptionsByOrg(ctx context.Context, orgID int64) ([]*compare.Subscription, error) {
	var list []*compare.Subscription
	for _, s := range m.subscriptions {
		if s.OrganizationID != nil && *s.OrganizationID == orgID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) UpdateSubscriptionStatus(ctx context.Context, id int64, status compare.SubscriptionStatus) error {
	if s, ok := m.subscriptions[id]; ok {
		s.Status = status
		return nil
	}
	return apperr.NotFound("subscription")
}

func (m *mockCompareRepo) AssignSubscriptionUser(ctx context.Context, subID int64, userID int64) error {
	m.subUsers[subID] = append(m.subUsers[subID], &compare.SubscriptionUser{
		SubscriptionID: subID,
		UserID:         userID,
		IsActive:       true,
	})
	return nil
}

func (m *mockCompareRepo) RemoveSubscriptionUser(ctx context.Context, subID int64, userID int64) error {
	for _, u := range m.subUsers[subID] {
		if u.UserID == userID {
			u.IsActive = false
		}
	}
	return nil
}

func (m *mockCompareRepo) ListSubscriptionUsers(ctx context.Context, subID int64) ([]*compare.SubscriptionUser, error) {
	return m.subUsers[subID], nil
}

func (m *mockCompareRepo) IsUserAssignedToSubscription(ctx context.Context, subID int64, userID int64) (bool, error) {
	for _, u := range m.subUsers[subID] {
		if u.UserID == userID && u.IsActive {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockCompareRepo) UpsertUserSession(ctx context.Context, sess *compare.UserSession) error {
	m.nextID++
	sess.ID = m.nextID
	m.sessions[sess.UserID] = append(m.sessions[sess.UserID], sess)
	return nil
}

func (m *mockCompareRepo) TouchUserSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *mockCompareRepo) CountActiveUserSessions(ctx context.Context, userID int64) (int, error) {
	count := 0
	for _, s := range m.sessions[userID] {
		if s.IsActive {
			count++
		}
	}
	return count, nil
}

func (m *mockCompareRepo) ListActiveUserSessions(ctx context.Context, userID int64) ([]*compare.UserSession, error) {
	var list []*compare.UserSession
	for _, s := range m.sessions[userID] {
		if s.IsActive {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) EvictOldestSessions(ctx context.Context, userID int64, keepCount int) error {
	sessions := m.sessions[userID]
	active := 0
	for _, s := range sessions {
		if s.IsActive {
			active++
		}
	}
	if active <= keepCount {
		return nil
	}
	toEvict := active - keepCount
	for _, s := range sessions {
		if s.IsActive && toEvict > 0 {
			s.IsActive = false
			now := time.Now().UTC()
			s.LoggedOutAt = &now
			toEvict--
		}
	}
	return nil
}

func (m *mockCompareRepo) DeactivateUserSession(ctx context.Context, sessionID string) error {
	for _, userSessions := range m.sessions {
		for _, s := range userSessions {
			if s.SessionID == sessionID {
				s.IsActive = false
			}
		}
	}
	return nil
}

// Compare files mock store
func (m *mockCompareRepo) CreateFile(ctx context.Context, f *compare.CompareFile) error {
	m.nextID++
	f.ID = m.nextID
	f.PublicID = "pub-file-" + string(rune(m.nextID))
	m.files[f.ID] = f
	return nil
}

func (m *mockCompareRepo) GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error) {
	if f, ok := m.files[id]; ok {
		return f, nil
	}
	return nil, apperr.NotFound("compare file")
}

func (m *mockCompareRepo) GetFileByPublicID(ctx context.Context, publicID string) (*compare.CompareFile, error) {
	for _, f := range m.files {
		if f.PublicID == publicID {
			return f, nil
		}
	}
	return nil, apperr.NotFound("compare file")
}

func (m *mockCompareRepo) ListFiles(ctx context.Context, userID int64, orgID *int64, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	for _, f := range m.files {
		if status != nil && f.Status != *status {
			continue
		}
		if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
			list = append(list, f)
		}
	}
	return list, nil
}

func (m *mockCompareRepo) CountActiveFiles(ctx context.Context, userID int64, orgID *int64) (int, error) {
	count := 0
	for _, f := range m.files {
		if f.Status != compare.FileArchived {
			if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
				count++
			}
		}
	}
	return count, nil
}

func (m *mockCompareRepo) UpdateFile(ctx context.Context, f *compare.CompareFile) error {
	m.files[f.ID] = f
	return nil
}

func (m *mockCompareRepo) RenameFile(ctx context.Context, id int64, newSupplierName string) error {
	if f, ok := m.files[id]; ok {
		f.SupplierName = newSupplierName
		return nil
	}
	return apperr.NotFound("compare file")
}

func (m *mockCompareRepo) ArchiveOldestFiles(ctx context.Context, userID int64, orgID *int64, keepCount int, reason string) ([]string, error) {
	var active []*compare.CompareFile
	for _, f := range m.files {
		if f.Status != compare.FileArchived {
			if (orgID != nil && f.OrganizationID != nil && *f.OrganizationID == *orgID) || (f.OrganizationID == nil && f.UserID == userID) {
				active = append(active, f)
			}
		}
	}
	if len(active) <= keepCount {
		return nil, nil
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	toArchiveCount := len(active) - keepCount
	var archivedNames []string
	for i := 0; i < toArchiveCount; i++ {
		f := active[i]
		f.Status = compare.FileArchived
		f.ArchiveReason = reason
		now := time.Now().UTC()
		f.ArchivedAt = &now
		archivedNames = append(archivedNames, f.SupplierName)
		f.SupplierName = f.SupplierName + " - مؤرشف 1"
	}
	return archivedNames, nil
}

func (m *mockCompareRepo) ArchiveFile(ctx context.Context, id int64, reason string) error {
	if f, ok := m.files[id]; ok {
		f.Status = compare.FileArchived
		f.ArchiveReason = reason
		now := time.Now().UTC()
		f.ArchivedAt = &now
		return nil
	}
	return apperr.NotFound("compare file")
}

func (m *mockCompareRepo) UnarchiveFile(ctx context.Context, id int64) error {
	if f, ok := m.files[id]; ok {
		f.Status = compare.FileReady
		f.ArchivedAt = nil
		f.ArchiveReason = ""
		return nil
	}
	return apperr.NotFound("compare file")
}

func (m *mockCompareRepo) DeleteFile(ctx context.Context, id int64) error {
	delete(m.files, id)
	return nil
}

func (m *mockCompareRepo) InsertFileRows(ctx context.Context, rows []*compare.CompareFileRow) error {
	for _, r := range rows {
		m.nextID++
		r.ID = m.nextID
		m.fileRows[r.FileID] = append(m.fileRows[r.FileID], r)
	}
	return nil
}

func (m *mockCompareRepo) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*compare.CompareFileRow, error) {
	return m.fileRows[fileID], nil
}

func (m *mockCompareRepo) DeleteFileRows(ctx context.Context, fileID int64) error {
	delete(m.fileRows, fileID)
	return nil
}

func (m *mockCompareRepo) UpdateFileRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, method compare.MatchMethod, confidence float64) error {
	for _, rows := range m.fileRows {
		for _, r := range rows {
			if r.ID == rowID {
				r.MatchedProductID = matchedProductID
				r.MatchMethod = method
				r.MatchConfidence = confidence
				return nil
			}
		}
	}
	return nil
}

func (m *mockCompareRepo) SaveCustomerProductMapping(ctx context.Context, orgID *int64, rawName string, productID int64, source string) error {
	m.savedMappings[rawName] = productID
	return nil
}

func (m *mockCompareRepo) GetSavedProductMapping(ctx context.Context, orgID *int64, rawName string) (*int64, error) {
	if id, ok := m.savedMappings[rawName]; ok {
		return &id, nil
	}
	return nil, nil
}

func (m *mockCompareRepo) FindCandidateProducts(ctx context.Context, orgID *int64, query, sku string, limit int) ([]*compare.CandidateProduct, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEntitlementResolution(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	// Free unlimited platform feature access
	ent, err := svc.EntitlementFor(ctx, 101, 201)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ent.Active {
		t.Errorf("expected active entitlement for all users")
	}
	if ent.MaxActiveFiles < 10 {
		t.Errorf("expected high active file limit, got %d", ent.MaxActiveFiles)
	}
	if !ent.AIMatchingEnabled {
		t.Errorf("expected AIMatchingEnabled = true")
	}
}

func TestSessionCapEvictionParity(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(301)
	maxSessions := 2

	// Log in first 2 sessions
	_ = svc.EnforceSessionCap(ctx, userID, nil, maxSessions, compare.ClientInfo{SessionID: "sess-1", DeviceName: "Chrome on Windows"})
	_ = svc.EnforceSessionCap(ctx, userID, nil, maxSessions, compare.ClientInfo{SessionID: "sess-2", DeviceName: "Safari on iPhone"})

	count, _ := repo.CountActiveUserSessions(ctx, userID)
	if count != 2 {
		t.Fatalf("expected 2 active sessions, got %d", count)
	}

	// Log in 3rd session -> should evict the oldest session (sess-1)
	_ = svc.EnforceSessionCap(ctx, userID, nil, maxSessions, compare.ClientInfo{SessionID: "sess-3", DeviceName: "Firefox on Mac"})

	count, _ = repo.CountActiveUserSessions(ctx, userID)
	if count != 2 {
		t.Errorf("expected session cap of 2 maintained after eviction, got %d", count)
	}

	activeList, _ := repo.ListActiveUserSessions(ctx, userID)
	if len(activeList) != 2 {
		t.Fatalf("expected 2 active sessions in list")
	}
	if activeList[0].SessionID == "sess-1" {
		t.Errorf("expected sess-1 to be evicted, but it was still active")
	}
}

func TestPlanRequestReviewFlow(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	req, err := svc.RequestPlan(ctx, 2, 401, 501, "Please approve our vendor pro comparison plan")
	if err != nil {
		t.Fatalf("failed to request plan: %v", err)
	}
	if req.Status != compare.RequestPending {
		t.Errorf("expected pending status, got %s", req.Status)
	}

	// Admin approves
	err = svc.ReviewPlanRequest(ctx, req.ID, true, 999, "Approved by admin")
	if err != nil {
		t.Fatalf("failed to approve plan request: %v", err)
	}

	// User should now have active entitlement for compare
	ent, err := svc.EntitlementFor(ctx, 501, 401)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ent.Active {
		t.Errorf("expected active entitlement after plan approval")
	}
}

func TestPlanValidation(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	// Missing slug
	_, err := svc.CreatePlan(ctx, &compare.Plan{
		Name: i18n.Text{"ar": "خطة جديدة"},
	})
	if err == nil {
		t.Errorf("expected error creating plan with missing slug")
	}

	// Missing name
	_, err = svc.CreatePlan(ctx, &compare.Plan{
		Slug: "valid-slug",
	})
	if err == nil {
		t.Errorf("expected error creating plan with missing name")
	}

	// Valid creation
	created, err := svc.CreatePlan(ctx, &compare.Plan{
		Slug:         "custom-plan",
		Name:         i18n.Text{"ar": "خطة مخصصة", "en": "Custom Plan"},
		PriceMonthly: money.FromMajor(500),
	})
	if err != nil {
		t.Fatalf("failed to create valid plan: %v", err)
	}
	if created.ID <= 0 {
		t.Errorf("expected valid ID assigned to created plan")
	}
}

func TestArchiveRetentionPolicyAndQuota(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	userID := int64(701)
	orgID := int64(801)

	// Direct upload succeeds with free platform entitlement
	f1, _, err := svc.UploadCompareFile(ctx, userID, &orgID, "Sup-1", "file1.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", 1024, "key1")
	if err != nil {
		t.Fatalf("expected upload to succeed: %v", err)
	}
	if f1 == nil || f1.ID <= 0 {
		t.Errorf("expected valid created file")
	}

	// Rename file
	err = svc.RenameFile(ctx, f1.ID, "Supplier I (Cairo Branch)")
	if err != nil {
		t.Fatalf("failed to rename file: %v", err)
	}
	f, _ := svc.GetFile(ctx, f1.ID)
	if f.SupplierName != "Supplier I (Cairo Branch)" {
		t.Errorf("expected renamed supplier label, got %s", f.SupplierName)
	}

	// Manual archive and unarchive
	err = svc.ArchiveFile(ctx, f1.ID, "Seasonal pause")
	if err != nil {
		t.Fatalf("failed to manually archive file: %v", err)
	}
	f, _ = svc.GetFile(ctx, f1.ID)
	if f.Status != compare.FileArchived {
		t.Errorf("expected status archived")
	}

	err = svc.UnarchiveFile(ctx, f1.ID)
	if err != nil {
		t.Fatalf("failed to unarchive file: %v", err)
	}
	f, _ = svc.GetFile(ctx, f1.ID)
	if f.Status != compare.FileReady && f.Status != compare.FileUploaded {
		t.Errorf("expected status active after unarchive")
	}
}
