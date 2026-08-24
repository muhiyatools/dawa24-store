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
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	CreditsConsumed float64 `json:"credits_consumed"`
	CostUSD         float64 `json:"cost_usd"`
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
