package assistant

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// The agent loop.
//
// It is deliberately small. The model asks for data, the registry decides
// whether it may have it, the answer comes back, and the loop repeats until the
// model stops asking or the round budget runs out. There is no planner, no
// scratchpad, no self-critique step: every one of those costs a full round trip
// and none of them makes an answer about last month's spend more correct than
// reading last month's spend.
//
// What the loop does own is the two bounds that keep it from becoming
// expensive: a hard cap on rounds, and a hard cap on wall time. Both exist
// because an agent that cannot answer usually cannot answer with more tries
// either — it is asking the wrong question, and the useful response is to say
// so rather than to keep spending.

// maxToolRounds is how many times the model may call tools before it must
// answer. Four covers "list, then detail, then summarise, then answer", which
// is the deepest real question anybody has asked this assistant.
const maxToolRounds = 4

// turnDeadline bounds one whole question, tool calls included.
const turnDeadline = 90 * time.Second

// ToolOutcome is one dispatched tool call, ready to hand back to the model.
type ToolOutcome struct {
	CallID   string
	Name     string
	Content  string
	Decision string
	Rows     int
}

// ToolRunner is the assistant's view of the tool registry.
//
// Declared here and implemented in assistant/tools so that this package never
// imports that one: the loop knows there are tools, and knows nothing about
// what any of them do.
type ToolRunner interface {
	Schemas(actor authctx.Actor) []gateway.ToolSpec
	Dispatch(ctx context.Context, actor authctx.Actor, turnID int64, call gateway.ToolCall) ToolOutcome
}

// Emitter receives a turn's events as they happen. The HTTP layer implements it
// over the durable stream buffer, so a disconnect loses nothing.
type Emitter interface {
	Delta(text string)
	Reasoning(text string)
	Status(stage string, data map[string]any)
	Usage(input, output int)
	Done(answer string)
	Failed(code Code)
}

// KeyResolver returns the tenant's own Gateway virtual key, so consumption is
// billed to the منشأة that spent it rather than to the platform.
type KeyResolver func(ctx context.Context, orgID int64) (string, error)

// Service runs turns.
type Service struct {
	repo    Repository
	gateway gateway.Client
	tools   ToolRunner
	keys    KeyResolver
	log     *slog.Logger
}

// NewService constructs the assistant service.
func NewService(repo Repository, gw gateway.Client, runner ToolRunner, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:    repo,
		gateway: gw,
		tools:   runner,
		log:     log.With("module", "assistant"),
	}
}

// SetKeyResolver installs the tenant key lookup.
func (s *Service) SetKeyResolver(k KeyResolver) { s.keys = k }

// Repo exposes the repository to the HTTP layer, which owns conversation
// lifecycle. The service owns turns.
func (s *Service) Repo() Repository { return s.repo }

// ContextWindow reports the primary model's context size, for the usage meter.
func (s *Service) ContextWindow(ctx context.Context) int {
	if s.gateway == nil {
		return defaultContextWindow
	}
	caps, err := s.gateway.Capabilities(ctx, gateway.RolePrimary)
	if err != nil || caps.ContextWindow <= 0 {
		return defaultContextWindow
	}
	return caps.ContextWindow
}

