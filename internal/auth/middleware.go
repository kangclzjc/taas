package auth

import (
	"strings"

	"github.com/gin-gonic/gin"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// JWTMiddleware validates the Authorization: Bearer <jwt> header
// and sets user context keys (user_id, org_id, email, role) on the gin context.
// If a blocklist is provided, revoked tokens are rejected.
func JWTMiddleware(jwtSvc *JWTService, blocklist *Blocklist) gin.HandlerFunc {
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

		claims, err := jwtSvc.ValidateToken(parts[1])
		if err != nil {
			middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid or expired token"))
			return
		}
		if claims.TokenType != "access" {
			middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid token type"))
			return
		}

		// Check if token has been revoked
		if blocklist != nil && claims.ID != "" {
			blocked, err := blocklist.IsBlocked(c.Request.Context(), claims.ID)
			if err != nil {
				// Log error but fail closed: reject the request if we can't verify
				middleware.ErrorResponse(c, taasErrors.Internal("failed to verify token status"))
				return
			}
			if blocked {
				middleware.ErrorResponse(c, taasErrors.Unauthorized("token has been revoked"))
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("org_id", claims.OrgID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Set("scopes", claims.Scopes)
		c.Next()
	}
}
