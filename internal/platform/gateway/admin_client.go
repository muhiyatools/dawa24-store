package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GatewayPlan represents an AI plan in the AI Gateway with its token/request limits.
type GatewayPlan struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RPMLimit    int    `json:"rpm_limit"`
	TPMLimit    int    `json:"tpm_limit"`
	Description string `json:"description,omitempty"`
}

// GatewayUser represents a user or organization account in the AI Gateway.
type GatewayUser struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	PlanID    string    `json:"plan_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// GatewayBudgetUsage tracks windowed consumption in the Gateway.
type GatewayBudgetUsage struct {
	WindowID        string    `json:"window_id"`
	Name            string    `json:"name"`
	DurationSeconds int       `json:"duration_seconds"`
	BudgetUSD       float64   `json:"budget_usd"`
	CurrentSpent    float64   `json:"current_spent"`
	ResetTime       time.Time `json:"reset_time"`
}

// GatewayUserDetail provides full user information including budget windows and extra credits.
type GatewayUserDetail struct {
	ID                    string               `json:"id"`
	Name                  string               `json:"name"`
	Email                 string               `json:"email"`
	PlanID                string               `json:"plan_id"`
	Status                string               `json:"status"`
	BudgetUsage           []GatewayBudgetUsage `json:"budget_usage"`
	RemainingExtraCredits float64              `json:"remaining_extra_credits"`
}

// GatewayUserSummary aggregates total lifetime requests, tokens, and cost.
type GatewayUserSummary struct {
	Requests        int     `json:"requests"`
	Successful      int     `json:"successful"`
	Failed          int     `json:"failed"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
	CostUSD         float64 `json:"cost_usd"`
	CostNanoUSD     int64   `json:"cost_nano_usd"`
	CreditsConsumed float64 `json:"credits_consumed"`
}

// GatewayVirtualKey represents an API Key generated for a Gateway user.
type GatewayVirtualKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token,omitempty"`
	Key       string    `json:"key,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Secret returns the plaintext token from either Key or Token field.
func (k GatewayVirtualKey) Secret() string {
	if k.Key != "" {
		return k.Key
	}
	return k.Token
}

// AdminClient interacts with the AI Gateway's REST Admin management API.
type AdminClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewAdminClient creates a client targeting the AI Gateway Admin API.
// username and password can be explicit or passed as single API key / basic credential in password.
func NewAdminClient(baseURL, username, password string) *AdminClient {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.muhiya.com"
	}
	if username == "" && password != "" {
		if strings.Contains(password, ":") {
			parts := strings.SplitN(password, ":", 2)
			username = parts[0]
			password = parts[1]
		} else {
			username = "admin"
		}
	}
	if username == "" {
		username = "admin"
	}
	return &AdminClient{
		baseURL:    baseURL,
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ListPlans queries all active plans configured in the AI Gateway.
func (c *AdminClient) ListPlans(ctx context.Context) ([]GatewayPlan, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/plans", nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
	}
	var plans []GatewayPlan
	if err := json.NewDecoder(resp.Body).Decode(&plans); err != nil {
		return nil, err
	}
	return plans, nil
}

// GetUser queries a specific user's details and active budget allocations.
func (c *AdminClient) GetUser(ctx context.Context, userID string) (*GatewayUserDetail, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("empty user ID")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/users?id=%s", c.baseURL, userID), nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
	}
	var u GatewayUserDetail
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserUsageSummary fetches total consumption counters for a user.
func (c *AdminClient) GetUserUsageSummary(ctx context.Context, userID string) (*GatewayUserSummary, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("empty user ID")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/logs/summary?user_id=%s", c.baseURL, userID), nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
	}
	var s GatewayUserSummary
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GatewayLogEntry represents one logged AI request from the Gateway.
type GatewayLogEntry struct {
	ID               string    `json:"id"`
	VirtualKeyID     string    `json:"virtual_key_id"`
	UserID           string    `json:"user_id"`
	OwnerName        string    `json:"owner_name,omitempty"`
	ModelID          string    `json:"model_id"`
	RequestedModel   string    `json:"requested_model"`
	Model            string    `json:"model,omitempty"`
	ProviderID       string    `json:"provider_id,omitempty"`
	RequestPath      string    `json:"request_path"`
	StatusCode       int       `json:"status_code"`
	RequestStatus    string    `json:"request_status"`
	Status           string    `json:"status,omitempty"`
	Streamed         bool      `json:"streamed"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	CacheReadTokens  int       `json:"cache_read_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	ReasoningTokens  *int      `json:"reasoning_tokens,omitempty"`
	Cost             float64   `json:"cost"`
	CostUSD          float64   `json:"cost_usd,omitempty"`
	CostNanoUSD      int64     `json:"cost_nano_usd"`
	CreditsConsumed  float64   `json:"credits_consumed"`
	LatencyMS        int       `json:"latency_ms"`
	DurationMs       int64     `json:"duration_ms,omitempty"`
	ClientApp        string    `json:"client_app"`
	Capability       string    `json:"capability,omitempty"`
	Feature          string    `json:"feature,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func (e GatewayLogEntry) ResolvedModel() string {
	if e.ModelID != "" {
		return e.ModelID
	}
	if e.RequestedModel != "" {
		return e.RequestedModel
	}
	if e.Model != "" {
		return e.Model
	}
	return "qwen3.7-flash"
}

