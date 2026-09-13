package main

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/telegram"
	telegramHTTP "github.com/muhiya/dawa24-store/internal/modules/telegram/http"
	telegramPostgres "github.com/muhiya/dawa24-store/internal/modules/telegram/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Telegram as a second door to Capsule.
//
// The telegram module may not import the assistant module, so the two meet
// here. capsuleBridge hands Telegram the very service the browser drawer uses
// — the same instance, with the same tool registry, gateway key resolver and
// rate limiter — rather than a second assistant assembled for the bot.

// capsuleBridge implements telegram.Assistant over the mounted assistant.
//
// It is bound after construction because the assistant is mounted inside the
// authenticated API group while the bridge routes sit outside it. Until it is
// bound, it refuses everything.
type capsuleBridge struct {
	mu            sync.RWMutex
	svc           *assistant.Service
	allowQuestion func(userID int64) bool
	baseURL       string
}

var _ telegram.Assistant = (*capsuleBridge)(nil)

func newCapsuleBridge(baseURL string) *capsuleBridge {
	return &capsuleBridge{baseURL: strings.TrimRight(baseURL, "/")}
}

func (c *capsuleBridge) bind(svc *assistant.Service, allowQuestion func(int64) bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.svc, c.allowQuestion = svc, allowQuestion
	c.mu.Unlock()
}

func (c *capsuleBridge) service() (*assistant.Service, func(int64) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.svc, c.allowQuestion
}

// Allowed is assistant.Allowed — the gate requireAssistant applies to every
// browser assistant route.
func (c *capsuleBridge) Allowed(actor authctx.Actor) bool {
	svc, _ := c.service()
	_, ok := assistant.Allowed(actor)
	return svc != nil && ok
}

func (c *capsuleBridge) AllowQuestion(userID int64) bool {
	_, allow := c.service()
	return allow != nil && allow(userID)
}

func (c *capsuleBridge) Ask(ctx context.Context, actor authctx.Actor, conversationID int64, question string) telegram.Answer {
	svc, _ := c.service()
	if svc == nil {
		return telegram.Answer{Failure: assistant.Fail(assistant.CodeGatewayUnavailable).Message}
	}
	res := svc.Ask(ctx, actor, conversationID, question)
	ans := telegram.Answer{Markdown: res.Answer, ConversationID: res.ConversationID}
	if res.Code != "" {
		ans.Failure = assistant.Fail(res.Code).Message
	}
	// Entity URLs are built by the assistant from rows it read, for the
	// caller's own dashboard. Only site-relative paths are accepted, so
	// nothing can turn a reference into a link to somewhere else.
	for _, e := range res.Entities {
		if !strings.HasPrefix(e.URL, "/") || strings.HasPrefix(e.URL, "//") || c.baseURL == "" {
			continue
		}
		title := e.Title
		if title == "" {
			title = e.Label
		}
		ans.Links = append(ans.Links, telegram.AnswerLink{Title: title, URL: c.baseURL + e.URL})
	}
	return ans
}

// mountTelegram wires the bridge and returns its service, or nil when the
// integration is not configured.
func mountTelegram(
	r chi.Router,
	cfg *config.Config,
	log *slog.Logger,
	db *database.DB,
	resolver *rbac.Resolver,
	capsule *capsuleBridge,
) *telegram.Service {
	if !cfg.Telegram.Enabled() {
		log.Info("telegram bridge disabled (TELEGRAM_BOT_USERNAME / TELEGRAM_BRIDGE_TOKEN not set)")
		return nil
	}
	svc := telegram.NewService(telegramPostgres.New(db), resolver, capsule, telegram.Config{
		BotUsername: cfg.Telegram.BotUsername,
		BaseURL:     cfg.BaseURL,
	}, log)
	telegramHTTP.NewBridge(svc, cfg.Telegram.BridgeToken, log).RegisterRoutes(r)
	log.Info("telegram bridge mounted", "bot", cfg.Telegram.BotUsername, "prefix", telegramHTTP.Prefix)
	return svc
}