// RunTurn answers one question, streaming as it goes.
//
// ctx here is NOT the HTTP request's context. The caller detaches it, so the
// turn keeps running when the browser goes away: the answer is finished, the
// tokens that were already bought are used, and the result is persisted where
// a reconnecting client can find it.
func (s *Service) RunTurn(
	ctx context.Context,
	actor authctx.Actor,
	cfg AgentConfig,
	turn *Turn,
	in TurnInput,
	em Emitter,
) {
	ctx, cancel := context.WithTimeout(ctx, turnDeadline)
	defer cancel()

	window := s.ContextWindow(ctx)
	messages := s.BuildMessages(ctx, actor, cfg, turn.ConversationID, in, window)

	var (
		answer     strings.Builder
		lastUsage  *gateway.Usage
		toolsUsed  int
		virtualKey string
	)
	if s.keys != nil && actor.OrgID > 0 {
		if vk, err := s.keys(ctx, actor.OrgID); err == nil {
			virtualKey = vk
		}
	}

	schemas := s.tools.Schemas(actor)

	for round := 0; round <= maxToolRounds; round++ {
		// The last permitted round drops the tools entirely. Left in, a model
		// that has decided to call something keeps calling it, hits the cap and
		// returns nothing; taken away, it answers from what it has already
		// read, which is what the user wanted three rounds ago.
		roundTools := schemas
		if round == maxToolRounds {
			roundTools = nil
		}

		events, err := s.gateway.Stream(ctx, gateway.ChatRequest{
			Role:        gateway.RolePrimary,
			Messages:    messages,
			Tools:       roundTools,
			MaxTokens:   2048,
			Temperature: 0.3,
			OrgID:       actor.OrgID,
			UserID:      actor.UserID,
			VirtualKey:  virtualKey,
			Feature:     matchflow.FeatureAssistant,
		})
		if err != nil {
			s.log.WarnContext(ctx, "assistant stream failed",
				"user_id", actor.UserID, "org_id", actor.OrgID, "error", err)
			s.fail(ctx, turn, em, ClassifyGateway(err), answer.String())
			return
		}

		text, calls, usage, streamErr := s.consume(events, em, &answer)
		if usage != nil {
			lastUsage = usage
		}
		if streamErr != nil {
			// Partial text is kept. An answer that was two thirds written is
			// worth more to the reader than an error message, and it has
			// already been paid for.
			s.fail(ctx, turn, em, ClassifyGateway(streamErr), answer.String())
			return
		}

		if len(calls) == 0 {
			s.succeed(ctx, actor, turn, in, em, answer.String(), lastUsage, toolsUsed)
			return
		}

		messages = append(messages, gateway.ChatMessage{
			Role:      "assistant",
			Text:      text,
			ToolCalls: calls,
		})
		for _, call := range calls {
			toolsUsed++
			em.Status("tool", map[string]any{"tool": call.Name, "state": "running"})

			outcome := s.tools.Dispatch(ctx, actor, turn.ID, call)
			em.Status("tool", map[string]any{
				"tool":  outcome.Name,
				"state": outcome.Decision,
				"rows":  outcome.Rows,
			})
			messages = append(messages, gateway.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Text:       outcome.Content,
			})
		}
	}

	// Round budget exhausted with the model still asking. Answer with what was
	// collected rather than with nothing.
	s.succeed(ctx, actor, turn, in, em, answer.String(), lastUsage, toolsUsed)
}

// consume drains one gateway stream, forwarding deltas as they arrive.
func (s *Service) consume(
	events <-chan gateway.StreamEvent, em Emitter, answer *strings.Builder,
) (string, []gateway.ToolCall, *gateway.Usage, error) {
	var (
		round strings.Builder
		calls []gateway.ToolCall
		usage *gateway.Usage
	)
	for ev := range events {
		switch {
		case ev.Err != nil:
			return round.String(), nil, usage, ev.Err
		case ev.Reasoning != "":
			em.Reasoning(ev.Reasoning)
		}
		if ev.Delta != "" {
			round.WriteString(ev.Delta)
			answer.WriteString(ev.Delta)
			em.Delta(ev.Delta)
		}
		if ev.Usage != nil {
			usage = ev.Usage
			em.Usage(ev.Usage.PromptTokens, ev.Usage.CompletionTokens)
		}
		if ev.Done {
			calls = ev.ToolCalls
		}
	}
	return round.String(), calls, usage, nil
}

// succeed persists the turn and tells the reader it is finished.
func (s *Service) succeed(
	ctx context.Context,
	actor authctx.Actor,
	turn *Turn,
	in TurnInput,
	em Emitter,
	answer string,
	usage *gateway.Usage,
	toolsUsed int,
) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = "لم أتمكن من صياغة إجابة لهذا السؤال. جرّب صياغة أوضح أو فترة أقصر."
	}
	turn.Status = TurnDone
	turn.Answer = answer
	turn.ToolCalls = toolsUsed
	if usage != nil {
		turn.InputTokens = usage.PromptTokens
		turn.OutputTokens = usage.CompletionTokens
	}
	s.persist(ctx, actor, turn, in, answer)
	em.Done(answer)
}

