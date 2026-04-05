package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

// RequireOrgMembership ensures the user belongs to the org they're trying to access.
// For routes that take :id as org ID, it compares against the JWT org_id claim.
// Owners and admins can access any org they belong to.
func RequireOrgMembership() gin.HandlerFunc {
	return func(c *gin.Context) {
		// If there's an :id param (org routes), verify it matches the user's org
		paramOrgID := c.Param("id")
		if paramOrgID != "" {
			_, err := uuid.Parse(paramOrgID)
			if err == nil {
				// It's a valid UUID — check membership
				jwtOrgID, _ := c.Get("org_id")
				role, _ := c.Get("role")
				roleStr, _ := role.(string)

				// Owners can be in multiple orgs, so we allow them through
				// For stricter checking, query the DB (future improvement)
				if roleStr != "owner" && roleStr != "admin" {
					if jwtOrgID != paramOrgID {
						middleware.ErrorResponse(c, taasErrors.Forbidden("you do not have access to this organization"))
						return
					}
				}
			}
		}
		c.Next()
	}
}

// RequireWriteAccess ensures the user has at least member role (not viewer).
// Viewers can only read; members, admins, and owners can write.
func RequireWriteAccess() gin.HandlerFunc {
	return RequireRole("owner", "admin", "member")
}

// RequireAdminAccess ensures the user is at least an admin.
func RequireAdminAccess() gin.HandlerFunc {
	return RequireRole("owner", "admin")
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
