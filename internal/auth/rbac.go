package auth

import (
	"github.com/gin-gonic/gin"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// RequireRole returns middleware that ensures the authenticated user has one of the allowed roles.
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	roleSet := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		roleSet[r] = struct{}{}
	}

	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			middleware.ErrorResponse(c, taasErrors.Unauthorized("missing role in token"))
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid role format"))
			return
		}

		if _, allowed := roleSet[roleStr]; !allowed {
			middleware.ErrorResponse(c, taasErrors.Forbidden("insufficient permissions: requires one of "+formatRoles(allowedRoles)))
			return
		}

		c.Next()
	}
}

func formatRoles(roles []string) string {
	s := ""
	for i, r := range roles {
		if i > 0 {
			s += ", "
		}
		s += r
	}
	return s
}
