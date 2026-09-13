package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Config is what the bridge needs to know about its surroundings.
type Config struct {
	// BotUsername is the bot's @name without the @, for t.me deep links.
	BotUsername string
	// BaseURL is the public site address, for links back into the dashboard.
	BaseURL string
}

const (
	// linkCodeTTL bounds how long a link code is worth anything.
	linkCodeTTL = 10 * time.Minute
	// confirmTTL bounds how long a pending link waits for the browser.
	confirmTTL = 10 * time.Minute
	// maxCodesPerWindow and codeWindow rate-limit link code creation.
	maxCodesPerWindow = 5
	codeWindow        = 10 * time.Minute
)

// Service is the Telegram bridge.
type Service struct {
	repo      Repository
	grants    GrantResolver
	assistant Assistant
	cfg       Config
	log       *slog.Logger
	now       func() time.Time
}

// NewService constructs the bridge.
func NewService(repo Repository, grants GrantResolver, assistant Assistant, cfg Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	cfg.BotUsername = strings.TrimPrefix(strings.TrimSpace(cfg.BotUsername), "@")
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &Service{
		repo:      repo,
		grants:    grants,
		assistant: assistant,
		cfg:       cfg,
		log:       log.With("module", "telegram"),
		now:       time.Now,
	}
}

// Enabled reports whether a bot is configured at all.
func (s *Service) Enabled() bool { return s != nil && s.cfg.BotUsername != "" }

// LinkCode is a freshly issued one-time code.
type LinkCode struct {
	DeepLink  string
	ExpiresAt time.Time
}

// StartLink issues a one-time link code for the signed-in user.
//
// The code is 32 random bytes; only its hash is stored. It remembers the
// organisation the user was working in, which becomes the chat's starting
// منشأة. Creating a new code invalidates any unused older one.
func (s *Service) StartLink(ctx context.Context, actor authctx.Actor) (*LinkCode, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if actor.UserID <= 0 {
		return nil, errors.New("telegram: start link requires a signed-in user")
	}
	ctx = database.AsSystem(ctx)
	n, err := s.repo.CountLinkTokensSince(ctx, actor.UserID, s.now().Add(-codeWindow))
	if err != nil {
		return nil, err
	}
	if n >= maxCodesPerWindow {
		return nil, ErrTooManyCodes
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("telegram: generate link code: %w", err)
	}
	// base64url without padding: 43 characters from [A-Za-z0-9_-], which is
	// exactly the alphabet and within the 64-character limit Telegram allows
	// for a /start payload.
	code := base64.RawURLEncoding.EncodeToString(raw)

	var orgID *int64
	if actor.OrgID > 0 {
		id := actor.OrgID
		orgID = &id
	}
	expires := s.now().Add(linkCodeTTL)
	if err := s.repo.CreateLinkToken(ctx, actor.UserID, orgID, hashCode(code), expires); err != nil {
		return nil, err
	}
	return &LinkCode{
		DeepLink:  "https://t.me/" + s.cfg.BotUsername + "?start=" + code,
		ExpiresAt: expires,
	}, nil
}

// CurrentLink returns the user's pending or confirmed link, nil when none.
func (s *Service) CurrentLink(ctx context.Context, userID int64) (*Link, error) {
	if userID <= 0 {
		return nil, nil
	}
	return s.repo.CurrentLinkForUser(database.AsSystem(ctx), userID)
}

// ConfirmLink is the browser half of linking: the signed-in user confirms that
// the Telegram account that opened their code is theirs.
//
// This second step is what makes a leaked code harmless. Whoever opens the
// code only creates a pending link showing their Telegram name; nothing is
// answered and nothing is delivered until the account owner, signed in to
// Dawa24, accepts it.
func (s *Service) ConfirmLink(ctx context.Context, actor authctx.Actor, publicID string) (*Link, error) {
	ctx = database.AsSystem(ctx)
	link, err := s.repo.ConfirmPendingLink(ctx, actor.UserID, publicID)
	if err != nil {
		return nil, err
	}
	msg := "✅ <b>تم ربط حسابك في Dawa24 بتيليجرام.</b>\n\n" +
		"يمكنك الآن سؤال المساعد كبسولة مباشرة هنا، وستصلك إشعارات منشأتك حسب صلاحياتك.\n" +
		"اكتب /help لعرض الأوامر."
	if err := s.repo.EnqueueSystemMessage(ctx, link.ID, msg); err != nil {
		s.log.WarnContext(ctx, "telegram: enqueue welcome", "error", err, "user_id", actor.UserID)
	}
	s.log.InfoContext(ctx, "telegram link confirmed", "user_id", actor.UserID, "telegram_user_id", link.TelegramUserID)
	return link, nil
}

// Unlink revokes the user's pending or confirmed link. Rejecting a pending
// link the user does not recognise is the same operation.
func (s *Service) Unlink(ctx context.Context, actor authctx.Actor) error {
	ctx = database.AsSystem(ctx)
	link, err := s.repo.CurrentLinkForUser(ctx, actor.UserID)
	if err != nil || link == nil {
		return err
	}
	if err := s.repo.RevokeLink(ctx, actor.UserID, link.ID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "telegram link revoked", "user_id", actor.UserID, "telegram_user_id", link.TelegramUserID)
	return nil
}

// SetMuted replaces the user's muted notification categories.
func (s *Service) SetMuted(ctx context.Context, actor authctx.Actor, muted []Category) error {
	ctx = database.AsSystem(ctx)
	link, err := s.repo.CurrentLinkForUser(ctx, actor.UserID)
	if err != nil {
		return err
	}
	if link == nil || link.Status == LinkPending {
		return ErrNoPendingLink
	}
	return s.repo.SetMutedCategories(ctx, link.ID, categoryKeys(muted))
}

func categoryKeys(cs []Category) []string {
	out := make([]string, 0, len(cs))
	seen := map[Category]bool{}
	for _, c := range cs {
		if _, ok := ParseCategory(string(c)); ok && !seen[c] {
			seen[c] = true
			out = append(out, string(c))
		}
	}
	return out
}

func hashCode(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}
