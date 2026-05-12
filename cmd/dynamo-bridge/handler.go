package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/billing"
	"github.com/taas-platform/taas/internal/dynamo"
	"github.com/taas-platform/taas/internal/monitoring"
	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// BridgeHandler receives OpenAI-compatible requests from LiteLLM and
// forwards them to NVIDIA Dynamo with tenant metadata injection.
type BridgeHandler struct {
	dynamo    *dynamo.Client
	publisher *billing.Publisher
	costCalc  *billing.CostCalculator
	metrics   *monitoring.Metrics
	logger    *zap.Logger
}

func NewBridgeHandler(dc *dynamo.Client, pub *billing.Publisher, costCalc *billing.CostCalculator, metrics *monitoring.Metrics, logger *zap.Logger) *BridgeHandler {
	return &BridgeHandler{dynamo: dc, publisher: pub, costCalc: costCalc, metrics: metrics, logger: logger}
}

// ChatCompletions handles POST /v1/chat/completions from LiteLLM.
func (h *BridgeHandler) ChatCompletions(c *gin.Context) {
	h.forwardToDynamo(c, "/v1/chat/completions")
}

// Completions handles POST /v1/completions from LiteLLM.
func (h *BridgeHandler) Completions(c *gin.Context) {
	h.forwardToDynamo(c, "/v1/completions")
}

// Embeddings handles POST /v1/embeddings from LiteLLM.
func (h *BridgeHandler) Embeddings(c *gin.Context) {
	h.forwardToDynamo(c, "/v1/embeddings")
}

// litellmMetadata represents metadata injected by LiteLLM in the request headers.
// LiteLLM forwards user/key metadata that we translate to Dynamo tenant headers.
type litellmMetadata struct {
	UserID    string `json:"user"`        // LiteLLM user_id (maps to our org_id)
	KeyAlias  string `json:"key_alias"`   // Virtual key alias (maps to token name)
	TeamID    string `json:"team_id"`     // LiteLLM team (maps to org_id)
	KeyID     string `json:"key_id"`      // LiteLLM key hash
	SLATier   string `json:"sla_tier"`    // Custom metadata passed through
}

func (h *BridgeHandler) forwardToDynamo(c *gin.Context, path string) {
	const maxBodySize = 10 << 20 // 10MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodySize)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("failed to read request body (max 10MB)"))
		return
	}

	// Extract model from request body
	var reqBody struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		h.logger.Warn("failed to parse request body for model extraction", zap.Error(err))
	}

	// Extract tenant metadata from LiteLLM headers
	meta := h.extractTenantMetadata(c)

	start := time.Now()

	// Check streaming
	var streamCheck struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &streamCheck)

	var buf bytes.Buffer
	var writer io.Writer = &buf

	if streamCheck.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)
		writer = c.Writer
		if flusher, ok := c.Writer.(http.Flusher); ok {
			defer flusher.Flush()
		}
	}

	result, err := h.dynamo.Forward(c.Request.Context(), path, body, meta, writer)
	latency := time.Since(start)

	if err != nil {
		h.logger.Error("dynamo forward error", zap.Error(err), zap.String("path", path))
		h.publishUsage(c, meta, reqBody.Model, 0, 0, latency, "error")
		h.recordMetrics(meta, reqBody.Model, latency, 0, 0, "error")
		middleware.ErrorResponse(c, taasErrors.New(taasErrors.CodeDynamoError, "inference backend error", http.StatusBadGateway).WithCause(err))
		return
	}

	// Publish usage event
	h.publishUsage(c, meta, reqBody.Model, result.PromptTokens, result.CompletionTokens, latency, "success")
	h.recordMetrics(meta, reqBody.Model, latency, result.PromptTokens, result.CompletionTokens, "success")

	// Non-streaming: write buffered response
	if !streamCheck.Stream {
		c.Header("Content-Type", "application/json")
		c.Status(result.HTTPStatus)
		c.Writer.Write(buf.Bytes()) //nolint:errcheck
	}
}

