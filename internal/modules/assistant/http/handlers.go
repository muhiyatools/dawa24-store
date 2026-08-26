package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// KeyResolver resolves tenant virtual key by organization ID.
type KeyResolver func(ctx context.Context, orgID int64) (string, error)

// Handler serves HTTP and SSE assistant endpoints.
type Handler struct {
	svc         *assistant.Service
	gwClient    gateway.Client
	repo        assistant.Repository
	limiter     *RateLimiter
	keyResolver KeyResolver
	log         *slog.Logger
	handleMu    sync.RWMutex
	handles     map[string]assistant.Attachment
}

// NewHandler constructs an assistant HTTP handler.
func NewHandler(svc *assistant.Service, gw gateway.Client, repo assistant.Repository, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{
		svc:      svc,
		gwClient: gw,
		repo:     repo,
		limiter:  NewRateLimiter(20, time.Minute),
		log:      log.With("handler", "assistant_http"),
		handles:  make(map[string]assistant.Attachment),
	}
}

// SetKeyResolver sets the tenant key resolver for gateway authorization.
func (h *Handler) SetKeyResolver(r KeyResolver) {
	h.keyResolver = r
}

// RegisterRoutes mounts assistant endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/assistant/messages", h.AssistantStream)
	r.Post("/api/v1/assistant/attachments", h.AssistantUpload)
	r.Post("/api/v1/assistant/transcribe", h.AssistantTranscribe)
	r.Get("/api/v1/assistant/health", h.AssistantHealth)
	r.Get("/api/v1/assistant/conversations", h.AssistantConversations)
	r.Get("/api/v1/assistant/conversations/{id}", h.AssistantHistory)
	r.Delete("/api/v1/assistant/conversations/{id}", h.AssistantDeleteConversation)
}

type streamRequestPayload struct {
	Text              string   `json:"text"`
	ConversationID    int64    `json:"conversation_id"`
	AttachmentHandles []string `json:"attachment_handles"`
}

// AssistantStream streams assistant completions over Server-Sent Events (SSE).
func (h *Handler) AssistantStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	if !h.limiter.Allow(actor.UserID) {
		http.Error(w, `{"error":"rate_limited","message":"لقد تجاوزت الحد المسموح من الطلبات، يرجى الانتظار دقيقة."}`, http.StatusTooManyRequests)
		return
	}

	var payload streamRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming_unsupported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Resolve attachment handles
	var resolvedAtts []assistant.Attachment
	h.handleMu.RLock()
	for _, handle := range payload.AttachmentHandles {
		if att, exists := h.handles[handle]; exists && att.UserID == actor.UserID {
			resolvedAtts = append(resolvedAtts, att)
		}
	}
	h.handleMu.RUnlock()

	// Build conversation turn with attachment preprocessing
	chatMsgs, plan, err := h.svc.BuildTurn(ctx, payload.ConversationID, payload.Text, resolvedAtts, func(stage, file string) {
		h.writeSSE(w, flusher, "status", map[string]string{
			"stage": stage,
			"file":  file,
		})
	})
	if err != nil {
		h.writeSSE(w, flusher, "error", map[string]string{
			"code":    "build_failed",
			"message": "تعذر تجهيز المحادثة والمرفقات.",
		})
		return
	}

	if len(plan.Rejected) > 0 {
		for _, rej := range plan.Rejected {
			h.writeSSE(w, flusher, "status", map[string]string{
				"stage":   "attachment_rejected",
				"file":    rej.Attachment.Filename,
				"reason":  rej.Reason,
				"message": fmt.Sprintf("تم استبعاد الملف %s: %s", rej.Attachment.Filename, rej.Reason),
			})
		}
	}

	var virtualKey string
	if h.keyResolver != nil && (actor.OrgID > 0 || actor.OrganizationID > 0) {
		oid := actor.OrgID
		if oid <= 0 {
			oid = actor.OrganizationID
		}
		if vk, err := h.keyResolver(ctx, oid); err == nil && vk != "" {
			virtualKey = vk
		}
	}

	chatReq := gateway.ChatRequest{
		Role:        gateway.RolePrimary,
		Messages:    chatMsgs,
		MaxTokens:   2048,
		Temperature: 0.7,
		OrgID:       actor.OrgID,
		UserID:      actor.UserID,
		VirtualKey:  virtualKey,
	}

	events, err := h.gwClient.Stream(ctx, chatReq)
	if err != nil {
		h.log.WarnContext(ctx, "failed to start gateway stream", "error", err)
		h.writeSSE(w, flusher, "error", map[string]string{
			"code":    "gateway_unavailable",
			"message": "خدمة المساعد الذكي غير متاحة حالياً، يرجى المحاولة لاحقاً.",
		})
		return
	}

	var fullAnswer strings.Builder
	var lastUsage *gateway.Usage

	for ev := range events {
		if ev.Err != nil {
			h.log.WarnContext(ctx, "stream event error", "error", ev.Err)
			h.writeSSE(w, flusher, "error", map[string]string{
				"code":    "stream_error",
				"message": "حدث انقطاع في تدفق الإجابة.",
			})
			return
		}
		if ev.Reasoning != "" {
			h.writeSSE(w, flusher, "reasoning", map[string]string{"text": ev.Reasoning})
		}
		if ev.Delta != "" {
			fullAnswer.WriteString(ev.Delta)
			h.writeSSE(w, flusher, "delta", map[string]string{"text": ev.Delta})
		}
		if ev.Usage != nil {
			lastUsage = ev.Usage
		}
		if ev.Done {
			// Persist turn if repo exists
			var convID = payload.ConversationID
			if h.repo != nil {
				if convID <= 0 {
					conv := &assistant.Conversation{
						OrganizationID: actor.OrgID,
						UserID:         actor.UserID,
						Title:          payload.Text,
					}
					if len(conv.Title) > 50 {
						conv.Title = conv.Title[:50] + "..."
					}
					_ = h.repo.CreateConversation(ctx, conv)
					convID = conv.ID
				}

				_ = h.repo.SaveMessage(ctx, &assistant.Message{
					ConversationID: convID,
					OrganizationID: actor.OrgID,
					Role:           "user",
					Content:        payload.Text,
					Attachments:    resolvedAtts,
					PromptVersion:  assistant.SystemPromptVersion,
					ModelRole:      string(gateway.RolePrimary),
				})

				inTok, outTok := 0, 0
				if lastUsage != nil {
					inTok = lastUsage.PromptTokens
					outTok = lastUsage.CompletionTokens
				}
				_ = h.repo.SaveMessage(ctx, &assistant.Message{
					ConversationID: convID,
					OrganizationID: actor.OrgID,
					Role:           "assistant",
					Content:        fullAnswer.String(),
					PromptVersion:  assistant.SystemPromptVersion,
					ModelRole:      string(gateway.RolePrimary),
					InputTokens:    inTok,
					OutputTokens:   outTok,
				})
			}

			h.writeSSE(w, flusher, "done", map[string]any{
				"conversation_id": convID,
				"usage":           lastUsage,
			})
			return
		}
	}
}

func (h *Handler) writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	bytesData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(bytesData))
	flusher.Flush()
}
