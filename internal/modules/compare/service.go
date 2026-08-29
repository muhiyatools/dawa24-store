package compare

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
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
	storage   *storage.Client
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

// SetStorage configures the object storage client for downloading uploaded files.
func (s *Service) SetStorage(st *storage.Client) {
	s.storage = st
}

// EntitlementFor answers "what may this user do in the compare tool right now?".
// All subscription/plan paywalls are removed as per user directive.
func (s *Service) EntitlementFor(ctx context.Context, userID, orgID int64) (Entitlement, error) {
	return Entitlement{
		Active:            true,
		PlanSlug:          "unlimited",
		MaxActiveFiles:    100,
		MaxSessions:       10,
		AIMatchingEnabled: true,
	}, nil
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

	// 50MB file size cap check
	if sizeBytes > 50*1024*1024 {
		return nil, nil, apperr.Validation("file.too_large", "File size exceeds 50MB limit.", nil)
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

// ListAllFiles lists all compare files across the system with optional search and status filter.
func (s *Service) ListAllFiles(ctx context.Context, search string, status *CompareFileStatus) ([]*CompareFile, error) {
	return s.repo.ListAllFiles(ctx, search, status)
}

// GetFile retrieves a file by ID.
func (s *Service) GetFile(ctx context.Context, fileID int64) (*CompareFile, error) {
	return s.repo.GetFileByID(ctx, fileID)
}

// GetFileByPublicID retrieves a file by public UUID.
func (s *Service) GetFileByPublicID(ctx context.Context, publicID string) (*CompareFile, error) {
	return s.repo.GetFileByPublicID(ctx, publicID)
}

// ListFileRows retrieves extracted rows for a compare file.
func (s *Service) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*CompareFileRow, error) {
	return s.repo.ListFileRows(ctx, fileID, limit, offset)
}

// SaveFileMapping validates and persists user-defined column mapping for a spreadsheet (Plan V5 Phase 2 §2.3.3).
// After saving the mapping, it automatically processes the file to extract rows.
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
	if err := s.repo.UpdateFile(ctx, file); err != nil {
		return err
	}

	// Automatically process the file after mapping is saved
	return s.ProcessCompareFile(ctx, fileID)
}

// UploadAndProcessCompareFile uploads a spreadsheet, detects columns, extracts rows, and stores them in compare.file_rows immediately.
func (s *Service) UploadAndProcessCompareFile(
	ctx context.Context, userID int64, orgID *int64, supplierName, originalFilename, mimeType string,
	sizeBytes int64, storageKey string, fileBytes []byte,
) (*CompareFile, []string, error) {
	file, archived, err := s.UploadCompareFile(ctx, userID, orgID, supplierName, originalFilename, mimeType, sizeBytes, storageKey)
	if err != nil {
		return nil, nil, err
	}

	if len(fileBytes) == 0 {
		return file, archived, nil
	}

	// 1. Read all rows from file using universal spreadsheet reader
	allRows, err := sheet.ReadRows(fileBytes, originalFilename)
	if err != nil || len(allRows) == 0 {
		file.Status = FileFailed
		file.ErrorMessage = "الملف فارغ أو تعذر قراءة الجداول بداخله"
		_ = s.repo.UpdateFile(ctx, file)
		return file, archived, nil
	}

	// 2. Find best header row and detect columns
	headerRowIdx, fieldMapping, _ := FindBestHeaderRow(allRows)
	var config MappingConfig
	colMapping := make(map[TargetField]*int)
	for colIdx, field := range fieldMapping {
		idx := colIdx
		colMapping[field] = &idx
	}
	config.NameCol = colMapping[FieldProductName]
	config.PriceCol = colMapping[FieldPrice]
	config.DiscountCol = colMapping[FieldDiscount]
	config.CodeCol = colMapping[FieldSKU]
	if config.CodeCol == nil {
		config.CodeCol = colMapping[FieldProductID]
	}

	// Heuristic fallbacks if columns were not auto-detected
	headerRow := allRows[headerRowIdx]
	if config.NameCol == nil && len(headerRow) > 0 {
		idx := 0
		if len(headerRow) > 1 {
			idx = 1
		}
		config.NameCol = &idx
	}
	if config.PriceCol == nil && len(headerRow) > 2 {
		idx := 2
		config.PriceCol = &idx
	}
	if config.DiscountCol == nil && len(headerRow) > 3 {
		idx := 3
		config.DiscountCol = &idx
	}
	if config.CodeCol == nil && len(headerRow) > 0 {
		idx := 0
		config.CodeCol = &idx
	}

	file.MappingConfig = config

	// 3. Extract and insert rows
	var rows []*CompareFileRow
	rowNum := 1
	for i := headerRowIdx + 1; i < len(allRows); i++ {
		record := allRows[i]
		if len(record) == 0 {
			continue
		}
		row := s.extractRowFromRecord(record, headerRow, file, rowNum)
		if row != nil {
			rows = append(rows, row)
			rowNum++
		}
	}

	if len(rows) > 0 {
		_ = s.repo.DeleteFileRows(ctx, file.ID)
		if insertErr := s.repo.InsertFileRows(ctx, rows); insertErr == nil {
			file.RowCount = len(rows)
			file.Status = FileReady
			file.ErrorMessage = ""
		} else {
			s.log.ErrorContext(ctx, "failed to insert compare file rows", "error", insertErr, "file_id", file.ID)
			file.Status = FileFailed
			file.ErrorMessage = "تعذر إدراج أصناف الملف في قاعدة البيانات"
		}
	} else {
		file.RowCount = 0
		file.Status = FileUploaded
		file.ErrorMessage = "لم يتم العثور على صفوف أصناف صالحة"
	}

	_ = s.repo.UpdateFile(ctx, file)
	return file, archived, nil
}

