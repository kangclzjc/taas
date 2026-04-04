package model

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// Handler holds model management HTTP handlers.
type Handler struct {
	svc     *Service
	sharing *SharingService
	logger  *zap.Logger
}

func NewHandler(svc *Service, sharing *SharingService, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, sharing: sharing, logger: logger}
}

type createModelRequest struct {
	Name           string    `json:"name" binding:"required"`
	Slug           string    `json:"slug" binding:"required"`
	Description    string    `json:"description"`
	Framework      Framework `json:"framework" binding:"required"`
	Format         string    `json:"format"`
	StorageURI     string    `json:"storage_uri"`
	ParameterCount int64     `json:"parameter_count"`
	ContextLength  int       `json:"context_length"`
	IsPublic       bool      `json:"is_public"`
}

type deployModelRequest struct {
	Name               string  `json:"name" binding:"required"`
	SLATier            SLATier `json:"sla_tier"`
	ReplicasMin        int     `json:"replicas_min"`
	ReplicasMax        int     `json:"replicas_max"`
	GPUType            string  `json:"gpu_type"`
	GPUCountPerReplica int     `json:"gpu_count_per_replica"`
	MaxBatchSize       int     `json:"max_batch_size"`
	MaxSequenceLength  int     `json:"max_sequence_length"`
}

type shareModelRequest struct {
	TargetOrgID uuid.UUID `json:"target_org_id" binding:"required"`
	Permission  string    `json:"permission"` // "read" or "deploy"
}

// CreateModel registers a new model.
func (h *Handler) CreateModel(c *gin.Context) {
	var req createModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	orgID, _ := c.Get("org_id")
	userID, _ := c.Get("user_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}
	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid user id"))
		return
	}

	m := &Model{
		ID:             uuid.New(),
		OrgID:          oid,
		OwnerUserID:    uid,
		Name:           req.Name,
		Slug:           strings.ToLower(req.Slug),
		Description:    req.Description,
		Framework:      req.Framework,
		Format:         req.Format,
		StorageURI:     req.StorageURI,
		ParameterCount: req.ParameterCount,
		ContextLength:  req.ContextLength,
		IsPublic:       req.IsPublic,
		Status:         StatusReady,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	if err := h.svc.repo.CreateModel(c.Request.Context(), m); err != nil {
		h.logger.Error("creating model", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to create model"))
		return
	}

	c.JSON(http.StatusCreated, m)
}

// ListModels returns models visible to the authenticated org.
func (h *Handler) ListModels(c *gin.Context) {
	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	filter := ListModelsFilter{OrgID: &oid, Limit: 50}
	models, total, err := h.svc.repo.ListModels(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("listing models", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to list models"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"models": models, "total": total})
}

// GetModel returns a single model by ID.
func (h *Handler) GetModel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid model id"))
		return
	}

	m, err := h.svc.repo.GetModel(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("getting model", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to get model"))
		return
	}
	if m == nil {
		middleware.ErrorResponse(c, taasErrors.NotFound("model"))
		return
	}

	c.JSON(http.StatusOK, m)
}

// DeleteModel removes a model.
func (h *Handler) DeleteModel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid model id"))
		return
	}

	if err := h.svc.repo.DeleteModel(c.Request.Context(), id); err != nil {
		h.logger.Error("deleting model", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.NotFound("model"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model deleted"})
}

// DeployModel initiates a deployment for a model.
func (h *Handler) DeployModel(c *gin.Context) {
	modelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid model id"))
		return
	}

	var req deployModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	if req.SLATier == "" {
		req.SLATier = SLAStandard
	}
	if req.ReplicasMin == 0 {
		req.ReplicasMin = 1
	}
	if req.ReplicasMax == 0 {
		req.ReplicasMax = req.ReplicasMin
	}

	d, err := h.svc.Deploy(c.Request.Context(), modelID, oid, DeployConfig{
		Name:               req.Name,
		SLATier:            req.SLATier,
		ReplicasMin:        req.ReplicasMin,
		ReplicasMax:        req.ReplicasMax,
		GPUType:            req.GPUType,
		GPUCountPerReplica: req.GPUCountPerReplica,
		MaxBatchSize:       req.MaxBatchSize,
		MaxSequenceLength:  req.MaxSequenceLength,
	})
	if err != nil {
		h.logger.Error("deploying model", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to deploy model: "+err.Error()))
		return
	}

	c.JSON(http.StatusCreated, d)
}

// ShareModel grants access to another org.
func (h *Handler) ShareModel(c *gin.Context) {
	modelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid model id"))
		return
	}

	var req shareModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	if req.Permission == "" {
		req.Permission = "read"
	}

	if err := h.sharing.Grant(c.Request.Context(), modelID, oid, req.TargetOrgID, req.Permission); err != nil {
		h.logger.Error("sharing model", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to share model"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model shared"})
}

// RegisterRoutes sets up model routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", h.CreateModel)
	rg.GET("", h.ListModels)
	rg.GET("/:id", h.GetModel)
	rg.DELETE("/:id", h.DeleteModel)
	rg.POST("/:id/deploy", h.DeployModel)
	rg.POST("/:id/share", h.ShareModel)
}