func (e GatewayLogEntry) ResolvedStatus() string {
	if e.RequestStatus != "" {
		return e.RequestStatus
	}
	if e.Status != "" {
		return e.Status
	}
	if e.StatusCode >= 200 && e.StatusCode < 300 {
		return "success"
	}
	return "failed"
}

func (e GatewayLogEntry) TotalTokens() int {
	return e.InputTokens + e.OutputTokens
}

func (e GatewayLogEntry) ResolvedCost() float64 {
	if e.Cost > 0 {
		return e.Cost
	}
	if e.CostUSD > 0 {
		return e.CostUSD
	}
	if e.CostNanoUSD > 0 {
		return float64(e.CostNanoUSD) / 1e9
	}
	return 0
}

func (e GatewayLogEntry) ResolvedLatency() int64 {
	if e.LatencyMS > 0 {
		return int64(e.LatencyMS)
	}
	return e.DurationMs
}

// GetUserLogs fetches request log records for a user from the AI Gateway.
func (c *AdminClient) GetUserLogs(ctx context.Context, userID string, limit, offset int) ([]GatewayLogEntry, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("empty user ID")
	}
	if limit <= 0 {
		limit = 50
	}
	reqURL := fmt.Sprintf("%s/api/logs?user_id=%s&limit=%d&offset=%d", c.baseURL, userID, limit, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, string(body))
	}
	var logs []GatewayLogEntry
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// ProvisionOrganization registers or synchronizes an organization with the AI Gateway,
// creating a dedicated user and issuing a virtual API key with the plan's quota.
func (c *AdminClient) ProvisionOrganization(ctx context.Context, orgID int64, name, email, planID string) (userID, virtualKey string, err error) {
	targetUserID := fmt.Sprintf("org-%d", orgID)
	if planID == "" {
		planID = "plan-pos-free"
	}
	if name == "" {
		name = fmt.Sprintf("Organization %d", orgID)
	}
	if email == "" {
		email = fmt.Sprintf("org-%d@dawa24.app", orgID)
	}

	// 1. Create or update user in AI Gateway
	u := GatewayUser{
		ID:     targetUserID,
		Name:   name,
		Email:  email,
		PlanID: planID,
		Status: "active",
	}
	body, _ := json.Marshal(u)
	postReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/users", bytes.NewReader(body))
	c.setAuth(postReq)
	postReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(postReq)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			// If already exists or conflicts, attempt a PUT update
			putReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/api/users", bytes.NewReader(body))
			c.setAuth(putReq)
			putReq.Header.Set("Content-Type", "application/json")
			if putResp, putErr := c.httpClient.Do(putReq); putErr == nil {
				_ = putResp.Body.Close()
			}
		}
	}

	// 2. Generate a Virtual Key for this organization
	keyReqBody, _ := json.Marshal(map[string]string{
		"name":    fmt.Sprintf("Dawa24-Org-%d-Key", orgID),
		"user_id": targetUserID,
	})
	keyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/keys", bytes.NewReader(keyReqBody))
	c.setAuth(keyReq)
	keyReq.Header.Set("Content-Type", "application/json")
	keyResp, err := c.httpClient.Do(keyReq)
	if err != nil {
		return targetUserID, "", err
	}
	defer keyResp.Body.Close()

	if keyResp.StatusCode == http.StatusCreated || keyResp.StatusCode == http.StatusOK {
		var createdKey GatewayVirtualKey
		if err := json.NewDecoder(keyResp.Body).Decode(&createdKey); err == nil && createdKey.Secret() != "" {
			return targetUserID, createdKey.Secret(), nil
		}
	}

	return targetUserID, "", nil
}