// extractTenantMetadata pulls tenant info from LiteLLM-injected headers.
// LiteLLM sends metadata in several ways:
//   - X-LiteLLM-User-Id: the user associated with the virtual key
//   - X-LiteLLM-Team-Id: the team associated with the virtual key
//   - X-LiteLLM-Key-Alias: the key alias/name
//   - litellm_metadata JSON header (custom fields)
func (h *BridgeHandler) extractTenantMetadata(c *gin.Context) dynamo.TenantMetadata {
	meta := dynamo.TenantMetadata{
		RequestID: c.GetHeader("X-Request-ID"),
	}

	// LiteLLM injects these headers when forwarding
	if userID := c.GetHeader("X-LiteLLM-User-Id"); userID != "" {
		meta.TenantID = userID
		meta.OrgID = userID
	}
	if teamID := c.GetHeader("X-LiteLLM-Team-Id"); teamID != "" {
		meta.OrgID = teamID
		if meta.TenantID == "" {
			meta.TenantID = teamID
		}
	}
	if keyAlias := c.GetHeader("X-LiteLLM-Key-Alias"); keyAlias != "" {
		meta.TokenID = keyAlias
	}

	// Parse custom metadata JSON if present
	if metaJSON := c.GetHeader("X-LiteLLM-Metadata"); metaJSON != "" {
		var lm litellmMetadata
		if err := json.Unmarshal([]byte(metaJSON), &lm); err == nil {
			if lm.SLATier != "" {
				meta.SLATier = lm.SLATier
			}
			if lm.KeyID != "" && meta.TokenID == "" {
				meta.TokenID = lm.KeyID
			}
		}
	}

	// Defaults
	if meta.SLATier == "" {
		meta.SLATier = "standard"
	}
	if meta.TenantID == "" {
		meta.TenantID = "default"
	}

	return meta
}

func (h *BridgeHandler) recordMetrics(meta dynamo.TenantMetadata, modelID string, latency time.Duration, promptTokens, completionTokens int, status string) {
	if h.metrics == nil {
		return
	}
	h.metrics.InferenceRequestsTotal.With(prometheus.Labels{
		"model_id": modelID, "org_id": meta.OrgID, "sla_tier": meta.SLATier, "status": status,
	}).Inc()
	h.metrics.InferenceLatency.With(prometheus.Labels{
		"model_id": modelID, "sla_tier": meta.SLATier,
	}).Observe(latency.Seconds())
	if promptTokens > 0 {
		h.metrics.InferenceTokensTotal.With(prometheus.Labels{
			"model_id": modelID, "org_id": meta.OrgID, "token_type": "prompt",
		}).Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		h.metrics.InferenceTokensTotal.With(prometheus.Labels{
			"model_id": modelID, "org_id": meta.OrgID, "token_type": "completion",
		}).Add(float64(completionTokens))
	}
}

func (h *BridgeHandler) publishUsage(c *gin.Context, meta dynamo.TenantMetadata, modelID string, promptTokens, completionTokens int, latency time.Duration, status string) {
	if h.publisher == nil {
		return
	}

	cost := h.costCalc.Calculate(modelID, promptTokens, completionTokens)

	event := billing.UsageEvent{
		RequestID:        meta.RequestID,
		TokenID:          meta.TokenID,
		OrgID:            meta.OrgID,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		LatencyMs:        int(latency.Milliseconds()),
		Status:           status,
		CostUSD:          cost,
		Timestamp:        time.Now().UTC(),
	}

	if err := h.publisher.Publish(c.Request.Context(), event); err != nil {
		h.logger.Warn("usage publish failed", zap.Error(err), zap.String("request_id", meta.RequestID))
		// Retry once
		time.Sleep(100 * time.Millisecond)
		if retryErr := h.publisher.Publish(c.Request.Context(), event); retryErr != nil {
			h.logger.Error("usage publish retry failed",
				zap.Error(retryErr),
				zap.String("request_id", meta.RequestID),
				zap.Int("total_tokens", event.TotalTokens),
			)
		}
	}
}
