package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Stream opens a streaming completion to the Gateway. The returned channel
// yields stream events as SSE frames arrive. The caller must drain the channel
// or cancel ctx to abort upstream.
func (c *HTTPClient) Stream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if len(req.Tools) > 0 {
		return nil, ErrToolsNotSupported
	}
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	if !c.breaker.allow() {
		return nil, ErrCircuitOpen
	}

	settings := c.resolve(ctx)
	authKey := settings.VirtualKey
	if req.VirtualKey != "" {
		authKey = req.VirtualKey
	}
	if !settings.Enabled || authKey == "" {
		return nil, ErrDisabled
	}

	modelName := resolveRoleModel(req.Role)
	wireMsgs := buildWireMessages(req.Messages)

	payload := wireChatRequest{
		Model:       modelName,
		Messages:    wireMsgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gateway: marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		settings.BaseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("gateway: build stream request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+authKey)
	httpReq.Header.Set("X-Client-App", settings.ClientApp)
	httpReq.Header.Set("X-Dawa-Org-ID", strconv.FormatInt(req.OrgID, 10))
	httpReq.Header.Set("X-Dawa-User-ID", strconv.FormatInt(req.UserID, 10))
	httpReq.Header.Set("X-Dawa-Role", string(req.Role))
	if tp := traceparentFrom(ctx); tp != "" {
		httpReq.Header.Set("traceparent", tp)
	}

	res, err := c.http.Do(httpReq)
	if err != nil {
		c.breaker.failure()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		classified := classifyStatus(res.StatusCode, raw)
		if !isRetryable(classified) {
			c.breaker.success()
		} else {
			c.breaker.failure()
		}
		return nil, classified
	}

	events := make(chan StreamEvent, 64)
	go c.consumeSSE(ctx, res.Body, events)
	return events, nil
}

func (c *HTTPClient) consumeSSE(ctx context.Context, body io.ReadCloser, events chan<- StreamEvent) {
	defer body.Close()
	defer close(events)

	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			c.breaker.failure()
			select {
			case events <- StreamEvent{Err: fmt.Errorf("%w: %v", ErrUnavailable, err)}:
			case <-ctx.Done():
			}
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			c.breaker.success()
			select {
			case events <- StreamEvent{Done: true}:
			case <-ctx.Done():
			}
			return
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					Thinking         string `json:"thinking"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens            int `json:"prompt_tokens"`
				CompletionTokens        int `json:"completion_tokens"`
				TotalTokens             int `json:"total_tokens"`
				CompletionTokensDetails struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"completion_tokens_details"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Error != nil {
			c.breaker.failure()
			select {
			case events <- StreamEvent{Err: fmt.Errorf("gateway: %s", chunk.Error.Message)}:
			case <-ctx.Done():
			}
			return
		}

		var delta, reasoning string
		if len(chunk.Choices) > 0 {
			delta = chunk.Choices[0].Delta.Content
			reasoning = chunk.Choices[0].Delta.ReasoningContent
			if reasoning == "" {
				reasoning = chunk.Choices[0].Delta.Thinking
			}
		}

		var usage *Usage
		if chunk.Usage != nil {
			usage = &Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				ReasoningTokens:  chunk.Usage.CompletionTokensDetails.ReasoningTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}

		if delta != "" || reasoning != "" || usage != nil {
			select {
			case events <- StreamEvent{Delta: delta, Reasoning: reasoning, Usage: usage}:
			case <-ctx.Done():
				return
			}
		}
	}
}
