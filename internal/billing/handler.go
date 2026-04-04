package billing

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// UsageSummary is the response for usage summary queries.
type UsageSummary struct {
	TotalRequests    int     `json:"total_requests"`
	TotalPromptTokens     int     `json:"total_prompt_tokens"`
	TotalCompletionTokens int     `json:"total_completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
}

// ModelUsage is usage broken down by model.
type ModelUsage struct {
	ModelID          string  `json:"model_id"`
	TotalRequests    int     `json:"total_requests"`
	TotalTokens      int     `json:"total_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
}

// TokenUsage is usage broken down by API token.
type TokenUsage struct {
	TokenID          string  `json:"token_id"`
	TotalRequests    int     `json:"total_requests"`
	TotalTokens      int     `json:"total_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
}

// Handler holds billing/usage HTTP handlers.
type Handler struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewHandler(db *pgxpool.Pool, logger *zap.Logger) *Handler {
	return &Handler{db: db, logger: logger}
}

// Summary returns aggregate usage for the authenticated org.
func (h *Handler) Summary(c *gin.Context) {
	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	var summary UsageSummary
	err = h.db.QueryRow(c.Request.Context(),
		`SELECT COALESCE(COUNT(*),0), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		 COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost_usd),0), COALESCE(AVG(latency_ms),0)
		 FROM usage_records WHERE org_id = $1`, oid.String(),
	).Scan(&summary.TotalRequests, &summary.TotalPromptTokens, &summary.TotalCompletionTokens,
		&summary.TotalTokens, &summary.TotalCostUSD, &summary.AvgLatencyMs)
	if err != nil {
		h.logger.Error("querying usage summary", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to query usage"))
		return
	}

	c.JSON(http.StatusOK, summary)
}

// ByModel returns usage grouped by model.
func (h *Handler) ByModel(c *gin.Context) {
	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	rows, err := h.db.Query(c.Request.Context(),
		`SELECT model_id, COUNT(*), COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost_usd),0)
		 FROM usage_records WHERE org_id = $1 GROUP BY model_id ORDER BY SUM(cost_usd) DESC`,
		oid.String(),
	)
	if err != nil {
		h.logger.Error("querying usage by model", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to query usage"))
		return
	}
	defer rows.Close()

	var results []ModelUsage
	for rows.Next() {
		var mu ModelUsage
		if err := rows.Scan(&mu.ModelID, &mu.TotalRequests, &mu.TotalTokens, &mu.TotalCostUSD); err != nil {
			h.logger.Error("scanning model usage", zap.Error(err))
			middleware.ErrorResponse(c, taasErrors.Internal("failed to parse usage"))
			return
		}
		results = append(results, mu)
	}

	c.JSON(http.StatusOK, gin.H{"usage": results})
}

// ByToken returns usage grouped by API token.
func (h *Handler) ByToken(c *gin.Context) {
	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	rows, err := h.db.Query(c.Request.Context(),
		`SELECT token_id, COUNT(*), COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost_usd),0)
		 FROM usage_records WHERE org_id = $1 GROUP BY token_id ORDER BY SUM(cost_usd) DESC`,
		oid.String(),
	)
	if err != nil {
		h.logger.Error("querying usage by token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to query usage"))
		return
	}
	defer rows.Close()

	var results []TokenUsage
	for rows.Next() {
		var tu TokenUsage
		if err := rows.Scan(&tu.TokenID, &tu.TotalRequests, &tu.TotalTokens, &tu.TotalCostUSD); err != nil {
			h.logger.Error("scanning token usage", zap.Error(err))
			middleware.ErrorResponse(c, taasErrors.Internal("failed to parse usage"))
			return
		}
		results = append(results, tu)
	}

	c.JSON(http.StatusOK, gin.H{"usage": results})
}

// RegisterRoutes sets up usage/billing routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/summary", h.Summary)
	rg.GET("/by-model", h.ByModel)
	rg.GET("/by-token", h.ByToken)
}
