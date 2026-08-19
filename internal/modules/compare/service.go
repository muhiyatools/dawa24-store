package compare

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ClientInfo captures client environment details for session device cap tracking.
type ClientInfo struct {
	SessionID       string
	DeviceUUID      string
	DeviceName      string
	DeviceType      string
	Platform        string
	PlatformVersion string
	Browser         string
	BrowserVersion  string
	IPAddress       string
	UserAgent       string
	Country         string
	City            string
}

// Service coordinates compare plans, subscriptions, user seats, and entitlements.
type Service struct {
	repo      Repository
	log       *slog.Logger
	aiMatcher AIMatcher
}

// NewService creates a new compare service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// SetAIMatcher configures the optional AI matching capability (Wave B / Plan V5 §2.6).
func (s *Service) SetAIMatcher(m AIMatcher) {
	s.aiMatcher = m
}

// EntitlementFor answers "what may this user do in the compare tool right now?" (Plan V5 Phase 2 §2.1.4).
func (s *Service) EntitlementFor(ctx context.Context, userID, orgID int64) (Entitlement, error) {
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}

	sub, err := s.repo.GetActiveSubscription(ctx, userID, orgPtr)
	if err != nil || sub == nil || !sub.IsValid() {
		return Entitlement{
			Active:            false,
			MaxActiveFiles:    0,
			MaxSessions:       0,
			AIMatchingEnabled: false,
		}, nil
	}

	ent := Entitlement{
		Active:            true,
		PlanSlug:          "",
		MaxActiveFiles:    8, // Default customer limit
		MaxSessions:       1, // Default single device
		AIMatchingEnabled: true,
		ExpiresAt:         sub.EndsAt,
	}

	if sub.Plan != nil {
		ent.PlanSlug = sub.Plan.Slug
		for _, f := range sub.Plan.Features {
			if !f.IsActive {
				continue
			}
			switch f.Key {
			case "max_active_files":
				if v, err := strconv.Atoi(f.Value); err == nil && v > 0 {
					ent.MaxActiveFiles = v
				}
			case "max_concurrent_sessions":
				if v, err := strconv.Atoi(f.Value); err == nil && v > 0 {
					ent.MaxSessions = v
				}
			case "ai_matching_enabled":
				ent.AIMatchingEnabled = strings.EqualFold(f.Value, "true") || f.Value == "1"
			}
		}
	}

	return ent, nil
}

// EnforceSessionCap logs the current user session and evicts oldest sessions if exceeding max allowed (Laravel parity).
func (s *Service) EnforceSessionCap(ctx context.Context, userID int64, subUserID *int64, maxSessions int, client ClientInfo) error {
	if maxSessions <= 0 {
		maxSessions = 1
	}

	sess := &UserSession{
		SubscriptionUserID: subUserID,
		UserID:             userID,
		SessionID:          client.SessionID,
		DeviceUUID:         client.DeviceUUID,
		IsActive:           true,
		DeviceName:         client.DeviceName,
		DeviceType:         client.DeviceType,
		Platform:           client.Platform,
		PlatformVersion:    client.PlatformVersion,
		Browser:            client.Browser,
		BrowserVersion:     client.BrowserVersion,
		IPAddress:          client.IPAddress,
		UserAgent:          client.UserAgent,
		Country:            client.Country,
		City:               client.City,
		LoggedInAt:         time.Now().UTC(),
		LastActivityAt:     time.Now().UTC(),
	}

	if err := s.repo.UpsertUserSession(ctx, sess); err != nil {
		return err
	}

	// Check active session count and evict oldest if necessary
	count, err := s.repo.CountActiveUserSessions(ctx, userID)
	if err != nil {
		return err
	}

	if count > maxSessions {
		s.log.InfoContext(ctx, "evicting oldest compare sessions for user", "user_id", userID, "active", count, "cap", maxSessions)
		return s.repo.EvictOldestSessions(ctx, userID, maxSessions)
	}

	return nil
}

// ListPlans returns public pricing tiers or all tiers for admins.
func (s *Service) ListPlans(ctx context.Context, onlyPublic bool) ([]*Plan, error) {
	return s.repo.ListPlans(ctx, onlyPublic)
}

// GetPlan retrieves a single plan by ID or slug.
func (s *Service) GetPlan(ctx context.Context, id int64) (*Plan, error) {
	return s.repo.GetPlanByID(ctx, id)
}

func (s *Service) GetPlanBySlug(ctx context.Context, slug string) (*Plan, error) {
	return s.repo.GetPlanBySlug(ctx, slug)
}

