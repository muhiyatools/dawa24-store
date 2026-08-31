package assistant

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"
	"strings"
	"sync"

	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// Service orchestrates the conversational assistant, attachment preprocessing, and Gateway turns.
type Service struct {
	repo        Repository
	gateway     gateway.Client
	log         *slog.Logger
	digestMu    sync.RWMutex
	digestCache map[string]Digest // keyed by ContentHash
}

// NewService constructs a new assistant service.
func NewService(repo Repository, gw gateway.Client, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:        repo,
		gateway:     gw,
		log:         log.With("module", "assistant"),
		digestCache: make(map[string]Digest),
	}
}

// GetCachedDigest returns a pre-computed digest for a content hash if available.
func (s *Service) GetCachedDigest(hash string) (Digest, bool) {
	s.digestMu.RLock()
	defer s.digestMu.RUnlock()
	d, ok := s.digestCache[hash]
	return d, ok
}

// SetCachedDigest caches an attachment digest.
func (s *Service) SetCachedDigest(hash string, d Digest) {
	s.digestMu.Lock()
	defer s.digestMu.Unlock()
	s.digestCache[hash] = d
}

// ExecutePrePass runs the attachment understanding pass via RoleAttachment model (Voxtral).
func (s *Service) ExecutePrePass(ctx context.Context, atts []Attachment, onStatus func(filename string)) ([]Digest, error) {
	if len(atts) == 0 {
		return nil, nil
	}

	digests := make([]Digest, 0, len(atts))
	for _, a := range atts {
		if cached, ok := s.GetCachedDigest(a.ContentHash); ok && a.ContentHash != "" {
			digests = append(digests, cached)
			continue
		}

		if onStatus != nil {
			onStatus(a.Filename)
		}

		var partKind gateway.PartKind
		switch ClassifyMIME(a.MIMEType) {
		case KindAudio:
			partKind = gateway.PartAudio
		case KindImage:
			partKind = gateway.PartImage
		case KindVideo:
			partKind = gateway.PartVideo
		default:
			partKind = gateway.PartFile
		}

		contentParts := []gateway.ContentPart{
			{
				Kind:     partKind,
				DataURL:  a.DataURL,
				Filename: a.Filename,
				MIMEType: a.MIMEType,
			},
			{
				Kind: gateway.PartText,
				Text: fmt.Sprintf(i18n.TDefault("w4_mod.s_57"), a.Filename),
			},
		}

		preReq := gateway.ChatRequest{
			Role: gateway.RoleAttachment,
			Messages: []gateway.ChatMessage{
				{
					Role: "system",
					Text: i18n.TDefault("w4_mod.24_58"),
				},
				{
					Role:  "user",
					Parts: contentParts,
				},
			},
			MaxTokens:   1500,
			Temperature: 0.2,
			OrgID:       a.OrgID,
			UserID:      a.UserID,
		}

		events, err := s.gateway.Stream(ctx, preReq)
		if err != nil {
			s.log.WarnContext(ctx, "attachment prepass failed", "filename", a.Filename, "error", err)
			// Return fallback digest without crashing turn
			d := Digest{
				Filename:  a.Filename,
				Kind:      ClassifyMIME(a.MIMEType),
				Summary:   fmt.Sprintf(i18n.TDefault("w4_mod.s_59"), a.Filename),
				Truncated: false,
			}
			digests = append(digests, d)
			continue
		}

		var sb strings.Builder
		for ev := range events {
			if ev.Err != nil {
				s.log.WarnContext(ctx, "attachment stream error", "filename", a.Filename, "error", ev.Err)
				break
			}
			sb.WriteString(ev.Delta)
		}

		rawText := strings.TrimSpace(sb.String())
		truncated := false
		if len(rawText) > DigestMaxChars {
			rawText = rawText[:DigestMaxChars]
			truncated = true
		}

		digest := Digest{
			Filename:  a.Filename,
			Kind:      ClassifyMIME(a.MIMEType),
			Summary:   rawText,
			Truncated: truncated,
		}

		if a.ContentHash != "" {
			s.SetCachedDigest(a.ContentHash, digest)
		}
		digests = append(digests, digest)
	}

	return digests, nil
}

// BuildTurn compiles the conversation context, attachment digests, and direct multimodal parts.
func (s *Service) BuildTurn(
	ctx context.Context,
	convID int64,
	userText string,
	atts []Attachment,
	onStatus func(stage, file string),
) ([]gateway.ChatMessage, *Plan, error) {
	plan, err := s.PlanTurn(ctx, atts)
	if err != nil {
		return nil, nil, err
	}

	var prePassDigests []Digest
	if len(plan.PrePass) > 0 {
		if onStatus != nil {
			onStatus("analyzing_attachment", plan.PrePass[0].Filename)
		}
		digests, err := s.ExecutePrePass(ctx, plan.PrePass, func(f string) {
			if onStatus != nil {
				onStatus("analyzing_attachment", f)
			}
		})
		if err != nil {
			s.log.WarnContext(ctx, "failed prepass", "error", err)
		}
		prePassDigests = digests
	}

	// Compile user text with pre-pass summaries
	var userContent strings.Builder
	for _, d := range prePassDigests {
		userContent.WriteString(d.RenderBlock())
		userContent.WriteString("\n\n")
	}
	prompt := strings.TrimSpace(userText)
	if prompt == "" && (len(plan.DirectParts) > 0 || len(prePassDigests) > 0) {
		prompt = i18n.TDefault("w4_mod.w4str_60_60")
	}
	userContent.WriteString(prompt)

	var messages []gateway.ChatMessage
	messages = append(messages, gateway.ChatMessage{
		Role: "system",
		Text: DefaultSystemPrompt,
	})

	// Load prior turns if conversation exists
	if s.repo != nil && convID > 0 {
		if history, err := s.repo.ListMessages(ctx, convID, 20); err == nil {
			for _, m := range history {
				messages = append(messages, gateway.ChatMessage{
					Role: m.Role,
					Text: m.Content,
				})
			}
		}
	}

	// Final user message for this turn
	userMsg := gateway.ChatMessage{
		Role: "user",
		Text: userContent.String(),
	}
	if len(plan.DirectParts) > 0 {
		userMsg.Parts = append([]gateway.ContentPart{
			{Kind: gateway.PartText, Text: userContent.String()},
		}, plan.DirectParts...)
		userMsg.Text = ""
	}
	messages = append(messages, userMsg)

	return messages, &plan, nil
}
