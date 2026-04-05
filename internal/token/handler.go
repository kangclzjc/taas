package token

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// Handler holds token management HTTP handlers.
type Handler struct {
	svc    *Service
	logger *zap.Logger
}

func NewHandler(svc *Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Create generates a new API token.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	userID, _ := c.Get("user_id")
	orgID, _ := c.Get("org_id")

	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid user id"))
		return
	}
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	result, err := h.svc.Create(c.Request.Context(), uid, oid, req)
	if err != nil {
		h.logger.Error("creating token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to create token"))
		return
	}

	c.JSON(http.StatusCreated, result)
}

// List returns all tokens for the authenticated user's org.
func (h *Handler) List(c *gin.Context) {
	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	// Parse pagination parameters (P2)
	limit := 100
	offset := 0
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	tokens, err := h.svc.List(c.Request.Context(), oid, limit, offset)
	if err != nil {
		h.logger.Error("listing tokens", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to list tokens"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"tokens": tokens, "limit": limit, "offset": offset})
}

// Delete revokes a token by ID.
func (h *Handler) Delete(c *gin.Context) {
	tokenID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid token id"))
		return
	}

	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	if err := h.svc.Revoke(c.Request.Context(), tokenID, oid); err != nil {
		h.logger.Error("revoking token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.NotFound("token"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "token revoked"})
}

// Rotate generates a new secret for an existing token.
func (h *Handler) Rotate(c *gin.Context) {
	tokenID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid token id"))
		return
	}

	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	newRaw, err := h.svc.Rotate(c.Request.Context(), tokenID, oid)
	if err != nil {
		h.logger.Error("rotating token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to rotate token"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"key": newRaw})
}

// RegisterRoutes sets up token routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", h.Create)
	rg.GET("", h.List)
	rg.DELETE("/:id", h.Delete)
	rg.POST("/:id/rotate", h.Rotate)
}
