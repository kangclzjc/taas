package proxy

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
	"github.com/taas-platform/taas/internal/token"
	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// Handler proxies OpenAI-compatible requests to NVIDIA Dynamo.
type Handler struct {
	dynamo    *dynamo.Client
	publisher *billing.Publisher
	costCalc  *billing.CostCalculator
	metrics   *monitoring.Metrics
	logger    *zap.Logger
}

func NewHandler(dc *dynamo.Client, pub *billing.Publisher, costCalc *billing.CostCalculator, metrics *monitoring.Metrics, logger *zap.Logger) *Handler {
	return &Handler{dynamo: dc, publisher: pub, costCalc: costCalc, metrics: metrics, logger: logger}
}

// ChatCompletions handles POST /v1/chat/completions.
func (h *Handler) ChatCompletions(c *gin.Context) {
	h.forwardToDynamo(c, "/v1/chat/completions")
}

// Completions handles POST /v1/completions.
func (h *Handler) Completions(c *gin.Context) {
	h.forwardToDynamo(c, "/v1/completions")
}

// Embeddings handles POST /v1/embeddings.
func (h *Handler) Embeddings(c *gin.Context) {
	h.forwardToDynamo(c, "/v1/embeddings")
}

func (h *Handler) forwardToDynamo(c *gin.Context, path string) {
	// Limit request body to 10MB to prevent memory exhaustion
	const maxBodySize = 10 << 20 // 10MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodySize)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("failed to read request body (max 10MB)"))
		return
	}

	tokenInfo := getTokenInfo(c)
	if tokenInfo == nil {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("missing token context"))
		return
	}

	// Extract model ID from request body for cost calculation and metrics
	var reqBody struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		h.logger.Warn("failed to parse request body for model extraction", zap.Error(err))
		// Continue with empty model — best effort for metrics
	}

	requestID, _ := c.Get("request_id")
	rid, _ := requestID.(string)

	meta := dynamo.TenantMetadata{
		TenantID:  tokenInfo.OrgID,
		OrgID:     tokenInfo.OrgID,
		TokenID:   tokenInfo.TokenID,
		SLATier:   tokenInfo.SLATier,
		RequestID: rid,
	}

	start := time.Now()

	// For streaming, write directly to the client; for non-streaming, buffer
	var buf bytes.Buffer
	var writer io.Writer = &buf

	// Check if request asks for streaming
	var streamCheck struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &streamCheck); err != nil {
		h.logger.Warn("failed to parse stream flag from request body", zap.Error(err))
	}

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
		h.publishUsage(c, tokenInfo, rid, reqBody.Model, 0, 0, latency, "error")
		h.recordMetrics(tokenInfo, reqBody.Model, latency, 0, 0, "error")
		middleware.ErrorResponse(c, taasErrors.New(taasErrors.CodeDynamoError, "inference backend error", http.StatusBadGateway).WithCause(err))
		return
	}

	// Publish usage event asynchronously
	h.publishUsage(c, tokenInfo, rid, reqBody.Model, result.PromptTokens, result.CompletionTokens, latency, "success")
	h.recordMetrics(tokenInfo, reqBody.Model, latency, result.PromptTokens, result.CompletionTokens, "success")

	// For non-streaming responses, write the buffered body
	if !streamCheck.Stream {
		c.Header("Content-Type", "application/json")
		c.Status(result.HTTPStatus)
		c.Writer.Write(buf.Bytes()) //nolint:errcheck
	}
}

// recordMetrics updates Prometheus counters/histograms for inference requests.
func (h *Handler) recordMetrics(info *token.CachedTokenInfo, modelID string, latency time.Duration, promptTokens, completionTokens int, status string) {
	if h.metrics == nil || info == nil {
		return
	}
	h.metrics.InferenceRequestsTotal.With(prometheus.Labels{
		"model_id": modelID, "org_id": info.OrgID, "sla_tier": info.SLATier, "status": status,
	}).Inc()
	h.metrics.InferenceLatency.With(prometheus.Labels{
		"model_id": modelID, "sla_tier": info.SLATier,
	}).Observe(latency.Seconds())
	if promptTokens > 0 {
		h.metrics.InferenceTokensTotal.With(prometheus.Labels{
			"model_id": modelID, "org_id": info.OrgID, "token_type": "prompt",
		}).Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		h.metrics.InferenceTokensTotal.With(prometheus.Labels{
			"model_id": modelID, "org_id": info.OrgID, "token_type": "completion",
		}).Add(float64(completionTokens))
	}
}

func (h *Handler) publishUsage(c *gin.Context, info *token.CachedTokenInfo, requestID, path string, promptTokens, completionTokens int, latency time.Duration, status string) {
	if h.publisher == nil {
		return
	}

	cost := h.costCalc.Calculate(path, promptTokens, completionTokens)

	event := billing.UsageEvent{
		RequestID:        requestID,
		TokenID:          info.TokenID,
		UserID:           info.UserID,
		OrgID:            info.OrgID,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		LatencyMs:        int(latency.Milliseconds()),
		Status:           status,
		CostUSD:          cost,
		Timestamp:        time.Now().UTC(),
	}

	if err := h.publisher.Publish(c.Request.Context(), event); err != nil {
		h.logger.Warn("usage publish failed, retrying once", zap.Error(err))
		// Retry once after a short delay (P2: billing retry)
		time.Sleep(100 * time.Millisecond)
		if retryErr := h.publisher.Publish(c.Request.Context(), event); retryErr != nil {
			h.logger.Error("usage publish retry failed, usage event lost",
				zap.Error(retryErr),
				zap.String("request_id", event.RequestID),
				zap.String("token_id", event.TokenID),
				zap.Int("total_tokens", event.TotalTokens),
			)
		}
	}
}

// getTokenInfo extracts the validated token info set by the API key middleware.
func getTokenInfo(c *gin.Context) *token.CachedTokenInfo {
	val, exists := c.Get("token_info")
	if !exists {
		return nil
	}
	info, ok := val.(*token.CachedTokenInfo)
	if !ok {
		return nil
	}
	return info
}

// RegisterRoutes sets up inference proxy routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/chat/completions", h.ChatCompletions)
	rg.POST("/completions", h.Completions)
	rg.POST("/embeddings", h.Embeddings)
}