// ProcessCompareFile downloads the uploaded spreadsheet from storage or local disk, parses it using the
// saved column mapping, extracts rows, and inserts them into compare.file_rows.
func (s *Service) ProcessCompareFile(ctx context.Context, fileID int64) error {
	file, err := s.repo.GetFileByID(ctx, fileID)
	if err != nil {
		return err
	}

	if file.MappingConfig.NameCol == nil {
		return apperr.Validation("compare.no_mapping", "Column mapping not configured for this file.", nil)
	}

	var reader io.ReadCloser
	// 1. Try object storage if available
	if s.storage != nil && file.StorageKey != "" && !strings.HasPrefix(file.StorageKey, "/") && !strings.HasPrefix(file.StorageKey, "data/") {
		r, _, err := s.storage.Get(ctx, file.StorageKey)
		if err == nil {
			reader = r
		}
	}

	// 2. Try exact storage key on local disk
	if reader == nil && file.StorageKey != "" {
		cleanKey := strings.TrimPrefix(filepath.FromSlash(file.StorageKey), string(filepath.Separator))
		candidates := []string{
			file.StorageKey,
			filepath.Join("data", cleanKey),
			filepath.Join("data", "uploads", "compare", filepath.Base(file.StorageKey)),
			filepath.Join("data", "uploads", "compare", filepath.Base(file.OriginalFilename)),
			"data" + file.StorageKey,
		}
		for _, cand := range candidates {
			if f, err := os.Open(cand); err == nil {
				reader = f
				break
			}
		}
	}

	// 3. Scan data/uploads/compare directory for matching files
	if reader == nil {
		entries, _ := os.ReadDir(filepath.Join("data", "uploads", "compare"))
		for _, entry := range entries {
			if !entry.IsDir() && (strings.Contains(entry.Name(), file.OriginalFilename) || strings.HasSuffix(entry.Name(), filepath.Ext(file.OriginalFilename))) {
				if f, err := os.Open(filepath.Join("data", "uploads", "compare", entry.Name())); err == nil {
					reader = f
					break
				}
			}
		}
	}

	// 4. If raw file is unavailable but rows were already extracted in DB, keep file ready without crashing
	if reader == nil {
		if existingRows, _ := s.repo.ListFileRows(ctx, fileID, 1000, 0); len(existingRows) > 0 {
			file.RowCount = len(existingRows)
			file.Status = FileReady
			file.ErrorMessage = ""
			_ = s.repo.UpdateFile(ctx, file)
			return nil
		}
		return apperr.Internal(fmt.Errorf("تعذر العثور على ملف كشف الأسعار على السيرفر (معرف %d). يرجى إعادة رفع الملف.", fileID))
	}
	defer reader.Close()

	// Parse the spreadsheet by content, not by extension. Suppliers send
	// legacy BIFF .xls, HTML tables named .xls, and CSVs named .xlsx; the
	// universal reader sniffs the real container, where excelize alone rejects
	// everything that is not a true .xlsx.
	var rows []*CompareFileRow
	rows, err = s.parseSpreadsheet(reader, file)
	if err != nil {
		file.Status = FileFailed
		file.ErrorMessage = err.Error()
		_ = s.repo.UpdateFile(ctx, file)
		return fmt.Errorf("parse spreadsheet: %w", err)
	}

	// Delete any existing rows for this file (re-process case)
	if err := s.repo.DeleteFileRows(ctx, fileID); err != nil {
		return fmt.Errorf("delete old rows: %w", err)
	}

	// Insert new extracted rows
	if len(rows) > 0 {
		if err := s.repo.InsertFileRows(ctx, rows); err != nil {
			file.Status = FileFailed
			file.ErrorMessage = fmt.Sprintf("Failed to insert file rows: %v", err)
			_ = s.repo.UpdateFile(ctx, file)
			return fmt.Errorf("insert rows: %w", err)
		}
	}

	// Update file with row count and status
	file.RowCount = len(rows)
	file.Status = FileReady
	file.ErrorMessage = ""
	return s.repo.UpdateFile(ctx, file)
}

