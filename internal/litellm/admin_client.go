package litellm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// AdminClient communicates with the LiteLLM Proxy Admin API to manage virtual keys.
// When TaaS creates/deletes/rotates API tokens, we sync them as LiteLLM virtual keys.
type AdminClient struct {
	baseURL    string
	masterKey  string
	httpClient *http.Client
	logger     *zap.Logger
}

func NewAdminClient(baseURL, masterKey string, logger *zap.Logger) *AdminClient {
	return &AdminClient{
		baseURL:   baseURL,
		masterKey: masterKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// GenerateKeyRequest is the payload for POST /key/generate.
type GenerateKeyRequest struct {
	KeyAlias     string            `json:"key_alias,omitempty"`     // Human-readable name
	UserID       string            `json:"user_id,omitempty"`       // Owner user
	TeamID       string            `json:"team_id,omitempty"`       // Organization/team
	Models       []string          `json:"models,omitempty"`        // Allowed models
	MaxBudget    *float64          `json:"max_budget,omitempty"`    // USD budget limit
	RPM          *int              `json:"rpm,omitempty"`           // Requests per minute
	TPM          *int              `json:"tpm,omitempty"`           // Tokens per minute
	Duration     string            `json:"duration,omitempty"`      // Key expiry (e.g., "30d")
	Metadata     map[string]string `json:"metadata,omitempty"`      // Custom metadata
}

// GenerateKeyResponse is the response from POST /key/generate.
type GenerateKeyResponse struct {
	Key       string  `json:"key"`        // The actual API key (sk-...)
	KeyName   string  `json:"key_name"`   // Internal name
	Token     string  `json:"token"`      // Key hash/ID in LiteLLM DB
	ExpiresAt *string `json:"expires"`
}

// GenerateKey creates a new virtual key in LiteLLM Proxy.
func (c *AdminClient) GenerateKey(ctx context.Context, req GenerateKeyRequest) (*GenerateKeyResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/key/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.masterKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("litellm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("litellm /key/generate returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result GenerateKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	c.logger.Info("litellm virtual key created",
		zap.String("key_alias", req.KeyAlias),
		zap.String("token", result.Token),
	)

	return &result, nil
}

// DeleteKeyRequest is the payload for POST /key/delete.
type DeleteKeyRequest struct {
	Keys []string `json:"keys"` // Key hashes/tokens to delete
}

// DeleteKey removes a virtual key from LiteLLM Proxy.
func (c *AdminClient) DeleteKey(ctx context.Context, keyToken string) error {
	body, err := json.Marshal(DeleteKeyRequest{Keys: []string{keyToken}})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/key/delete", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.masterKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("litellm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("litellm /key/delete returned %d: %s", resp.StatusCode, string(respBody))
	}

	c.logger.Info("litellm virtual key deleted", zap.String("key_token", keyToken))
	return nil
}

// InfoKeyResponse contains key details from LiteLLM.
type InfoKeyResponse struct {
	Token     string   `json:"token"`
	KeyAlias  string   `json:"key_alias"`
	KeyName   string   `json:"key_name"`
	UserID    string   `json:"user_id"`
	TeamID    string   `json:"team_id"`
	Models    []string `json:"models"`
	MaxBudget *float64 `json:"max_budget"`
	RPM       *int     `json:"rpm"`
	TPM       *int     `json:"tpm"`
	Spend     float64  `json:"spend"`
}

// GetKeyInfo retrieves information about a virtual key.
func (c *AdminClient) GetKeyInfo(ctx context.Context, keyToken string) (*InfoKeyResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/key/info?key="+keyToken, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.masterKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("litellm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("litellm /key/info returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result InfoKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
