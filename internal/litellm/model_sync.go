package litellm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"
)

// ─── Model Management ──────────────────────────────────────────────────────
// LiteLLM /model/new and /model/delete APIs for dynamic model registration.
// When TaaS deploys a model to Dynamo, we register the Dynamo endpoint in LiteLLM.
// When TaaS stops a deployment, we remove it from LiteLLM.

// AddModelRequest is the payload for POST /model/new.
// Docs: https://docs.litellm.ai/docs/proxy/model_management
type AddModelRequest struct {
	ModelName    string         `json:"model_name"`     // User-facing model name (e.g., "llama-3-8b")
	LiteLLMParams ModelParams  `json:"litellm_params"`
	ModelInfo    *ModelInfo     `json:"model_info,omitempty"`
}

// ModelParams configures how LiteLLM routes to this model deployment.
type ModelParams struct {
	Model               string             `json:"model"`                          // Provider format: "openai/<model-name>"
	APIBase             string             `json:"api_base"`                       // Dynamo endpoint URL
	APIKey              string             `json:"api_key,omitempty"`              // Optional auth key
	RPM                 int                `json:"rpm,omitempty"`                  // Requests per minute
	TPM                 int                `json:"tpm,omitempty"`                  // Tokens per minute
	InputCostPerToken   float64            `json:"input_cost_per_token,omitempty"` // $/token
	OutputCostPerToken  float64            `json:"output_cost_per_token,omitempty"`
	ExtraHeaders        map[string]string  `json:"extra_headers,omitempty"`        // Injected into every request (tenant metadata)
}

// ModelInfo contains optional metadata about the model.
type ModelInfo struct {
	ID          string `json:"id,omitempty"`
	Description string `json:"description,omitempty"`
	MaxTokens   int    `json:"max_tokens,omitempty"`
}

// AddModelResponse is the response from POST /model/new.
type AddModelResponse struct {
	ModelID string `json:"model_id"`
}

// AddModel registers a new model deployment in LiteLLM Proxy.
// Called when a Dynamo deployment reaches "running" status.
func (c *AdminClient) AddModel(ctx context.Context, req AddModelRequest) (*AddModelResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/model/new", bytes.NewReader(body))
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("litellm /model/new returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result AddModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	c.logger.Info("litellm model registered",
		zap.String("model_name", req.ModelName),
		zap.String("api_base", req.LiteLLMParams.APIBase),
		zap.String("model_id", result.ModelID),
	)

	return &result, nil
}

// DeleteModelRequest is the payload for POST /model/delete.
type DeleteModelRequest struct {
	ID string `json:"id"` // Model ID returned by AddModel
}

// DeleteModel removes a model deployment from LiteLLM Proxy.
// Called when a Dynamo deployment is stopped or deleted.
func (c *AdminClient) DeleteModel(ctx context.Context, modelID string) error {
	body, err := json.Marshal(DeleteModelRequest{ID: modelID})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/model/delete", bytes.NewReader(body))
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
		return fmt.Errorf("litellm /model/delete returned %d: %s", resp.StatusCode, string(respBody))
	}

	c.logger.Info("litellm model removed", zap.String("model_id", modelID))
	return nil
}

// ListModelsResponse is the response from GET /model/info.
type ListModelsResponse struct {
	Data []ListModelEntry `json:"data"`
}

// ListModelEntry is a single model entry from /model/info.
type ListModelEntry struct {
	ModelID   string      `json:"model_id"`
	ModelName string      `json:"model_name"`
	ModelInfo interface{} `json:"model_info"`
}

// ListModels retrieves all registered models from LiteLLM Proxy.
func (c *AdminClient) ListModels(ctx context.Context) (*ListModelsResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/model/info", nil)
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
		return nil, fmt.Errorf("litellm /model/info returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result ListModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
