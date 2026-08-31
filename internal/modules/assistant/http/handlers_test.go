package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

type mockGateway struct {
	mu          sync.Mutex
	streamCalls int
	events      []gateway.StreamEvent
	streamErr   error
	transOutput string
	transErr    error
}

func (m *mockGateway) Invoke(ctx context.Context, req gateway.Request) (*gateway.Response, error) {
	return nil, nil
}

func (m *mockGateway) Stream(ctx context.Context, req gateway.ChatRequest) (<-chan gateway.StreamEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamCalls++
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	ch := make(chan gateway.StreamEvent, len(m.events)+1)
	for _, ev := range m.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func (m *mockGateway) Transcribe(ctx context.Context, audio io.Reader, filename, mime string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transErr != nil {
		return "", m.transErr
	}
	return m.transOutput, nil
}

func (m *mockGateway) Capabilities(ctx context.Context, role gateway.Role) (gateway.ModelCapabilities, error) {
	return gateway.ModelCapabilities{
		Vision:          true,
		Video:           true,
		Documents:       true,
		Audio:           true,
		MaxAttachmentMB: 10,
	}, nil
}

func (m *mockGateway) Health(ctx context.Context) error {
	return nil
}

func (m *mockGateway) Enabled() bool {
	return true
}

type memoryRepo struct {
	mu       sync.Mutex
	convs    map[int64]*assistant.Conversation
	messages map[int64][]*assistant.Message
	nextID   int64
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		convs:    make(map[int64]*assistant.Conversation),
		messages: make(map[int64][]*assistant.Message),
	}
}

func (m *memoryRepo) CreateConversation(ctx context.Context, c *assistant.Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	c.ID = m.nextID
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	m.convs[c.ID] = c
	return nil
}

func (m *memoryRepo) GetConversation(ctx context.Context, id int64) (*assistant.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.convs[id], nil
}

func (m *memoryRepo) GetConversationSummary(ctx context.Context, id int64) (*assistant.ConversationSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.convs[id]
	if c == nil {
		return nil, nil
	}
	return &assistant.ConversationSummary{
		ID:             c.ID,
		PublicID:       c.PublicID,
		OrganizationID: c.OrganizationID,
		UserID:         c.UserID,
		Title:          c.Title,
		MessageCount:   len(m.messages[c.ID]),
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}, nil
}

func (m *memoryRepo) ListAllConversations(ctx context.Context, search string, limit, offset int) ([]*assistant.ConversationSummary, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*assistant.ConversationSummary
	for _, c := range m.convs {
		if search == "" || strings.Contains(strings.ToLower(c.Title), strings.ToLower(search)) {
			list = append(list, &assistant.ConversationSummary{
				ID:             c.ID,
				PublicID:       c.PublicID,
				OrganizationID: c.OrganizationID,
				UserID:         c.UserID,
				Title:          c.Title,
				MessageCount:   len(m.messages[c.ID]),
				CreatedAt:      c.CreatedAt,
				UpdatedAt:      c.UpdatedAt,
			})
		}
	}
	return list, len(list), nil
}

func (m *memoryRepo) GetAssistantStats(ctx context.Context) (*assistant.AssistantStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	totalMsgs := 0
	for _, ms := range m.messages {
		totalMsgs += len(ms)
	}
	return &assistant.AssistantStats{
		TotalConversations: len(m.convs),
		TotalMessages:      totalMsgs,
		ActiveUsers:        len(m.convs),
	}, nil
}

func (m *memoryRepo) DeleteConversation(ctx context.Context, id int64, orgID, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.convs, id)
	delete(m.messages, id)
	return nil
}

func (m *memoryRepo) ListConversations(ctx context.Context, orgID, userID int64, limit, offset int) ([]*assistant.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*assistant.Conversation
	for _, c := range m.convs {
		if c.OrganizationID == orgID && c.UserID == userID {
			list = append(list, c)
		}
	}
	return list, nil
}

