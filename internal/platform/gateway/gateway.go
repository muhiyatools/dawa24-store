// Package gateway is the Store's only connection to AI functionality.
//
// Architectural invariant, enforced by a CI grep (see Makefile target
// "check-provider-isolation"): the words openai, anthropic, deepseek, gemini,
// groq and openrouter must not appear anywhere in this repository outside this
// package's documentation. The Store does not know which provider answers a
// request, does not hold provider credentials, and does not choose models. It
// asks for a capability; the MuhiyaLLM Gateway decides everything else.
//
// That boundary is what makes provider changes a Gateway admin action rather
// than a Store deployment.
//
// Every capability has a deterministic fallback (see fallback.go). A pharmacy
// must be able to place an order and a supplier must be able to import a
// catalogue when the Gateway is unreachable. AI is an enhancement here, never a
// dependency of commerce.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// Capability names a business task, not a model. The mapping from capability to
// model alias lives in Gateway configuration.
type Capability string

const (
	CapProductMatch  Capability = "product.match"
	CapColumnDetect  Capability = "import.detect_columns"
	CapProductEnrich Capability = "product.enrich"
	CapOrderOptimize Capability = "order.optimize"
	CapCatalogChat   Capability = "catalog.chat"
	CapSearchExpand  Capability = "search.expand_query"
)

// budget is the per-capability latency ceiling and retry policy.
//
// These are far below the Gateway's own 30-minute upstream timeout, which is
// tuned for long agentic coding turns. A vendor waiting on a column-detection
// call needs an answer or a fallback in seconds.
type budget struct {
	timeout time.Duration
	retries int
	// alias is the Store-side configuration key naming a Gateway model alias.
	// It never contains a provider or model name.
	alias string
}

var budgets = map[Capability]budget{
	CapColumnDetect:  {timeout: 30 * time.Second, retries: 2, alias: "dawa24-fast"},
	CapProductMatch:  {timeout: 60 * time.Second, retries: 3, alias: "dawa24-fast"},
	CapProductEnrich: {timeout: 120 * time.Second, retries: 1, alias: "dawa24-quality"},
	CapOrderOptimize: {timeout: 15 * time.Second, retries: 1, alias: "dawa24-fast"},
	CapCatalogChat:   {timeout: 120 * time.Second, retries: 0, alias: "dawa24-quality"},
	CapSearchExpand:  {timeout: 3 * time.Second, retries: 0, alias: "dawa24-fast"},
}

// Request is a capability invocation.
type Request struct {
	Capability     Capability
	System         string         // prompt template, versioned in this repo
	Input          string         // rendered user content
	Schema         map[string]any // optional JSON schema for structured output
	OrganizationID int64          // cost attribution and per-tenant quota
	UserID         int64
	VirtualKey     string // optional tenant virtual key for per-tenant quotas
	IdempotencyKey string // required for anything that mutates domain state
	MaxTokens      int
}

// Response is a capability result.
type Response struct {
	Content     string
	Model       string // Gateway's resolved model id, for logging only
	RequestID   string // Gateway request_logs.id, stored alongside the domain row
	InputTok    int
	OutputTok   int
	CostNanoUSD int64
	FromCache   bool
	Fallback    bool // true when this came from the deterministic path
}

// Client is the Store-facing interface. Modules depend on this, never on the
// concrete implementation, so tests inject a stub or a black-hole.
type Client interface {
	Invoke(ctx context.Context, req Request) (*Response, error)
	Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
	Transcribe(ctx context.Context, audio io.Reader, filename, mime string) (string, error)
	Capabilities(ctx context.Context, role Role) (ModelCapabilities, error)
	Health(ctx context.Context) error
	Enabled() bool
}

// HTTPClient talks to the Gateway over its OpenAI-compatible surface.
type HTTPClient struct {
	cfg        config.Gateway
	http       *http.Client
	log        *slog.Logger
	breaker    *breaker
	source     SettingsSource
	cache      *settingsCache
	capCache   *capabilityCache
	sourceMu   sync.Mutex
	lastSource string
}

func New(cfg config.Gateway, log *slog.Logger) *HTTPClient {
	return &HTTPClient{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		log:      log.With("component", "gateway"),
		breaker:  newBreaker(5, 30*time.Second, 60*time.Second),
		cache:    newSettingsCache(30 * time.Second),
		capCache: newCapabilityCache(5 * time.Minute),
	}
}

func (c *HTTPClient) Enabled() bool {
	s := c.resolve(context.Background())
	return s.Enabled
}

