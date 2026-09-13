package telegram

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// GrantResolver is the platform's permission resolver. *rbac.Resolver
// satisfies it; nothing in this package computes a permission of its own.
type GrantResolver interface {
	Resolve(ctx context.Context, userID, orgID int64) (rbac.Grant, error)
}

// Answer is one finished assistant turn, already in the terms Telegram needs.
type Answer struct {
	// Markdown is the answer text, or the part written before a failure.
	Markdown       string
	ConversationID int64
	// Links are dashboard records the answer names, as absolute URLs.
	Links []AnswerLink
	// Failure is the user-facing sentence when the turn did not finish; empty
	// on success.
	Failure string
}

// AnswerLink is one record the answer referred to.
type AnswerLink struct {
	Title string
	URL   string
}

// Assistant is the Capsule assistant, seen from Telegram.
//
// Implemented in cmd/server over assistant.Service, so this module never
// imports that one. Allowed must be the very gate the browser endpoints use,
// and Ask re-applies it: an interface that forgot to ask still gets refused.
type Assistant interface {
	Allowed(actor authctx.Actor) bool
	// AllowQuestion is the per-user question rate limit, shared with the
	// browser drawer so Telegram is not a way around it.
	AllowQuestion(userID int64) bool
	Ask(ctx context.Context, actor authctx.Actor, conversationID int64, question string) Answer
}

// Candidate is one in-app notification that may be owed to a Telegram link.
type Candidate struct {
	LogID              int64
	LinkID             int64
	LinkActiveOrgID    int64
	MutedCategories    []string
	UserID             int64
	OrganizationID     int64
	OrganizationName   string
	Title              string
	Body               string
	RequiredPermission string
}

// Decision is the outcome recorded for one Candidate.
type Decision struct {
	LogID      int64
	LinkID     int64
	Category   Category
	Text       string
	DropReason string // empty: queue for delivery
}

// Repository is this module's persistence.
type Repository interface {
	// Link codes.
	CountLinkTokensSince(ctx context.Context, userID int64, since time.Time) (int, error)
	CreateLinkToken(ctx context.Context, userID int64, orgID *int64, hash []byte, expires time.Time) error
	ConsumeLinkToken(ctx context.Context, hash []byte) (userID int64, orgID *int64, err error)

	// Links.
	LiveLinkByTelegramUser(ctx context.Context, telegramUserID int64) (*Link, error)
	CurrentLinkForUser(ctx context.Context, userID int64) (*Link, error)
	CreatePendingLink(ctx context.Context, l *Link, confirmBy time.Time) error
	ConfirmPendingLink(ctx context.Context, userID int64, publicID string) (*Link, error)
	RevokeLink(ctx context.Context, userID, linkID int64) error
	SetLinkStatus(ctx context.Context, linkID int64, status LinkStatus) error
	TouchLink(ctx context.Context, linkID, chatID int64, username, displayName string) error
	SetActiveOrganization(ctx context.Context, linkID int64, orgID *int64) error
	SetConversation(ctx context.Context, linkID int64, conversationID *int64) error
	SetMutedCategories(ctx context.Context, linkID int64, muted []string) error
	AcquireBusy(ctx context.Context, linkID int64, until time.Time) (bool, error)
	ReleaseBusy(ctx context.Context, linkID int64) error
	MarkUpdateProcessed(ctx context.Context, updateID int64) (first bool, err error)

	// What the user belongs to, for display and selection only.
	Memberships(ctx context.Context, userID int64) ([]Membership, error)
	OffersTopicEnabled(ctx context.Context, userID int64) (bool, error)

	// Outbox.
	NotificationCandidates(ctx context.Context, since time.Time, limit int) ([]Candidate, error)
	RecordDecisions(ctx context.Context, decisions []Decision) error
	EnqueueSystemMessage(ctx context.Context, linkID int64, text string) error
	ClaimDeliveries(ctx context.Context, limit int, lease time.Duration) ([]Outgoing, error)
	MarkDelivered(ctx context.Context, id int64) error
	// RetryDelivery requeues a leased delivery, or fails it once attempts
	// reach maxAttempts. A zero retryAfter backs off exponentially.
	RetryDelivery(ctx context.Context, id int64, errText string, retryAfter time.Duration, maxAttempts int) error
	FailDeliveryAndBlockLink(ctx context.Context, id int64, errText string) error
	PurgeProcessedUpdates(ctx context.Context, olderThan time.Time) error
}
