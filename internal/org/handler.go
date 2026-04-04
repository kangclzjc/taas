package org

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/auth"
	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// Handler holds organization HTTP handlers.
type Handler struct {
	repo     OrgRepository
	userRepo auth.UserRepository
	logger   *zap.Logger
}

// NewHandler creates a new org handler.
func NewHandler(repo OrgRepository, userRepo auth.UserRepository, logger *zap.Logger) *Handler {
	return &Handler{
		repo:     repo,
		userRepo: userRepo,
		logger:   logger,
	}
}

type createOrgRequest struct {
	Slug        string `json:"slug" binding:"required,min=2,max=64"`
	DisplayName string `json:"display_name" binding:"required,min=1,max=256"`
	SLATier     string `json:"sla_tier"`
}

type updateOrgRequest struct {
	Slug        string `json:"slug" binding:"omitempty,min=2,max=64"`
	DisplayName string `json:"display_name" binding:"omitempty,min=1,max=256"`
	SLATier     string `json:"sla_tier"`
}

type addMemberRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role"`
}

// CreateOrg creates a new organization and adds the creator as owner.
func (h *Handler) CreateOrg(c *gin.Context) {
	var req createOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	userID, _ := c.Get("user_id")
	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid user id"))
		return
	}

	// Check slug uniqueness
	existing, err := h.repo.GetBySlug(c.Request.Context(), req.Slug)
	if err != nil {
		h.logger.Error("checking slug uniqueness", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to create organization"))
		return
	}
	if existing != nil {
		middleware.ErrorResponse(c, taasErrors.Conflict("organization slug already taken"))
		return
	}

	org := &Organization{
		Slug:        req.Slug,
		DisplayName: req.DisplayName,
		SLATier:     req.SLATier,
	}

	if err := h.repo.Create(c.Request.Context(), org); err != nil {
		h.logger.Error("creating organization", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to create organization"))
		return
	}

	// Add creator as owner
	member := &OrgMember{
		OrgID:  org.ID,
		UserID: uid,
		Role:   "owner",
	}
	if err := h.repo.AddMember(c.Request.Context(), member); err != nil {
		h.logger.Error("adding org creator as member", zap.Error(err))
		// Still return the org, but log the error
	}

	c.JSON(http.StatusCreated, org)
}

// ListOrgs returns all organizations the authenticated user belongs to.
func (h *Handler) ListOrgs(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid user id"))
		return
	}

	orgs, err := h.repo.ListByUser(c.Request.Context(), uid)
	if err != nil {
		h.logger.Error("listing organizations", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to list organizations"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"organizations": orgs})
}

// GetOrg returns a single organization by ID.
func (h *Handler) GetOrg(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid organization id"))
		return
	}

	org, err := h.repo.GetByID(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("getting organization", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to get organization"))
		return
	}
	if org == nil {
		middleware.ErrorResponse(c, taasErrors.NotFound("organization"))
		return
	}

	c.JSON(http.StatusOK, org)
}

// UpdateOrg updates an organization.
func (h *Handler) UpdateOrg(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid organization id"))
		return
	}

	var req updateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	org, err := h.repo.GetByID(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("getting organization for update", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to update organization"))
		return
	}
	if org == nil {
		middleware.ErrorResponse(c, taasErrors.NotFound("organization"))
		return
	}

	if req.Slug != "" {
		// Check slug uniqueness if changing
		if req.Slug != org.Slug {
			existing, err := h.repo.GetBySlug(c.Request.Context(), req.Slug)
			if err != nil {
				h.logger.Error("checking slug uniqueness", zap.Error(err))
				middleware.ErrorResponse(c, taasErrors.Internal("failed to update organization"))
				return
			}
			if existing != nil {
				middleware.ErrorResponse(c, taasErrors.Conflict("organization slug already taken"))
				return
			}
		}
		org.Slug = req.Slug
	}
	if req.DisplayName != "" {
		org.DisplayName = req.DisplayName
	}
	if req.SLATier != "" {
		org.SLATier = req.SLATier
	}

	if err := h.repo.Update(c.Request.Context(), org); err != nil {
		h.logger.Error("updating organization", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to update organization"))
		return
	}

	c.JSON(http.StatusOK, org)
}

// DeleteOrg deletes an organization.
func (h *Handler) DeleteOrg(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid organization id"))
		return
	}

	if err := h.repo.Delete(c.Request.Context(), orgID); err != nil {
		h.logger.Error("deleting organization", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.NotFound("organization"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "organization deleted"})
}

// ListMembers returns all members of an organization.
func (h *Handler) ListMembers(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid organization id"))
		return
	}

	members, err := h.repo.ListMembers(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("listing org members", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to list members"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}

// AddMember adds a new member to the organization by email.
func (h *Handler) AddMember(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid organization id"))
		return
	}

	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	// Verify org exists
	org, err := h.repo.GetByID(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("getting organization for add member", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to add member"))
		return
	}
	if org == nil {
		middleware.ErrorResponse(c, taasErrors.NotFound("organization"))
		return
	}

	// Find user by email
	user, err := h.userRepo.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		h.logger.Error("looking up user by email", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to add member"))
		return
	}
	if user == nil {
		middleware.ErrorResponse(c, taasErrors.NotFound("user with that email"))
		return
	}

	role := req.Role
	if role == "" {
		role = "member"
	}

	member := &OrgMember{
		OrgID:  orgID,
		UserID: user.ID,
		Role:   role,
	}
	if err := h.repo.AddMember(c.Request.Context(), member); err != nil {
		h.logger.Error("adding org member", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to add member"))
		return
	}

	c.JSON(http.StatusCreated, member)
}

// RemoveMember removes a member from the organization.
func (h *Handler) RemoveMember(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid organization id"))
		return
	}

	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid user id"))
		return
	}

	if err := h.repo.RemoveMember(c.Request.Context(), orgID, userID); err != nil {
		h.logger.Error("removing org member", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.NotFound("member"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member removed"})
}

// RegisterRoutes sets up organization routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", h.CreateOrg)
	rg.GET("", h.ListOrgs)
	rg.GET("/:id", h.GetOrg)
	rg.PUT("/:id", h.UpdateOrg)
	rg.DELETE("/:id", h.DeleteOrg)
	rg.GET("/:id/members", h.ListMembers)
	rg.POST("/:id/members", h.AddMember)
	rg.DELETE("/:id/members/:userId", h.RemoveMember)
}
