package dynamo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client proxies requests to the NVIDIA Dynamo frontend HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ChatCompletionRequest mirrors the OpenAI chat completions API.
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TenantMetadata is injected by TaaS before forwarding to Dynamo.
type TenantMetadata struct {
	TenantID     string
	OrgID        string
	TokenID      string
	SLATier      string
	RequestID    string
	DeploymentID string
}

// Forward proxies a request body to Dynamo and streams back the response.
// It injects tenant headers required by Dynamo for priority routing.
func (c *Client) Forward(ctx context.Context, path string, body []byte, meta TenantMetadata, w io.Writer) (*ForwardResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", meta.TenantID)
	req.Header.Set("X-Org-ID", meta.OrgID)
	req.Header.Set("X-Token-ID", meta.TokenID)
	req.Header.Set("X-SLA-Tier", meta.SLATier)
	req.Header.Set("X-Request-ID", meta.RequestID)
	req.Header.Set("X-Deployment-ID", meta.DeploymentID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dynamo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dynamo error %d: %s", resp.StatusCode, string(body))
	}

	result := &ForwardResult{HTTPStatus: resp.StatusCode}

	// Stream or buffer response
	if resp.Header.Get("Content-Type") == "text/event-stream" {
		result.Streamed = true
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if _, wErr := w.Write(append(line, '\n')); wErr != nil {
				return result, wErr
			}
			// Parse usage from final [DONE] chunk if present
		}
		return result, scanner.Err()
	}

	// Non-streaming: parse response for usage metrics
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading dynamo response: %w", err)
	}
	if _, wErr := w.Write(respBody); wErr != nil {
		return result, wErr
	}

	var parsed struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err == nil {
		result.PromptTokens = parsed.Usage.PromptTokens
		result.CompletionTokens = parsed.Usage.CompletionTokens
		result.TotalTokens = parsed.Usage.TotalTokens
	}

	return result, nil
}

// ForwardResult contains post-request metadata extracted from the Dynamo response.
type ForwardResult struct {
	HTTPStatus       int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Streamed         bool
	LatencyMs        int64
}