// UpdateOrganizationPlan updates the AI plan assigned to an organization.
func (c *AdminClient) UpdateOrganizationPlan(ctx context.Context, orgID int64, name, email, planID string) error {
	targetUserID := fmt.Sprintf("org-%d", orgID)
	if planID == "" {
		planID = "plan-pos-free"
	}
	if name == "" {
		name = fmt.Sprintf("Organization %d", orgID)
	}
	if email == "" {
		email = fmt.Sprintf("org-%d@dawa24.app", orgID)
	}
	u := GatewayUser{
		ID:     targetUserID,
		Name:   name,
		Email:  email,
		PlanID: planID,
		Status: "active",
	}
	body, _ := json.Marshal(u)
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/api/users", bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setAuth(putReq)
	putReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(putReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *AdminClient) setAuth(req *http.Request) {
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
}

// AdminPanelUserID is the Gateway identity the admin panel itself uses.
//
// The platform's own AI work — catalogue enrichment, the admin assistant — is
// not done on behalf of any tenant, so it needs an identity of its own rather
// than borrowing a pharmacy's key and spending that pharmacy's budget.
const AdminPanelUserID = "dawa24-admin-panel"

// ProvisionAdminPanel creates (or refreshes) the admin panel's Gateway user and
// issues it a virtual key.
//
// It exists because the admin credentials configured in إعدادات النظام are
// basic-auth for the /api management surface, not a Bearer token for /v1. Using
// them to call a completion is a 401 every time, which is exactly why the AI
// features appeared to do nothing. This turns the credentials an operator does
// have into the key the runtime actually needs.
func (c *AdminClient) ProvisionAdminPanel(ctx context.Context, planID string) (userID, virtualKey string, err error) {
	if planID == "" {
		planID = "plan-pos-free"
	}

	user := GatewayUser{
		ID:     AdminPanelUserID,
		Name:   "Dawa24 Admin Panel",
		Email:  "admin-panel@dawa24.app",
		PlanID: planID,
		Status: "active",
	}
	if err := c.upsertUser(ctx, user); err != nil {
		return AdminPanelUserID, "", err
	}

	key, err := c.issueKey(ctx, AdminPanelUserID, "Dawa24-Admin-Panel-Key")
	if err != nil {
		return AdminPanelUserID, "", err
	}
	return AdminPanelUserID, key, nil
}

// upsertUser creates the user, falling back to an update when it already exists.
func (c *AdminClient) upsertUser(ctx context.Context, user GatewayUser) error {
	body, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("gateway admin: encode user: %w", err)
	}

	status, raw, err := c.send(ctx, http.MethodPost, "/api/users", body)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusCreated {
		return nil
	}

	// Already registered: bring its plan and status back into line.
	status, raw, err = c.send(ctx, http.MethodPut, "/api/users", body)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("gateway admin: register %q failed (%d): %s", user.ID, status, truncateBody(raw))
	}
	return nil
}

// issueKey mints a virtual key for a Gateway user.
func (c *AdminClient) issueKey(ctx context.Context, userID, name string) (string, error) {
	body, err := json.Marshal(map[string]string{"name": name, "user_id": userID})
	if err != nil {
		return "", fmt.Errorf("gateway admin: encode key request: %w", err)
	}

	status, raw, err := c.send(ctx, http.MethodPost, "/api/keys", body)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", fmt.Errorf("gateway admin: issue key for %q failed (%d): %s", userID, status, truncateBody(raw))
	}

	var created GatewayVirtualKey
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", fmt.Errorf("gateway admin: decode issued key: %w", err)
	}
	if created.Secret() == "" {
		return "", fmt.Errorf("gateway admin: gateway issued an empty key for %q", userID)
	}
	return created.Secret(), nil
}

// send performs one authenticated management call and returns its raw result.
func (c *AdminClient) send(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("gateway admin: build %s %s: %w", method, path, err)
	}
	c.setAuth(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("gateway admin: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("gateway admin: read %s %s: %w", method, path, err)
	}
	return resp.StatusCode, raw, nil
}

// Ping checks that the configured admin credentials actually work, so the
// settings screen can tell an operator their credentials are wrong instead of
// leaving every AI feature quietly disabled.
func (c *AdminClient) Ping(ctx context.Context) error {
	status, raw, err := c.send(ctx, http.MethodGet, "/api/plans", nil)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("gateway admin: credentials rejected (%d)", status)
	}
	if status != http.StatusOK {
		return fmt.Errorf("gateway admin: unexpected status %d: %s", status, truncateBody(raw))
	}
	return nil
}

// truncateBody keeps an upstream error readable in a log line.
func truncateBody(raw []byte) string {
	const limit = 300
	text := strings.TrimSpace(string(raw))
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}

// ValidateVirtualKey reports whether a Bearer key the Gateway issued still
// works.
//
// A key can stop working without anything local changing: issuing a new one for
// the same user revokes the previous, so a second app instance booting — or a
// re-run of provisioning — silently invalidates the key the first one stored.
// Checking costs one credential-free request against the model list, which is
// far cheaper than discovering the problem as a failed import.
func (c *AdminClient) ValidateVirtualKey(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("gateway admin: empty virtual key")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("gateway admin: build key check: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gateway admin: check key: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("gateway admin: virtual key rejected (%d)", resp.StatusCode)
	}
	return nil
}