// Invoke calls the Gateway, honouring the capability's timeout and retry budget.
//
// It returns ErrUnavailable rather than a transport error so that callers have
// exactly one condition to branch on when deciding to use their fallback.
func (c *HTTPClient) Invoke(ctx context.Context, req Request) (*Response, error) {
	settings := c.resolve(ctx)
	authKey := settings.VirtualKey
	if req.VirtualKey != "" {
		authKey = req.VirtualKey
	}
	if !settings.Enabled || authKey == "" {
		return nil, ErrDisabled
	}
	b, ok := budgets[req.Capability]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCapability, req.Capability)
	}
	if !c.breaker.allow() {
		return nil, ErrCircuitOpen
	}

	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= b.retries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter, bounded by the remaining budget.
			delay := backoff(attempt)
			select {
			case <-ctx.Done():
				c.breaker.failure()
				return nil, ErrTimeout
			case <-time.After(delay):
			}
		}

		resp, err := c.do(ctx, req, b)
		if err == nil {
			c.breaker.success()
			return resp, nil
		}
		lastErr = err

		// Only transport and 5xx failures are worth retrying. A 400 will be a
		// 400 again, and a 402 means the budget is genuinely spent.
		if !isRetryable(err) {
			c.breaker.success() // the Gateway answered; it is not unhealthy
			return nil, err
		}
	}

	c.breaker.failure()
	c.log.WarnContext(ctx, "gateway call exhausted retries",
		"capability", req.Capability, "org_id", req.OrganizationID, "error", lastErr)
	return nil, fmt.Errorf("%w: %v", ErrUnavailable, lastErr)
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat any           `json:"response_format,omitempty"`
	Stream         bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *HTTPClient) do(ctx context.Context, req Request, b budget) (*Response, error) {
	payload := chatRequest{
		Model: b.alias,
		Messages: []chatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.Input},
		},
		MaxTokens: req.MaxTokens,
		Stream:    false,
	}
	if req.Schema != nil {
		payload.ResponseFormat = map[string]any{
			"type":        "json_schema",
			"json_schema": req.Schema,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gateway: marshal request: %w", err)
	}

	settings := c.resolve(ctx)
	authKey := settings.VirtualKey
	if req.VirtualKey != "" {
		authKey = req.VirtualKey
	}
	if !settings.Enabled || authKey == "" {
		return nil, ErrDisabled
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		settings.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gateway: build request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+authKey)
	httpReq.Header.Set("X-Client-App", settings.ClientApp)
	// Per-tenant attribution: lets the Gateway report and cap AI spend by
	// organisation without the Store having to meter tokens itself.
	httpReq.Header.Set("X-Dawa-Org-ID", strconv.FormatInt(req.OrganizationID, 10))
	httpReq.Header.Set("X-Dawa-User-ID", strconv.FormatInt(req.UserID, 10))
	httpReq.Header.Set("X-Dawa-Capability", string(req.Capability))
	if req.IdempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)
	}
	if tp := traceparentFrom(ctx); tp != "" {
		httpReq.Header.Set("traceparent", tp)
	}

	res, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, classifyStatus(res.StatusCode, raw)
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("gateway: decode response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("gateway: upstream refused: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("gateway: response contained no choices")
	}

	return &Response{
		Content:   parsed.Choices[0].Message.Content,
		Model:     parsed.Model,
		RequestID: parsed.ID,
		InputTok:  parsed.Usage.PromptTokens,
		OutputTok: parsed.Usage.CompletionTokens,
	}, nil
}

// Health probes the Gateway for the readiness endpoint. A failing Gateway does
// not make the Store unhealthy — commerce continues on fallbacks — so callers
// report this as a degraded subsystem, not a failed one.
func (c *HTTPClient) Health(ctx context.Context) error {
	settings := c.resolve(ctx)
	if !settings.Enabled || settings.VirtualKey == "" {
		return ErrDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, settings.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+settings.VirtualKey)
	if settings.ClientApp != "" {
		req.Header.Set("X-Client-App", settings.ClientApp)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: health returned %d", ErrUnavailable, res.StatusCode)
	}
	return nil
}

// breaker is a three-state circuit breaker: closed, open, half-open.
//
// Without it, a Gateway outage turns every import row into a 60-second timeout,
// and the import queue backs up until workers are saturated. Opening the circuit
// converts that into an immediate fallback.
type breaker struct {
	mu           sync.Mutex
	failures     int
	threshold    int
	openUntil    time.Time
	openDuration time.Duration
	window       time.Duration
	firstFailure time.Time
}

func newBreaker(threshold int, window, openDuration time.Duration) *breaker {
	return &breaker{threshold: threshold, window: window, openDuration: openDuration}
}

func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Now().Before(b.openUntil) {
		return false
	}
	return true
}

func (b *breaker) success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.firstFailure = time.Time{}
}

func (b *breaker) failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if b.firstFailure.IsZero() || now.Sub(b.firstFailure) > b.window {
		b.firstFailure = now
		b.failures = 0
	}
	b.failures++
	if b.failures >= b.threshold {
		b.openUntil = now.Add(b.openDuration)
		b.failures = 0
		b.firstFailure = time.Time{}
	}
}

func backoff(attempt int) time.Duration {
	base := time.Duration(1<<uint(attempt-1)) * 250 * time.Millisecond
	if base > 4*time.Second {
		base = 4 * time.Second
	}
	// Full jitter: spreads a thundering herd of import workers retrying together.
	return time.Duration(float64(base) * (0.5 + 0.5*pseudoRand()))
}

var (
	randMu    sync.Mutex
	randState uint64 = 0x2545F4914F6CDD1D
)

// pseudoRand is a tiny xorshift generator. Jitter does not need crypto entropy,
// and this avoids pulling math/rand's global lock into the hot retry path.
func pseudoRand() float64 {
	randMu.Lock()
	defer randMu.Unlock()
	randState ^= randState << 13
	randState ^= randState >> 7
	randState ^= randState << 17
	return float64(randState>>11) / float64(1<<53)
}

type traceKey struct{}

// WithTraceparent stores a W3C traceparent for propagation to the Gateway, so a
// slow AI call can be followed across both services in one trace.
func WithTraceparent(ctx context.Context, tp string) context.Context {
	return context.WithValue(ctx, traceKey{}, tp)
}

func traceparentFrom(ctx context.Context) string {
	tp, _ := ctx.Value(traceKey{}).(string)
	return tp
}
