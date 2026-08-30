package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockNotifRepo struct {
	logs   []*notifications.NotificationLog
	nextID int64
}

func (m *mockNotifRepo) CreateLog(_ context.Context, l *notifications.NotificationLog) error {
	m.nextID++
	l.ID = m.nextID
	m.logs = append(m.logs, l)
	return nil
}

func (m *mockNotifRepo) GetTemplateBySlug(_ context.Context, slug string) (*notifications.Template, error) {
	return nil, apperr.NotFound("template")
}

func (m *mockNotifRepo) ListUserNotifications(_ context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	var list []*notifications.NotificationLog
	for _, l := range m.logs {
		if l.UserID == userID {
			list = append(list, l)
		}
	}
	return list, nil
}

func (m *mockNotifRepo) MarkAsRead(_ context.Context, id int64, userID int64) error {
	for _, l := range m.logs {
		if l.ID == id && l.UserID == userID {
			l.IsRead = true
			return nil
		}
	}
	return apperr.NotFound("notification")
}

func (m *mockNotifRepo) GetUnreadCount(_ context.Context, userID int64) (int, error) {
	c := 0
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			c++
		}
	}
	return c, nil
}

func (m *mockNotifRepo) MarkAllAsRead(_ context.Context, userID int64) (int64, error) {
	var c int64
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			l.IsRead = true
			c++
		}
	}
	return c, nil
}

func (m *mockNotifRepo) ListUnread(_ context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	var list []*notifications.NotificationLog
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			list = append(list, l)
		}
	}
	return list, nil
}

func TestNotificationsDispatch_Comprehensive(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nRepo := &mockNotifRepo{logs: make([]*notifications.NotificationLog, 0)}
	notifSvc := notifications.NewService(nRepo, logger)

	handler := &UIHandler{
		notifSvc: notifSvc,
		log:      logger,
	}

	// 1. Account Registered Welcome Notification
	orgID := int64(100)
	handler.notifyAccountRegistered(ctx, 501, &orgID)
	assert.NotEmpty(t, nRepo.logs)
	lastLog := nRepo.logs[len(nRepo.logs)-1]
	assert.Equal(t, int64(501), lastLog.UserID)
	assert.Contains(t, lastLog.Title, "دواء 24")

	// 2. Org Approved / Rejected Notification
	handler.notifyOrgApproved(ctx, orgID)
	handler.notifyOrgRejected(ctx, orgID, "نقص السجل التجاري")

	// 3. Document Verified / Rejected Notification
	handler.notifyDocumentVerified(ctx, orgID, "السجل التجاري", true, "")
	handler.notifyDocumentVerified(ctx, orgID, "رخصة الصيدلية", false, "صورة غير واضحة")

	// 4. Wallet Deposit Approved / Rejected Notification
	amt, _ := money.Parse("5000.00")
	handler.notifyWalletDeposit(ctx, 501, orgID, amt, "approved")
	handler.notifyWalletDepositRejected(ctx, 501, orgID, amt, "عدم تطابق إيصال التحويل")

	// 5. Special Offer Approved / Rejected Notification
	handler.notifySpecialOfferStatus(ctx, orgID, "عرض خصم الصيف 20%", true, "")
	handler.notifySpecialOfferStatus(ctx, orgID, "عرض غير مطابق", false, "مخالفة الشروط")

	// 6. Sponsorship & Ad Status Notification
	handler.notifySponsorshipStatus(ctx, orgID, "الباقة الذهبية", true, "")
	handler.notifySponsorshipStatus(ctx, orgID, "الباقة الفضية", false, "انتهاء السعة")
	handler.notifyAdStatus(ctx, orgID, "بانر رئيسي", true, "")
	handler.notifyAdStatus(ctx, orgID, "بانر جانبي", false, "المقاسات غير متطابقة")

	// 7. Negotiation Offer & Decision Notification
	negAmt, _ := money.Parse("1200.00")
	handler.notifyNegotiationOffer(ctx, orgID, "صيدلية النور", "ORD-1234", negAmt)
	handler.notifyNegotiationDecision(ctx, 501, orgID, "شركة الأدوية المتحدة", "ORD-1234", true, "")
	handler.notifyNegotiationDecision(ctx, 501, orgID, "شركة الأدوية المتحدة", "ORD-1234", false, "السعر المقترح أقل من التكلفة")

	// 8. Quotes & RFQ Notification
	handler.notifyQuoteRequest(ctx, orgID, "صيدلية الشفاء", "بنادول إكسترا", 50)
	unitPrice, _ := money.Parse("45.50")
	handler.notifyQuoteProvided(ctx, 501, orgID, "شركة الأمل", "بنادول إكسترا", unitPrice)
	handler.notifyQuoteDecision(ctx, orgID, "صيدلية الشفاء", "بنادول إكسترا", true)
	handler.notifyQuoteDecision(ctx, orgID, "صيدلية الشفاء", "بنادول إكسترا", false)

	// Confirm that in-app notification logs were created without errors
	assert.Greater(t, len(nRepo.logs), 5)
}
