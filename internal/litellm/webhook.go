// Package litellm provides integration with LiteLLM Proxy for the TaaS platform.
//
// This package handles:
//   - Webhook callbacks from LiteLLM (usage tracking)
//   - Admin API client for virtual key management (sync with TaaS tokens)
package litellm

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// WebhookHandler receives LiteLLM success/failure callbacks and persists
// usage data to the TaaS usage_records table.
type WebhookHandler struct {
	db     *pgxpool.Pool
	secret string
	logger *zap.Logger
}

func NewWebhookHandler(db *pgxpool.Pool, secret string, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{db: db, secret: secret, logger: logger}
}

// LiteLLMCallback represents the webhook payload from LiteLLM.
// Docs: https://docs.litellm.ai/docs/proxy/logging#webhook-callbacks
type LiteLLMCallback struct {
	CallType    string            `json:"call_type"`    // "completion", "embedding", etc.
	Model       string            `json:"model"`
	Messages    json.RawMessage   `json:"messages"`
	StartTime   string            `json:"startTime"`
	EndTime     string            `json:"endTime"`
	Status      string            `json:"status"`       // "success" or "failure"
	Response    json.RawMessage   `json:"response"`
	Usage       *LiteLLMUsage     `json:"usage"`
	Metadata    *LiteLLMMetadata  `json:"metadata"`
	Cost        float64           `json:"response_cost"`
	RequestID   string            `json:"request_id"`
	APIKey      string            `json:"api_key"`      // Hashed key
	KeyAlias    string            `json:"key_alias"`
	TeamID      string            `json:"team_id"`
	UserID      string            `json:"user"`
	ErrorStr    string            `json:"error_str,omitempty"`
}

type LiteLLMUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type LiteLLMMetadata struct {
	UserAPIKey     string `json:"user_api_key"`
	UserAPIKeyHash string `json:"user_api_key_hash"`
	UserAPIKeyAlias string `json:"user_api_key_alias"`
	UserAPITeamID  string `json:"user_api_key_team_id"`
	UserAPIUserID  string `json:"user_api_key_user_id"`
}

// HandleCallback processes incoming LiteLLM webhook callbacks.
// POST /webhooks/litellm
func (h *WebhookHandler) HandleCallback(c *gin.Context) {
	// Validate webhook secret
	auth := c.GetHeader("Authorization")
	if auth != "Bearer "+h.secret {
		h.logger.Warn("webhook auth failed", zap.String("remote", c.ClientIP()))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var callback LiteLLMCallback
	if err := c.ShouldBindJSON(&callback); err != nil {
		h.logger.Warn("invalid webhook payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	h.logger.Debug("litellm callback received",
		zap.String("request_id", callback.RequestID),
		zap.String("model", callback.Model),
		zap.String("status", callback.Status),
		zap.Float64("cost", callback.Cost),
	)

	// Extract identifiers
	orgID := callback.TeamID
	if orgID == "" && callback.Metadata != nil {
		orgID = callback.Metadata.UserAPITeamID
	}
	if orgID == "" {
		orgID = callback.UserID
	}

	tokenID := callback.KeyAlias
	if tokenID == "" && callback.Metadata != nil {
		tokenID = callback.Metadata.UserAPIKeyAlias
	}

	userID := callback.UserID
	if userID == "" && callback.Metadata != nil {
		userID = callback.Metadata.UserAPIUserID
	}

	// Calculate latency from timestamps
	var latencyMs int
	if callback.StartTime != "" && callback.EndTime != "" {
		start, err1 := time.Parse(time.RFC3339Nano, callback.StartTime)
		end, err2 := time.Parse(time.RFC3339Nano, callback.EndTime)
		if err1 == nil && err2 == nil {
			latencyMs = int(end.Sub(start).Milliseconds())
		}
	}

	// Token counts
	var promptTokens, completionTokens, totalTokens int
	if callback.Usage != nil {
		promptTokens = callback.Usage.PromptTokens
		completionTokens = callback.Usage.CompletionTokens
		totalTokens = callback.Usage.TotalTokens
	}

	// Determine error code
	errorCode := ""
	if callback.Status == "failure" {
		errorCode = "litellm_error"
		if callback.ErrorStr != "" && len(callback.ErrorStr) <= 50 {
			errorCode = callback.ErrorStr
		}
	}

	// Insert into usage_records (same schema as billing collector)
	_, err := h.db.Exec(c.Request.Context(),
		`INSERT INTO usage_records (
			request_id, token_id, user_id, org_id, model_id,
			prompt_tokens, completion_tokens, total_tokens,
			latency_ms, status, error_code, cost_usd, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (request_id) DO UPDATE SET
			cost_usd = EXCLUDED.cost_usd,
			status = EXCLUDED.status`,
		callback.RequestID,
		tokenID,
		userID,
		orgID,
		callback.Model,
		promptTokens,
		completionTokens,
		totalTokens,
		latencyMs,
		callback.Status,
		errorCode,
		callback.Cost,
		time.Now().UTC(),
	)
	if err != nil {
		h.logger.Error("failed to insert usage record from webhook",
			zap.Error(err),
			zap.String("request_id", callback.RequestID),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record usage"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "recorded"})
}

// RegisterRoutes sets up webhook routes on the given router group.
func (h *WebhookHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/litellm", h.HandleCallback)
}