// parseSpreadsheet parses any supported workbook - .xlsx, legacy .xls, the XML
// 2003 dialect, an HTML table, or a delimited text file - using the column
// mapping saved for the file.
func (s *Service) parseSpreadsheet(reader io.Reader, file *CompareFile) ([]*CompareFileRow, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read spreadsheet data: %w", err)
	}

	allRows, err := sheet.ReadRows(data, file.OriginalFilename)
	if err != nil {
		return nil, fmt.Errorf("read spreadsheet: %w", err)
	}
	if len(allRows) == 0 {
		return nil, fmt.Errorf("empty sheet")
	}

	headerRowIdx, _, _ := FindBestHeaderRow(allRows)
	headers := allRows[headerRowIdx]

	var rows []*CompareFileRow
	rowNumber := 1

	for i := headerRowIdx + 1; i < len(allRows); i++ {
		columns := allRows[i]
		if len(columns) == 0 {
			continue
		}

		row := s.extractRowFromRecord(columns, headers, file, rowNumber)
		if row != nil {
			rows = append(rows, row)
			rowNumber++
		}
	}

	return rows, nil
}

// extractRowFromRecord extracts a CompareFileRow from a parsed record using the mapping config.
func (s *Service) extractRowFromRecord(record []string, headers []string, file *CompareFile, rowNumber int) *CompareFileRow {
	cfg := file.MappingConfig

	// Extract required fields using mapping config
	getValue := func(col *int) string {
		if col == nil {
			return ""
		}
		if *col >= 0 && *col < len(record) {
			return strings.TrimSpace(record[*col])
		}
		return ""
	}

	name := getValue(cfg.NameCol)
	if name == "" {
		return nil // Skip rows without product name
	}

	// Parse price
	priceStr := getValue(cfg.PriceCol)
	price := money.Zero
	if priceStr != "" {
		if val, err := strconv.ParseFloat(priceStr, 64); err == nil {
			price = money.FromMinor(int64(math.Round(val * 100)))
		} else {
			// Try to extract number from string
			if val, err := extractNumber(priceStr); err == nil {
				price = money.FromMinor(int64(math.Round(val * 100)))
			}
		}
	}
	// price and price_after_discount are numeric(12,2): ten integer digits.
	// A barcode read as a price would overflow and fail the batch.
	if price.Minor() < 0 || price.Minor() >= maxPriceMinor {
		price = money.Zero
	}

	// Parse discount
	discountStr := getValue(cfg.DiscountCol)
	discount := 0.0
	if discountStr != "" {
		if val, err := strconv.ParseFloat(discountStr, 64); err == nil {
			discount = val
		} else {
			if val, err := extractNumber(discountStr); err == nil {
				discount = val
			}
		}
	}
	// compare.file_rows.discount is numeric(5,2) and this value is a
	// percentage, so anything outside 0-100 is not a discount at all - it is
	// the auto-mapper having pointed at a price, a barcode or a pack size.
	// Storing it raw overflows the column, and because rows are inserted as one
	// batch a single such cell used to fail the whole file.
	if discount < 0 || discount > 100 {
		discount = 0
	}

	// Parse code/SKU
	code := getValue(cfg.CodeCol)

	// Calculate price after discount
	priceAfterDiscount := CalculatePriceAfterDiscount(price, discount)

	// Normalize name for matching
	normalizedName := arabic.Normalize(name)
	normalizedName = strings.ToLower(strings.TrimSpace(normalizedName))

	return &CompareFileRow{
		FileID:             file.ID,
		OrganizationID:     file.OrganizationID,
		RowNumber:          rowNumber,
		RawName:            name,
		NormalizedName:     normalizedName,
		SKU:                code,
		Price:              price,
		Discount:           discount,
		PriceAfterDiscount: priceAfterDiscount,
		MatchMethod:        MatchMethodNone,
	}
}

