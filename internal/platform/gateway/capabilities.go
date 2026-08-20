package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ModelCapabilities captures the live input modalities and attachment limits of a model.
type ModelCapabilities struct {
	Vision            bool     `json:"vision"`
	Thinking          bool     `json:"thinking"`
	Audio             bool     `json:"audio"`
	Video             bool     `json:"video"`
	Documents         bool     `json:"documents"`
	MaxAttachmentMB   int      `json:"max_attachment_mb"`
	AcceptedMIMETypes []string `json:"accepted_mime_types"`
}

type cachedCapability struct {
	caps      ModelCapabilities
	expiresAt time.Time
}

type capabilityCache struct {
	mu     sync.RWMutex
	cached map[Role]cachedCapability
	ttl    time.Duration
}

func newCapabilityCache(ttl time.Duration) *capabilityCache {
	return &capabilityCache{
		cached: make(map[Role]cachedCapability),
		ttl:    ttl,
	}
}

func (c *capabilityCache) get(role Role) (ModelCapabilities, bool) {
	if c == nil {
		return ModelCapabilities{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cached[role]
	if !ok || time.Now().After(entry.expiresAt) {
		return ModelCapabilities{}, false
	}
	return entry.caps, true
}

func (c *capabilityCache) set(role Role, caps ModelCapabilities) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached[role] = cachedCapability{
		caps:      caps,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// ConservativeDefaultCapabilities returns a safe text-only fallback (0 MB attachments).
func ConservativeDefaultCapabilities() ModelCapabilities {
	return ModelCapabilities{
		Vision:            false,
		Thinking:          false,
		Audio:             false,
		Video:             false,
		Documents:         false,
		MaxAttachmentMB:   0,
		AcceptedMIMETypes: nil,
	}
}

// Capabilities returns the live capability set for a role, cached.
// On fetch failure, returns conservative default without crashing.
func (c *HTTPClient) Capabilities(ctx context.Context, role Role) (ModelCapabilities, error) {
	if c.capCache == nil {
		c.capCache = newCapabilityCache(5 * time.Minute)
	}

	if cached, ok := c.capCache.get(role); ok {
		return cached, nil
	}

	if !c.Enabled() {
		return ConservativeDefaultCapabilities(), ErrDisabled
	}

	settings := c.resolve(ctx)
	if !settings.Enabled || settings.VirtualKey == "" {
		return ConservativeDefaultCapabilities(), ErrDisabled
	}

	modelName := resolveRoleModel(role)
	caps, err := c.fetchModelCapabilities(ctx, settings, modelName)
	if err != nil {
		c.log.WarnContext(ctx, "failed to fetch model capabilities, using conservative default",
			"role", role, "model", modelName, "error", err)
		return ConservativeDefaultCapabilities(), nil
	}

	c.capCache.set(role, caps)
	return caps, nil
}

func (c *HTTPClient) fetchModelCapabilities(ctx context.Context, settings Settings, modelName string) (ModelCapabilities, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, settings.BaseURL+"/v1/models", nil)
	if err != nil {
		return ConservativeDefaultCapabilities(), err
	}
	req.Header.Set("Authorization", "Bearer "+settings.VirtualKey)
	req.Header.Set("X-Client-App", settings.ClientApp)

	res, err := c.http.Do(req)
	if err != nil {
		return ConservativeDefaultCapabilities(), fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ConservativeDefaultCapabilities(), fmt.Errorf("%w: status %d", ErrUnavailable, res.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return ConservativeDefaultCapabilities(), err
	}

	var doc struct {
		Data []struct {
			ID           string            `json:"id"`
			Capabilities ModelCapabilities `json:"capabilities"`
		} `json:"data"`
		Models []struct {
			ModelID      string            `json:"model_id"`
			Capabilities ModelCapabilities `json:"capabilities"`
		} `json:"models"`
	}

	if err := json.Unmarshal(raw, &doc); err != nil {
		return ConservativeDefaultCapabilities(), err
	}

	for _, m := range doc.Data {
		if m.ID == modelName {
			return m.Capabilities, nil
		}
	}
	for _, m := range doc.Models {
		if m.ModelID == modelName {
			return m.Capabilities, nil
		}
	}

	// If model not found in catalogue, return conservative default
	return ConservativeDefaultCapabilities(), nil
}
