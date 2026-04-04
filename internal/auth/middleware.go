package auth

import (
	"strings"

	"github.com/gin-gonic/gin"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// JWTMiddleware validates the Authorization: Bearer <jwt> header
// and sets user context keys (user_id, org_id, email, role) on the gin context.
func JWTMiddleware(jwtSvc *JWTService) gin.HandlerFunc {
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

		c.Set("user_id", claims.UserID)
		c.Set("org_id", claims.OrgID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Set("scopes", claims.Scopes)
		c.Next()
	}
}