// fail persists whatever was produced and reports a user-facing code.
func (s *Service) fail(ctx context.Context, turn *Turn, em Emitter, code Code, partial string) {
	turn.Status = TurnFailed
	turn.ErrorCode = string(code)
	turn.Answer = strings.TrimSpace(partial)
	if s.repo != nil {
		// context.WithoutCancel: the turn may be failing precisely because the
		// deadline fired, and a cancelled context cannot write the record of
		// why.
		if err := s.repo.FinishTurn(context.WithoutCancel(ctx), turn); err != nil {
			s.log.ErrorContext(ctx, "assistant: finish failed turn", "error", err)
		}
	}
	em.Failed(code)
}

// persist writes the question, the answer and the turn record.
//
// Every write here uses a context detached from cancellation for the same
// reason: this runs at the end of a turn, which is exactly when the client is
// most likely to have gone.
func (s *Service) persist(
	ctx context.Context, actor authctx.Actor, turn *Turn, in TurnInput, answer string,
) {
	if s.repo == nil {
		return
	}
	saveCtx := context.WithoutCancel(ctx)

	question := strings.TrimSpace(in.Text)
	if question == "" && len(in.Attachments) > 0 {
		question = "مرفق: " + in.Attachments[0].Filename
	}

	if err := s.repo.SaveMessage(saveCtx, &Message{
		ConversationID: turn.ConversationID,
		OrganizationID: turn.OrganizationID,
		Role:           "user",
		Content:        question,
		Attachments:    in.Attachments,
		PromptVersion:  SystemPromptVersion,
		ModelRole:      string(gateway.RolePrimary),
	}); err != nil {
		s.log.ErrorContext(ctx, "assistant: save question", "error", err)
	}

	if err := s.repo.SaveMessage(saveCtx, &Message{
		ConversationID: turn.ConversationID,
		OrganizationID: turn.OrganizationID,
		Role:           "assistant",
		Content:        answer,
		PromptVersion:  SystemPromptVersion,
		ModelRole:      string(gateway.RolePrimary),
		InputTokens:    turn.InputTokens,
		OutputTokens:   turn.OutputTokens,
	}); err != nil {
		s.log.ErrorContext(ctx, "assistant: save answer", "error", err)
	}

	if ids := attachmentIDs(in.Attachments); len(ids) > 0 {
		if err := s.repo.MarkAttachmentsReferenced(saveCtx, ids, turn.ConversationID); err != nil {
			s.log.WarnContext(ctx, "assistant: mark attachments referenced", "error", err)
		}
	}

	if err := s.repo.FinishTurn(saveCtx, turn); err != nil {
		s.log.ErrorContext(ctx, "assistant: finish turn", "error", err)
	}
	_ = actor
}

func attachmentIDs(atts []Attachment) []int64 {
	var ids []int64
	for _, a := range atts {
		if a.RowID > 0 {
			ids = append(ids, a.RowID)
		}
	}
	return ids
}

// PurgeExpiredConversations deletes conversations six months after they were
// created. Called by the worker daily; see cmd/worker.
func (s *Service) PurgeExpiredConversations(ctx context.Context) (int, error) {
	if s.repo == nil {
		return 0, errors.New("assistant: no repository")
	}
	return s.repo.PurgeExpiredConversations(ctx, time.Now())
}

// PurgeOrphanAttachments removes uploads that were never sent with a question.
func (s *Service) PurgeOrphanAttachments(ctx context.Context) ([]string, error) {
	if s.repo == nil {
		return nil, errors.New("assistant: no repository")
	}
	return s.repo.PurgeOrphanAttachments(ctx, time.Now().Add(-24*time.Hour))
}
