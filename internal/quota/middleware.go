package quota

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/token"
	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// RateLimitMiddleware enforces RPM rate limits based on the validated token info.
func RateLimitMiddleware(limiter *RateLimiter, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		val, exists := c.Get("token_info")
		if !exists {
			c.Next()
			return
		}
		info, ok := val.(*token.CachedTokenInfo)
		if !ok || info.RateLimitRPM <= 0 {
			c.Next()
			return
		}

		allowed, remaining, err := limiter.CheckRPM(c.Request.Context(), info.TokenID, info.RateLimitRPM)
		if err != nil {
			logger.Error("rate limit check failed", zap.Error(err))
			// Fail open: allow request but log the error
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", info.RateLimitRPM))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if !allowed {
			middleware.ErrorResponse(c, taasErrors.RateLimitExceeded())
			return
		}

		c.Next()
	}
}
