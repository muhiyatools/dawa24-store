package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

const maxAudioUploadBytes = 10 << 20 // 10 MiB client-side ceiling

// Transcribe sends an audio stream to the Gateway's transcription endpoint and returns the text.
func (c *HTTPClient) Transcribe(ctx context.Context, audio io.Reader, filename, mimeType string) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
	}
	if !c.breaker.allow() {
		return "", ErrCircuitOpen
	}

	settings := c.resolve(ctx)
	if !settings.Enabled || settings.VirtualKey == "" {
		return "", ErrDisabled
	}

	if filename == "" {
		filename = "audio.webm"
	}

	// Limit and buffer client-side to enforce size ceiling before network transfer
	audioBytes, err := io.ReadAll(io.LimitReader(audio, maxAudioUploadBytes+1))
	if err != nil {
		return "", fmt.Errorf("gateway: read audio buffer: %w", err)
	}
	if len(audioBytes) > maxAudioUploadBytes {
		return "", fmt.Errorf("%w: audio file exceeds 10MB limit", ErrBadRequest)
	}

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("gateway: create multipart file: %w", err)
	}
	if _, err := part.Write(audioBytes); err != nil {
		return "", fmt.Errorf("gateway: write audio part: %w", err)
	}

	modelName := resolveRoleModel(RoleTranscribe)
	if err := writer.WriteField("model", modelName); err != nil {
		return "", fmt.Errorf("gateway: write model field: %w", err)
	}
	if err := writer.WriteField("language", "ar"); err != nil {
		return "", fmt.Errorf("gateway: write language field: %w", err)
	}
	if err := writer.WriteField("response_format", "verbose_json"); err != nil {
		return "", fmt.Errorf("gateway: write response_format field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("gateway: close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		settings.BaseURL+"/v1/audio/transcriptions", &requestBody)
	if err != nil {
		return "", fmt.Errorf("gateway: build transcription request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+settings.VirtualKey)
	httpReq.Header.Set("X-Client-App", settings.ClientApp)
	if tp := traceparentFrom(ctx); tp != "" {
		httpReq.Header.Set("traceparent", tp)
	}

	res, err := c.http.Do(httpReq)
	if err != nil {
		c.breaker.failure()
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode != http.StatusOK {
		classified := classifyStatus(res.StatusCode, raw)
		if !isRetryable(classified) {
			c.breaker.success()
		} else {
			c.breaker.failure()
		}
		return "", classified
	}

	var parsed struct {
		Text  string `json:"text"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("gateway: decode transcription: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("gateway: transcription error: %s", parsed.Error.Message)
	}

	c.breaker.success()
	return parsed.Text, nil
}