// CreatePlan adds a new plan (requires admin permission).
func (s *Service) CreatePlan(ctx context.Context, plan *Plan) (*Plan, error) {
	if plan.Slug == "" {
		return nil, apperr.Validation("plan.slug_required", "Plan slug is required.", nil)
	}
	if plan.Name.IsEmpty() {
		return nil, apperr.Validation("plan.name_required", "Plan name is required.", nil)
	}
	if err := s.repo.CreatePlan(ctx, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// UpdatePlan modifies plan metadata and prices.
func (s *Service) UpdatePlan(ctx context.Context, plan *Plan) error {
	return s.repo.UpdatePlan(ctx, plan)
}

// DeletePlan soft-deletes a plan.
func (s *Service) DeletePlan(ctx context.Context, id int64) error {
	return s.repo.DeletePlan(ctx, id)
}

// RequestPlan submits a self-serve enrollment request from a customer or vendor.
func (s *Service) RequestPlan(ctx context.Context, planID, orgID, userID int64, notes string) (*PlanRequest, error) {
	if planID <= 0 || orgID <= 0 || userID <= 0 {
		return nil, apperr.Validation("request.invalid_params", "Plan, organization, and user IDs are required.", nil)
	}

	req := &PlanRequest{
		PlanID:         planID,
		OrganizationID: orgID,
		UserID:         userID,
		Status:         RequestPending,
		Notes:          notes,
	}

	if err := s.repo.CreatePlanRequest(ctx, req); err != nil {
		return nil, err
	}
	return req, nil
}

// ListPlanRequests retrieves requests for an organization.
func (s *Service) ListPlanRequests(ctx context.Context, orgID int64) ([]*PlanRequest, error) {
	return s.repo.ListPlanRequestsByOrg(ctx, orgID)
}

// ListPendingPlanRequests lists requests for admin review.
func (s *Service) ListPendingPlanRequests(ctx context.Context) ([]*PlanRequest, error) {
	return s.repo.ListPendingPlanRequests(ctx)
}

// ReviewPlanRequest processes approval or rejection of a plan request by an administrator.
func (s *Service) ReviewPlanRequest(ctx context.Context, requestID int64, approve bool, reviewerID int64, reason string) error {
	req, err := s.repo.GetPlanRequestByID(ctx, requestID)
	if err != nil {
		return err
	}

	if !approve {
		return s.repo.ReviewPlanRequest(ctx, requestID, RequestRejected, reviewerID, reason)
	}

	// Approval creates the subscription
	plan, err := s.repo.GetPlanByID(ctx, req.PlanID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	var endsAt *time.Time
	if plan.TrialDays > 0 {
		t := now.AddDate(0, 0, plan.TrialDays)
		endsAt = &t
	} else {
		t := now.AddDate(0, 1, 0) // default 1 month
		endsAt = &t
	}

	sub := &Subscription{
		PlanID:         req.PlanID,
		OrganizationID: &req.OrganizationID,
		UserID:         req.UserID,
		BillingPeriod:  "monthly",
		PaymentMethod:  "cash",
		StartsAt:       now,
		EndsAt:         endsAt,
		Status:         SubActive,
		Seats:          1,
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return err
	}

	return s.repo.ReviewPlanRequest(ctx, requestID, RequestApproved, reviewerID, "Approved")
}

// SubscribeDirectly creates a direct active subscription (e.g. for self-serve or testing).
func (s *Service) SubscribeDirectly(ctx context.Context, planSlug string, orgID *int64, userID int64, period string) (*Subscription, error) {
	plan, err := s.repo.GetPlanBySlug(ctx, planSlug)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var endsAt *time.Time
	switch period {
	case "yearly":
		t := now.AddDate(1, 0, 0)
		endsAt = &t
	case "lifetime":
		endsAt = nil
	case "trial":
		days := plan.TrialDays
		if days <= 0 {
			days = 7
		}
		t := now.AddDate(0, 0, days)
		endsAt = &t
	default:
		t := now.AddDate(0, 1, 0)
		endsAt = &t
		period = "monthly"
	}

	sub := &Subscription{
		PlanID:         plan.ID,
		OrganizationID: orgID,
		UserID:         userID,
		BillingPeriod:  period,
		PaymentMethod:  "cash",
		StartsAt:       now,
		EndsAt:         endsAt,
		Status:         SubActive,
		Seats:          1,
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}
	sub.Plan = plan
	return sub, nil
}

// UploadCompareFile validates user entitlement, applies auto-archive retention if at max active capacity, and creates the compare file.
func (s *Service) UploadCompareFile(ctx context.Context, userID int64, orgID *int64, supplierName, originalFilename, mimeType string, sizeBytes int64, storageKey string) (*CompareFile, []string, error) {
	ent, err := s.EntitlementFor(ctx, userID, func() int64 {
		if orgID != nil {
			return *orgID
		}
		return 0
	}())
	if err != nil {
		return nil, nil, err
	}
	if !ent.Active {
		return nil, nil, apperr.Forbidden("compare.unentitled", "An active compare subscription is required to upload supplier files.")
	}

	if supplierName == "" {
		supplierName = strings.TrimSuffix(originalFilename, ".xlsx")
		supplierName = strings.TrimSuffix(supplierName, ".xls")
		supplierName = strings.TrimSuffix(supplierName, ".csv")
	}

	// 20MB file size cap check
	if sizeBytes > 20*1024*1024 {
		return nil, nil, apperr.Validation("file.too_large", "File size exceeds 20MB limit.", nil)
	}

	var archivedNames []string
	activeCount, err := s.repo.CountActiveFiles(ctx, userID, orgID)
	if err == nil && activeCount >= ent.MaxActiveFiles {
		// Needs room for 1 new file -> keep count must be MaxActiveFiles - 1
		keepCount := ent.MaxActiveFiles - 1
		if keepCount < 0 {
			keepCount = 0
		}
		reason := "تجاوز الحد الأقصى للملفات النشطة المسموح بها في باقتك (" + strconv.Itoa(ent.MaxActiveFiles) + ")"
		archivedNames, _ = s.repo.ArchiveOldestFiles(ctx, userID, orgID, keepCount, reason)
	}

	file := &CompareFile{
		OrganizationID:   orgID,
		UserID:           userID,
		SupplierName:     supplierName,
		OriginalFilename: originalFilename,
		StorageKey:       storageKey,
		MIMEType:         mimeType,
		SizeBytes:        sizeBytes,
		Status:           FileUploaded,
	}

	if err := s.repo.CreateFile(ctx, file); err != nil {
		return nil, nil, err
	}

	return file, archivedNames, nil
}

// RenameFile updates the user-assigned supplier label for a file.
func (s *Service) RenameFile(ctx context.Context, fileID int64, newSupplierName string) error {
	newSupplierName = strings.TrimSpace(newSupplierName)
	if newSupplierName == "" {
		return apperr.Validation("file.supplier_name_required", "Supplier name cannot be empty.", nil)
	}
	return s.repo.RenameFile(ctx, fileID, newSupplierName)
}

// ArchiveFile manually archives a file.
func (s *Service) ArchiveFile(ctx context.Context, fileID int64, reason string) error {
	if reason == "" {
		reason = "أرشفة يدوية من قبل المستخدم"
	}
	return s.repo.ArchiveFile(ctx, fileID, reason)
}

// UnarchiveFile restores an archived file.
func (s *Service) UnarchiveFile(ctx context.Context, fileID int64) error {
	return s.repo.UnarchiveFile(ctx, fileID)
}

// DeleteFile soft-deletes a file.
func (s *Service) DeleteFile(ctx context.Context, fileID int64) error {
	return s.repo.DeleteFile(ctx, fileID)
}

// ListFiles lists files for the given tenant / user.
func (s *Service) ListFiles(ctx context.Context, userID int64, orgID *int64, status *CompareFileStatus) ([]*CompareFile, error) {
	return s.repo.ListFiles(ctx, userID, orgID, status)
}

// GetFile retrieves a file by ID.
func (s *Service) GetFile(ctx context.Context, fileID int64) (*CompareFile, error) {
	return s.repo.GetFileByID(ctx, fileID)
}

// GetFileByPublicID retrieves a file by public UUID.
func (s *Service) GetFileByPublicID(ctx context.Context, publicID string) (*CompareFile, error) {
	return s.repo.GetFileByPublicID(ctx, publicID)
}

// SaveFileMapping validates and persists user-defined column mapping for a spreadsheet (Plan V5 Phase 2 §2.3.3).
func (s *Service) SaveFileMapping(ctx context.Context, fileID int64, config MappingConfig) error {
	if config.NameCol == nil {
		return apperr.Validation("mapping.name_required", "يرجى تحديد عمود اسم الصنف على الأقل.", map[string]string{
			"name_col": "اسم الصنف مطلوب لإتمام المقارنة",
		})
	}
	if config.PriceCol == nil && config.DiscountCol == nil {
		return apperr.Validation("mapping.price_or_discount_required", "يرجى تحديد عمود السعر أو الخصم على الأقل.", map[string]string{
			"price_col": "السعر أو الخصم مطلوب لإجراء المقارنة",
		})
	}

	file, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return err
	}

	file.MappingConfig = config
	file.Status = FileReady
	return s.repo.UpdateFile(ctx, file)
}
