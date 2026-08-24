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

// GatewayVirtualKey represents an API Key generated for a Gateway user.
type GatewayVirtualKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// AdminClient interacts with the AI Gateway's REST Admin management API.
type AdminClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewAdminClient creates a client targeting the AI Gateway Admin API.
func NewAdminClient(baseURL, username, password string) *AdminClient {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
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

// ProvisionOrganization registers or synchronizes an organization with the AI Gateway,
// creating a dedicated user and issuing a virtual API key with the plan's quota.
func (c *AdminClient) ProvisionOrganization(ctx context.Context, orgID int64, name, email, planID string) (userID, virtualKey string, err error) {
	targetUserID := fmt.Sprintf("org-%d", orgID)
	if planID == "" {
		planID = "plan-basic"
	}
	if name == "" {
		name = fmt.Sprintf("Organization %d", orgID)
	}
	if email == "" {
		email = fmt.Sprintf("org%d@dawa24.local", orgID)
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
		if err := json.NewDecoder(keyResp.Body).Decode(&createdKey); err == nil && createdKey.Token != "" {
			return targetUserID, createdKey.Token, nil
		}
	}

	return targetUserID, "", nil
}

// UpdateOrganizationPlan updates the AI plan assigned to an organization.
func (c *AdminClient) UpdateOrganizationPlan(ctx context.Context, orgID int64, name, email, planID string) error {
	targetUserID := fmt.Sprintf("org-%d", orgID)
	if planID == "" {
		planID = "plan-basic"
	}
	if name == "" {
		name = fmt.Sprintf("Organization %d", orgID)
	}
	if email == "" {
		email = fmt.Sprintf("org%d@dawa24.local", orgID)
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
