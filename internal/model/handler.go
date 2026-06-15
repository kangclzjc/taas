package model

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// slugPattern validates model slugs: lowercase letters, digits, hyphens only (P2)
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// Handler holds model management HTTP handlers.
type Handler struct {
	svc                   *Service
	sharing               *SharingService
	logger                *zap.Logger
	onDeploymentDeleted   func(context.Context, uuid.UUID) error
	runtimeStatusProvider DeploymentRuntimeStatusProvider
}

func NewHandler(svc *Service, sharing *SharingService, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, sharing: sharing, logger: logger}
}

func (h *Handler) SetDeploymentDeleteHook(hook func(context.Context, uuid.UUID) error) {
	h.onDeploymentDeleted = hook
}

func (h *Handler) SetDeploymentRuntimeStatusProvider(provider DeploymentRuntimeStatusProvider) {
	h.runtimeStatusProvider = provider
}

type deploymentListItem struct {
	*Deployment
	K8sStatus *DeploymentRuntimeStatus `json:"k8s_status,omitempty"`
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
	Name    string  `json:"name" binding:"required"`
	SLATier SLATier `json:"sla_tier"`

	// ── Deploy Mode ────────────────────────────────────────────
	// "dgdr" (default): DynamoGraphDeploymentRequest — SLA-driven, auto-profiling, auto-config.
	//   Dynamo runs AIConfigurator to find optimal config, then deploys. Slower but optimal.
	// "dgd": DynamoGraphDeployment — Direct deploy with explicit config. No profiling.
	//   User must specify exact replicas, TP/PP, image, etc. Fast but requires expertise.
	DeployMode string `json:"deploy_mode"` // "dgdr" (default) or "dgd"

	// ── Hardware ───────────────────────────────────────────────
	GPUType            string `json:"gpu_type"`              // GPU SKU: h200_sxm, h100_sxm, a100_sxm, etc.
	GPUCountPerReplica int    `json:"gpu_count_per_replica"` // GPUs per worker replica
	NumGPUsPerNode     int    `json:"num_gpus_per_node"`     // GPUs per node (for DGDR hardware spec)
	VRAMMb             int    `json:"vram_mb,omitempty"`     // GPU VRAM in MiB (auto-detected if omitted)

	// ── Scaling ────────────────────────────────────────────────
	ReplicasMin        int   `json:"replicas_min"`
	ReplicasMax        int   `json:"replicas_max"`
	AutoscalingEnabled *bool `json:"autoscaling_enabled,omitempty"`

	// ── Inference Engine ───────────────────────────────────────
	Backend      string `json:"backend"`                 // vllm, sglang, trtllm (default: vllm)
	BackendImage string `json:"backend_image,omitempty"` // Container image override

	// ── Parallelism (Dynamo disaggregated serving) ─────────────
	TensorParallelSize   int `json:"tensor_parallel_size,omitempty"`   // TP degree (default: 1)
	PipelineParallelSize int `json:"pipeline_parallel_size,omitempty"` // PP degree (default: 1)

	// ── Workload Profile (for DGDR SLA optimization) ──────────
	InputSequenceLength  int `json:"input_sequence_length,omitempty"`  // Expected input token length (ISL)
	OutputSequenceLength int `json:"output_sequence_length,omitempty"` // Expected output token length (OSL)

	// ── SLA Targets (for DGDR auto-configuration) ─────────────
	TargetTTFTMs float64 `json:"target_ttft_ms,omitempty"` // Time To First Token target (ms)
	TargetITLMs  float64 `json:"target_itl_ms,omitempty"`  // Inter-Token Latency target (ms)
	TargetTPOTMs float64 `json:"target_tpot_ms,omitempty"` // Time Per Output Token target (ms)

	// ── Disaggregated Serving ─────────────────────────────────
	DisaggEnabled               bool   `json:"disagg_enabled,omitempty"`   // Enable prefill/decode disaggregation
	PrefillReplicas             int    `json:"prefill_replicas,omitempty"` // Number of prefill workers
	DecodeReplicas              int    `json:"decode_replicas,omitempty"`  // Number of decode workers
	PrefillGPUCountPerReplica   int    `json:"prefill_gpu_count_per_replica,omitempty"`
	DecodeGPUCountPerReplica    int    `json:"decode_gpu_count_per_replica,omitempty"`
	PrefillTensorParallelSize   int    `json:"prefill_tensor_parallel_size,omitempty"`
	DecodeTensorParallelSize    int    `json:"decode_tensor_parallel_size,omitempty"`
	PrefillPipelineParallelSize int    `json:"prefill_pipeline_parallel_size,omitempty"`
	DecodePipelineParallelSize  int    `json:"decode_pipeline_parallel_size,omitempty"`
	PrefillBackendImage         string `json:"prefill_backend_image,omitempty"`
	DecodeBackendImage          string `json:"decode_backend_image,omitempty"`
	SearchStrategy              string `json:"search_strategy,omitempty"` // AIConfigurator strategy: rapid, thorough

	// ── DGD-specific (direct deploy) ──────────────────────────
	// These are only used when deploy_mode = "dgd".
	FrontendReplicas int               `json:"frontend_replicas,omitempty"` // Frontend HTTP server replicas
	WorkerCommand    string            `json:"worker_command,omitempty"`    // Custom worker command override
	DynamoNamespace  string            `json:"dynamo_namespace,omitempty"`  // Dynamo service discovery namespace
	RouterMode       string            `json:"router_mode,omitempty"`       // "random" or "kv" (KV-aware routing)
	EnvVars          map[string]string `json:"env_vars,omitempty"`          // Extra env vars for workers

	// ── Advanced ──────────────────────────────────────────────
	MaxBatchSize      int               `json:"max_batch_size"`
	MaxSequenceLength int               `json:"max_sequence_length"`  // Max context window
	Dtype             string            `json:"dtype,omitempty"`      // fp16, bf16, fp8 (default: auto)
	AutoApply         *bool             `json:"auto_apply,omitempty"` // DGDR autoApply (default: true)
	ExtraArgs         map[string]string `json:"extra_args,omitempty"` // Additional backend-specific args
	PrefillExtraArgs  map[string]string `json:"prefill_extra_args,omitempty"`
	DecodeExtraArgs   map[string]string `json:"decode_extra_args,omitempty"`
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

	// Validate slug charset: a-z, 0-9, hyphens only (P2)
	slug := strings.ToLower(req.Slug)
	if len(slug) < 2 || len(slug) > 64 || !slugPattern.MatchString(slug) {
		middleware.ErrorResponse(c, taasErrors.BadRequest("slug must be 2-64 chars, lowercase letters, digits, and hyphens only"))
		return
	}

	m := &Model{
		ID:             uuid.New(),
		OrgID:          oid,
		OwnerUserID:    uid,
		Name:           req.Name,
		Slug:           slug,
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

	// Default deploy mode
	deployMode := req.DeployMode
	if deployMode == "" {
		deployMode = "dgdr"
	}
	if deployMode != "dgdr" && deployMode != "dgd" {
		middleware.ErrorResponse(c, taasErrors.BadRequest("deploy_mode must be 'dgdr' or 'dgd'"))
		return
	}

	d, err := h.svc.Deploy(c.Request.Context(), modelID, oid, DeployConfig{
		Name:       req.Name,
		SLATier:    req.SLATier,
		DeployMode: deployMode,
		// Hardware
		GPUType:            req.GPUType,
		GPUCountPerReplica: req.GPUCountPerReplica,
		NumGPUsPerNode:     req.NumGPUsPerNode,
		VRAMMb:             req.VRAMMb,
		// Scaling
		ReplicasMin:        req.ReplicasMin,
		ReplicasMax:        req.ReplicasMax,
		AutoscalingEnabled: req.AutoscalingEnabled,
		// Inference engine
		Backend:      req.Backend,
		BackendImage: req.BackendImage,
		// Parallelism
		TensorParallelSize:   req.TensorParallelSize,
		PipelineParallelSize: req.PipelineParallelSize,
		// Workload profile
		InputSequenceLength:  req.InputSequenceLength,
		OutputSequenceLength: req.OutputSequenceLength,
		// SLA targets
		TargetTTFTMs: req.TargetTTFTMs,
		TargetITLMs:  req.TargetITLMs,
		TargetTPOTMs: req.TargetTPOTMs,
		// Disaggregated serving
		DisaggEnabled:               req.DisaggEnabled,
		PrefillReplicas:             req.PrefillReplicas,
		DecodeReplicas:              req.DecodeReplicas,
		PrefillGPUCountPerReplica:   req.PrefillGPUCountPerReplica,
		DecodeGPUCountPerReplica:    req.DecodeGPUCountPerReplica,
		PrefillTensorParallelSize:   req.PrefillTensorParallelSize,
		DecodeTensorParallelSize:    req.DecodeTensorParallelSize,
		PrefillPipelineParallelSize: req.PrefillPipelineParallelSize,
		DecodePipelineParallelSize:  req.DecodePipelineParallelSize,
		PrefillBackendImage:         req.PrefillBackendImage,
		DecodeBackendImage:          req.DecodeBackendImage,
		SearchStrategy:              req.SearchStrategy,
		// DGD-specific
		FrontendReplicas: req.FrontendReplicas,
		WorkerCommand:    req.WorkerCommand,
		DynamoNamespace:  req.DynamoNamespace,
		RouterMode:       req.RouterMode,
		EnvVars:          req.EnvVars,
		// Advanced
		MaxBatchSize:      req.MaxBatchSize,
		MaxSequenceLength: req.MaxSequenceLength,
		Dtype:             req.Dtype,
		AutoApply:         req.AutoApply,
		ExtraArgs:         req.ExtraArgs,
		PrefillExtraArgs:  req.PrefillExtraArgs,
		DecodeExtraArgs:   req.DecodeExtraArgs,
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

// ListDeployments returns all deployments for a model.
func (h *Handler) ListDeployments(c *gin.Context) {
	modelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid model id"))
		return
	}

	deployments, err := h.svc.repo.ListDeployments(c.Request.Context(), modelID)
	if err != nil {
		h.logger.Error("listing deployments", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to list deployments"))
		return
	}

	items := make([]deploymentListItem, 0, len(deployments))
	for _, dep := range deployments {
		item := deploymentListItem{Deployment: dep}
		if h.runtimeStatusProvider != nil && dep.Status != DeploymentStopped {
			status, statusErr := h.runtimeStatusProvider.GetDeploymentRuntimeStatus(c.Request.Context(), dep.ID)
			if statusErr != nil {
				h.logger.Warn(
					"reading deployment runtime status",
					zap.Error(statusErr),
					zap.String("deployment_id", dep.ID.String()),
				)
				status = &DeploymentRuntimeStatus{
					Available: false,
					Found:     false,
					Error:     statusErr.Error(),
				}
			}
			item.K8sStatus = status
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"deployments": items})
}

// DeleteDeployment stops a deployment and requests backend cleanup.
func (h *Handler) DeleteDeployment(c *gin.Context) {
	modelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid model id"))
		return
	}
	deploymentID, err := uuid.Parse(c.Param("deploymentId"))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid deployment id"))
		return
	}
	orgID, _ := c.Get("org_id")
	oid, err := uuid.Parse(orgID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid org id"))
		return
	}

	if err := h.svc.DeleteDeployment(c.Request.Context(), modelID, deploymentID, oid); err != nil {
		h.logger.Error("deleting deployment", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to delete deployment: "+err.Error()))
		return
	}
	if h.onDeploymentDeleted != nil {
		if err := h.onDeploymentDeleted(c.Request.Context(), deploymentID); err != nil {
			h.logger.Warn("deployment delete hook failed", zap.Error(err), zap.String("deployment_id", deploymentID.String()))
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "deployment deleted"})
}

// RegisterRoutes sets up model routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", h.CreateModel)
	rg.GET("", h.ListModels)
	rg.GET("/:id", h.GetModel)
	rg.DELETE("/:id", h.DeleteModel)
	rg.POST("/:id/deploy", h.DeployModel)
	rg.POST("/:id/share", h.ShareModel)
	rg.GET("/:id/deployments", h.ListDeployments)
	rg.DELETE("/:id/deployments/:deploymentId", h.DeleteDeployment)
}