// maxPriceMinor is the exclusive ceiling for a price in minor units, set by
// compare.file_rows.price being numeric(12,2).
const maxPriceMinor int64 = 10_000_000_000

// extractNumber extracts the first number from a string, handling Arabic/Eastern numerals and commas.
func extractNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}

	// Convert Eastern Arabic numerals to standard digits
	s = strings.ReplaceAll(s, "٠", "0")
	s = strings.ReplaceAll(s, "١", "1")
	s = strings.ReplaceAll(s, "٢", "2")
	s = strings.ReplaceAll(s, "٣", "3")
	s = strings.ReplaceAll(s, "٤", "4")
	s = strings.ReplaceAll(s, "٥", "5")
	s = strings.ReplaceAll(s, "٦", "6")
	s = strings.ReplaceAll(s, "٧", "7")
	s = strings.ReplaceAll(s, "٨", "8")
	s = strings.ReplaceAll(s, "٩", "9")
	s = strings.ReplaceAll(s, "٫", ".")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, "ج.م", "")
	s = strings.ReplaceAll(s, "جم", "")
	s = strings.ReplaceAll(s, "EGP", "")
	s = strings.ReplaceAll(s, "egp", "")
	s = strings.ReplaceAll(s, "LE", "")
	s = strings.ReplaceAll(s, "le", "")
	s = strings.TrimSpace(s)

	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", "")
	} else if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ",", ".")
	}

	var numStr strings.Builder
	hasDot := false
	foundDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			numStr.WriteRune(r)
			foundDigit = true
		} else if r == '.' && !hasDot {
			numStr.WriteRune(r)
			hasDot = true
		} else if foundDigit && r != ' ' && r != '\t' {
			break
		}
	}
	if !foundDigit {
		return 0, fmt.Errorf("no number found")
	}
	return strconv.ParseFloat(numStr.String(), 64)
}

// ListMarketDiscounts retrieves market-wide approved discounts with full search and filtering.
func (s *Service) ListMarketDiscounts(ctx context.Context, filter MarketDiscountsFilter) (*MarketDiscountsResult, error) {
	return s.repo.ListMarketDiscounts(ctx, filter)
}

// GetFileRowsPaginated retrieves paginated rows for a specific warehouse file.
func (s *Service) GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*CompareFileRow, int64, error) {
	return s.repo.GetFileRowsPaginated(ctx, fileID, page, limit)
}

// DeleteFileRow deletes a single item from a temporary warehouse and decrements row_count.
func (s *Service) DeleteFileRow(ctx context.Context, rowID int64) error {
	return s.repo.DeleteFileRow(ctx, rowID)
}

// CreateFile creates a new compare/warehouse file record.
func (s *Service) CreateFile(ctx context.Context, f *CompareFile) error {
	return s.repo.CreateFile(ctx, f)
}

// UpdateFile updates compare/warehouse file metadata.
func (s *Service) UpdateFile(ctx context.Context, f *CompareFile) error {
	return s.repo.UpdateFile(ctx, f)
}

// InsertFileRows inserts rows for a compare/warehouse file.
func (s *Service) InsertFileRows(ctx context.Context, rows []*CompareFileRow) error {
	return s.repo.InsertFileRows(ctx, rows)
}