func (m *memoryRepo) SaveMessage(ctx context.Context, msg *assistant.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	msg.ID = m.nextID
	msg.CreatedAt = time.Now()
	m.messages[msg.ConversationID] = append(m.messages[msg.ConversationID], msg)
	return nil
}

func (m *memoryRepo) ListMessages(ctx context.Context, convID int64, limit int) ([]*assistant.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages[convID], nil
}

func authContext(userID, orgID int64) context.Context {
	return authctx.WithActor(context.Background(), authctx.Actor{
		UserID: userID,
		OrgID:  orgID,
		Role:   "customer",
	})
}

func TestPhase4_SSEStreamingEndpoint(t *testing.T) {
	gw := &mockGateway{
		events: []gateway.StreamEvent{
			{Reasoning: "خطوة تفكير"},
			{Delta: "مرحبا "},
			{Delta: "بك"},
			{Done: true, Usage: &gateway.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
		},
	}
	repo := newMemoryRepo()
	svc := assistant.NewService(repo, gw, slog.Default())
	handler := NewHandler(svc, gw, repo, slog.Default())

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	reqBody := `{"text":"مرحبا","conversation_id":0}`
	req := httptest.NewRequest("POST", "/api/v1/assistant/messages", strings.NewReader(reqBody))
	req = req.WithContext(authContext(42, 10))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	// T4.1: incremental frames
	if !strings.Contains(body, "event: delta\ndata: {\"text\":\"مرحبا \"}\n\n") {
		t.Errorf("T4.1 failed: missing delta frame: %s", body)
	}
	if !strings.Contains(body, "event: reasoning\ndata: {\"text\":\"خطوة تفكير\"}\n\n") {
		t.Errorf("T4.1 failed: missing reasoning frame: %s", body)
	}
	if !strings.Contains(body, "event: done\n") {
		t.Errorf("T4.1 failed: missing done frame: %s", body)
	}

	// T7.1: Persisted message verification
	if len(repo.convs) != 1 {
		t.Fatalf("T7.1 failed: expected 1 conversation created, got %d", len(repo.convs))
	}
	msgs := repo.messages[1]
	if len(msgs) != 2 {
		t.Fatalf("T7.1 failed: expected 2 messages (user + assistant), got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "مرحبا" {
		t.Errorf("T7.1 failed: user msg incorrect: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "مرحبا بك" {
		t.Errorf("T7.1 failed: assistant msg incorrect: %+v", msgs[1])
	}
}

func TestPhase4_RateLimiter(t *testing.T) {
	// T4.3: Rate limiter returns 429 when max requests reached
	gw := &mockGateway{}
	svc := assistant.NewService(nil, gw, slog.Default())
	handler := NewHandler(svc, gw, nil, slog.Default())
	handler.limiter = NewRateLimiter(2, time.Minute)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	call := func() int {
		req := httptest.NewRequest("POST", "/api/v1/assistant/messages", strings.NewReader(`{"text":"hi"}`))
		req = req.WithContext(authContext(100, 10))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call(); code != http.StatusOK {
		t.Fatalf("call 1: expected 200, got %d", code)
	}
	if code := call(); code != http.StatusOK {
		t.Fatalf("call 2: expected 200, got %d", code)
	}
	// Call 3 should be rate limited (429)
	if code := call(); code != http.StatusTooManyRequests {
		t.Errorf("T4.3 failed: expected 429, got %d", code)
	}
}

func TestPhase4_UnauthenticatedRefused(t *testing.T) {
	// T4.4: Unauthenticated request returns 401 without calling gateway
	gw := &mockGateway{}
	svc := assistant.NewService(nil, gw, slog.Default())
	handler := NewHandler(svc, gw, nil, slog.Default())

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("POST", "/api/v1/assistant/messages", strings.NewReader(`{"text":"hi"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("T4.4 failed: expected 401, got %d", rec.Code)
	}
	if gw.streamCalls != 0 {
		t.Errorf("T4.4 failed: expected 0 gateway calls, got %d", gw.streamCalls)
	}
}
