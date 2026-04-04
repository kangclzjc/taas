package proxy

import (
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/token"
	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// APIKeyAuth validates the API key from the Authorization: Bearer <token> header.
// On success, it sets "token_info" on the gin context.
func APIKeyAuth(validator *token.Validator, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			middleware.ErrorResponse(c, taasErrors.Unauthorized("missing authorization header"))
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid authorization format"))
			return
		}

		rawToken := parts[1]
		info, err := validator.Validate(c.Request.Context(), rawToken)
		if err != nil {
			logger.Debug("token validation failed", zap.Error(err))
			switch err {
			case token.ErrTokenNotFound:
				middleware.ErrorResponse(c, taasErrors.New(taasErrors.CodeInvalidToken, "invalid API key", 401))
			case token.ErrTokenRevoked:
				middleware.ErrorResponse(c, taasErrors.New(taasErrors.CodeTokenRevoked, "API key has been revoked", 401))
			case token.ErrTokenExpired:
				middleware.ErrorResponse(c, taasErrors.New(taasErrors.CodeTokenExpired, "API key has expired", 401))
			default:
				middleware.ErrorResponse(c, taasErrors.Unauthorized("token validation failed"))
			}
			return
		}

		c.Set("token_info", info)
		c.Set("user_id", info.UserID)
		c.Set("org_id", info.OrgID)
		c.Next()
	}
}
