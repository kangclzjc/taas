package model

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Status represents a model's lifecycle state.
type Status string

const (
	StatusUploading  Status = "uploading"
	StatusValidating Status = "validating"
	StatusReady      Status = "ready"
	StatusError      Status = "error"
	StatusArchived   Status = "archived"
)

// Framework is the ML framework of the model.
type Framework string

const (
	FrameworkPyTorch    Framework = "pytorch"
	FrameworkTensorFlow Framework = "tensorflow"
	FrameworkONNX       Framework = "onnx"
	FrameworkTensorRT   Framework = "tensorrt"
)

// DeploymentStatus tracks a model deployment's state.
type DeploymentStatus string

const (
	DeploymentPending   DeploymentStatus = "pending"
	DeploymentDeploying DeploymentStatus = "deploying"
	DeploymentRunning   DeploymentStatus = "running"
	DeploymentScaling   DeploymentStatus = "scaling"
	DeploymentStopping  DeploymentStatus = "stopping"
	DeploymentStopped   DeploymentStatus = "stopped"
	DeploymentFailed    DeploymentStatus = "failed"
)

// SLATier defines service level agreement tiers.
type SLATier string

const (
	SLAStandard     SLATier = "standard"
	SLAProfessional SLATier = "professional"
	SLAEnterprise   SLATier = "enterprise"
)

// Model represents a registered AI model.
type Model struct {
	ID             uuid.UUID `db:"id"`
	OrgID          uuid.UUID `db:"org_id"`
	OwnerUserID    uuid.UUID `db:"owner_user_id"`
	Name           string    `db:"name"`
	Slug           string    `db:"slug"`
	Description    string    `db:"description"`
	Framework      Framework `db:"framework"`
	Format         string    `db:"format"`
	StorageURI     string    `db:"storage_uri"`
	StorageBytes   int64     `db:"storage_size_bytes"`
	ParameterCount int64     `db:"parameter_count"`
	ContextLength  int       `db:"context_length"`
	IsPublic       bool      `db:"is_public"`
	Status         Status    `db:"status"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

// Deployment represents a running model deployment.
// Fields map to NVIDIA Dynamo DGDR (DynamoGraphDeploymentRequest) spec.
type Deployment struct {
	ID         uuid.UUID        `db:"id"`
	ModelID    uuid.UUID        `db:"model_id"`
	OrgID      uuid.UUID        `db:"org_id"`
	Name       string           `db:"name"`
	Status     DeploymentStatus `db:"status"`
	SLATier    SLATier          `db:"sla_tier"`
	DeployMode string           `db:"deploy_mode"` // "dgdr" or "dgd"

	// Hardware
	GPUType            string `db:"gpu_type"` // GPU SKU: h200_sxm, h100_sxm, a100_sxm
	GPUCountPerReplica int    `db:"gpu_count_per_replica"`
	NumGPUsPerNode     int    `db:"num_gpus_per_node"` // DGDR hardware.numGpusPerNode
	VRAMMb             int    `db:"vram_mb"`           // DGDR hardware.vramMb

	// Scaling
	ReplicasMin     int `db:"replicas_min"`
	ReplicasMax     int `db:"replicas_max"`
	ReplicasCurrent int `db:"replicas_current"`

	// Inference engine
	Backend      string `db:"backend"`       // vllm, sglang, trtllm
	BackendImage string `db:"backend_image"` // Container image override
	EnvVars      map[string]string
	ExtraArgs    map[string]string

	// Parallelism (Dynamo disaggregated serving)
	TensorParallelSize   int `db:"tensor_parallel_size"`   // TP degree
	PipelineParallelSize int `db:"pipeline_parallel_size"` // PP degree

	// Workload profile (DGDR workload)
	InputSequenceLength  int `db:"input_sequence_length"`  // ISL
	OutputSequenceLength int `db:"output_sequence_length"` // OSL

	// SLA targets (DGDR sla)
	TargetTTFTMs float64 `db:"target_ttft_ms"` // Time To First Token (ms)
	TargetITLMs  float64 `db:"target_itl_ms"`  // Inter-Token Latency (ms)
	TargetTPOTMs float64 `db:"target_tpot_ms"` // Time Per Output Token (ms)

	// Disaggregated serving (P/D separation)
	DisaggEnabled               bool   `db:"disagg_enabled"`
	PrefillReplicas             int    `db:"prefill_replicas"`
	DecodeReplicas              int    `db:"decode_replicas"`
	PrefillGPUType              string `db:"prefill_gpu_type"`
	DecodeGPUType               string `db:"decode_gpu_type"`
	PrefillGPUCountPerReplica   int    `db:"prefill_gpu_count_per_replica"`
	DecodeGPUCountPerReplica    int    `db:"decode_gpu_count_per_replica"`
	PrefillTensorParallelSize   int    `db:"prefill_tensor_parallel_size"`
	DecodeTensorParallelSize    int    `db:"decode_tensor_parallel_size"`
	PrefillPipelineParallelSize int    `db:"prefill_pipeline_parallel_size"`
	DecodePipelineParallelSize  int    `db:"decode_pipeline_parallel_size"`
	PrefillBackendImage         string `db:"prefill_backend_image"`
	DecodeBackendImage          string `db:"decode_backend_image"`
	PrefillExtraArgs            map[string]string
	DecodeExtraArgs             map[string]string
	SearchStrategy              string `db:"search_strategy"` // AIConfigurator: rapid, thorough

	// DGD-specific (direct deploy, no profiling)
	FrontendReplicas int    `db:"frontend_replicas"` // Frontend HTTP replicas
	WorkerCommand    string `db:"worker_command"`    // Custom worker command
	DynamoNS         string `db:"dynamo_ns"`         // Dynamo service discovery namespace
	RouterMode       string `db:"router_mode"`       // "random" or "kv"

	// Advanced
	MaxBatchSize      int    `db:"max_batch_size"`
	MaxSequenceLength int    `db:"max_sequence_length"` // Max context window
	Dtype             string `db:"dtype"`               // fp16, bf16, fp8

	// Runtime state
	DynamoServiceName string     `db:"dynamo_service_name"`
	DynamoNamespace   string     `db:"dynamo_namespace"`
	EndpointURL       string     `db:"endpoint_url"`
	LiteLLMModelID    string     `db:"litellm_model_id"` // LiteLLM model ID for cleanup
	ErrorMessage      string     `db:"error_message"`
	DeployedAt        *time.Time `db:"deployed_at"`
	CreatedAt         time.Time  `db:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"`
}

// Repository defines data access methods for models and deployments.
type Repository interface {
	CreateModel(ctx context.Context, m *Model) error
	GetModel(ctx context.Context, id uuid.UUID) (*Model, error)
	GetModelBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Model, error)
	ListModels(ctx context.Context, filter ListModelsFilter) ([]*Model, int, error)
	UpdateModel(ctx context.Context, m *Model) error
	DeleteModel(ctx context.Context, id uuid.UUID) error

	CreateDeployment(ctx context.Context, d *Deployment) error
	GetDeployment(ctx context.Context, id uuid.UUID) (*Deployment, error)
	GetActiveDeployment(ctx context.Context, modelID uuid.UUID) (*Deployment, error)
	UpdateDeploymentStatus(ctx context.Context, id uuid.UUID, status DeploymentStatus, endpointURL, errorMsg string) error
	StopDeployment(ctx context.Context, id uuid.UUID) error
	ListDeployments(ctx context.Context, modelID uuid.UUID) ([]*Deployment, error)
	SetDeploymentLiteLLMID(ctx context.Context, id uuid.UUID, litellmModelID string) error
}

// ListModelsFilter specifies query criteria for model listing.
type ListModelsFilter struct {
	OrgID     *uuid.UUID
	IsPublic  *bool
	Framework *Framework
	Status    *Status
	Limit     int
	Offset    int
}

// Service provides business logic for model management.
type Service struct {
	repo            Repository
	sharing         *SharingService
	deployPublisher DeploymentPublisher
}

func NewService(repo Repository, sharing ...*SharingService) *Service {
	s := &Service{repo: repo}
	if len(sharing) > 0 {
		s.sharing = sharing[0]
	}
	return s
}

// SetDeploymentPublisher configures async deployment event publishing (optional).
func (s *Service) SetDeploymentPublisher(publisher DeploymentPublisher) {
	s.deployPublisher = publisher
}

// Deploy initiates a model deployment.
func (s *Service) Deploy(ctx context.Context, modelID, orgID uuid.UUID, cfg DeployConfig) (*Deployment, error) {
	model, err := s.repo.GetModel(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("get model: %w", err)
	}
	if model.Status != StatusReady {
		return nil, fmt.Errorf("model is not ready for deployment (status: %s)", model.Status)
	}
	if model.OrgID != orgID {
		// Check share permissions (P1: actually implement the check)
		if s.sharing == nil {
			return nil, fmt.Errorf("model not accessible by org %s", orgID)
		}
		hasAccess, err := s.sharing.HasAccess(ctx, modelID, orgID)
		if err != nil {
			return nil, fmt.Errorf("check share permissions: %w", err)
		}
		if !hasAccess {
			return nil, fmt.Errorf("model not accessible by org %s", orgID)
		}
	}

	// Default backend to vllm if not specified
	backend := cfg.Backend
	if backend == "" {
		backend = "vllm"
	}
	deployMode := cfg.DeployMode
	if deployMode == "" {
		deployMode = "dgdr"
	}
	tp := cfg.TensorParallelSize
	if tp == 0 {
		tp = 1
	}
	pp := cfg.PipelineParallelSize
	if pp == 0 {
		pp = 1
	}
	gpuType := cfg.GPUType
	if gpuType == "" {
		gpuType = "l20"
	}
	prefillGPUType := cfg.PrefillGPUType
	if prefillGPUType == "" {
		prefillGPUType = gpuType
	}
	decodeGPUType := cfg.DecodeGPUType
	if decodeGPUType == "" {
		decodeGPUType = gpuType
	}
	prefillGPU := cfg.PrefillGPUCountPerReplica
	if prefillGPU == 0 {
		prefillGPU = cfg.GPUCountPerReplica
	}
	decodeGPU := cfg.DecodeGPUCountPerReplica
	if decodeGPU == 0 {
		decodeGPU = cfg.GPUCountPerReplica
	}
	prefillTP := cfg.PrefillTensorParallelSize
	if prefillTP == 0 {
		prefillTP = tp
	}
	decodeTP := cfg.DecodeTensorParallelSize
	if decodeTP == 0 {
		decodeTP = tp
	}
	prefillPP := cfg.PrefillPipelineParallelSize
	if prefillPP == 0 {
		prefillPP = pp
	}
	decodePP := cfg.DecodePipelineParallelSize
	if decodePP == 0 {
		decodePP = pp
	}

	d := &Deployment{
		ID:         uuid.New(),
		ModelID:    modelID,
		OrgID:      orgID,
		Name:       cfg.Name,
		Status:     DeploymentPending,
		SLATier:    cfg.SLATier,
		DeployMode: deployMode,
		// Hardware
		GPUType:            gpuType,
		GPUCountPerReplica: cfg.GPUCountPerReplica,
		NumGPUsPerNode:     cfg.NumGPUsPerNode,
		VRAMMb:             cfg.VRAMMb,
		// Scaling
		ReplicasMin: cfg.ReplicasMin,
		ReplicasMax: cfg.ReplicasMax,
		// Engine
		Backend:      backend,
		BackendImage: cfg.BackendImage,
		EnvVars:      cfg.EnvVars,
		ExtraArgs:    cfg.ExtraArgs,
		// Parallelism
		TensorParallelSize:   tp,
		PipelineParallelSize: pp,
		// Workload
		InputSequenceLength:  cfg.InputSequenceLength,
		OutputSequenceLength: cfg.OutputSequenceLength,
		// SLA
		TargetTTFTMs: cfg.TargetTTFTMs,
		TargetITLMs:  cfg.TargetITLMs,
		TargetTPOTMs: cfg.TargetTPOTMs,
		// Disaggregated
		DisaggEnabled:               cfg.DisaggEnabled,
		PrefillReplicas:             cfg.PrefillReplicas,
		DecodeReplicas:              cfg.DecodeReplicas,
		PrefillGPUType:              prefillGPUType,
		DecodeGPUType:               decodeGPUType,
		PrefillGPUCountPerReplica:   prefillGPU,
		DecodeGPUCountPerReplica:    decodeGPU,
		PrefillTensorParallelSize:   prefillTP,
		DecodeTensorParallelSize:    decodeTP,
		PrefillPipelineParallelSize: prefillPP,
		DecodePipelineParallelSize:  decodePP,
		PrefillBackendImage:         cfg.PrefillBackendImage,
		DecodeBackendImage:          cfg.DecodeBackendImage,
		PrefillExtraArgs:            cfg.PrefillExtraArgs,
		DecodeExtraArgs:             cfg.DecodeExtraArgs,
		SearchStrategy:              cfg.SearchStrategy,
		// DGD-specific
		FrontendReplicas: cfg.FrontendReplicas,
		WorkerCommand:    cfg.WorkerCommand,
		DynamoNS:         cfg.DynamoNamespace,
		RouterMode:       cfg.RouterMode,
		// Advanced
		MaxBatchSize:      cfg.MaxBatchSize,
		MaxSequenceLength: cfg.MaxSequenceLength,
		Dtype:             cfg.Dtype,
		// Timestamps
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.repo.CreateDeployment(ctx, d); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}
	if s.deployPublisher != nil {
		if err := s.deployPublisher.PublishRequested(ctx, model, d, cfg); err != nil {
			return nil, fmt.Errorf("publish deployment request: %w", err)
		}
	}
	return d, nil
}

// CompleteSimulatedDeployment marks a deployment as running and sets its endpoint.
func (s *Service) CompleteSimulatedDeployment(ctx context.Context, deploymentID uuid.UUID, replicas int, endpointURL string) error {
	return s.repo.UpdateDeploymentStatus(ctx, deploymentID, DeploymentRunning, endpointURL, "")
}

// MarkDeploymentFailed marks a deployment as failed and stores an error message.
func (s *Service) MarkDeploymentFailed(ctx context.Context, deploymentID uuid.UUID, errorMessage string) error {
	return s.repo.UpdateDeploymentStatus(ctx, deploymentID, DeploymentFailed, "", errorMessage)
}

// DeleteDeployment stops a deployment and requests backend cleanup.
func (s *Service) DeleteDeployment(ctx context.Context, modelID, deploymentID, orgID uuid.UUID) error {
	d, err := s.repo.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if d == nil || d.ModelID != modelID || d.OrgID != orgID {
		return fmt.Errorf("deployment not found")
	}
	if s.deployPublisher != nil {
		if err := s.deployPublisher.PublishDeleteRequested(ctx, d); err != nil {
			return fmt.Errorf("publish deployment delete request: %w", err)
		}
	}
	if err := s.repo.StopDeployment(ctx, deploymentID); err != nil {
		return fmt.Errorf("mark deployment stopped: %w", err)
	}
	return nil
}

// DeployConfig holds parameters for a new model deployment.
// These map to NVIDIA Dynamo's CRDs:
//   - deploy_mode "dgdr" → DynamoGraphDeploymentRequest (auto-profiling, SLA-driven)
//   - deploy_mode "dgd"  → DynamoGraphDeployment (direct deploy, explicit config)
type DeployConfig struct {
	Name       string
	SLATier    SLATier
	DeployMode string // "dgdr" (auto-profile) or "dgd" (direct deploy)

	// Hardware
	GPUType            string
	GPUCountPerReplica int
	NumGPUsPerNode     int
	VRAMMb             int

	// Scaling
	ReplicasMin        int
	ReplicasMax        int
	AutoscalingEnabled *bool

	// Inference engine
	Backend      string // vllm, sglang, trtllm
	BackendImage string

	// Parallelism
	TensorParallelSize   int
	PipelineParallelSize int

	// Workload profile (DGDR only)
	InputSequenceLength  int
	OutputSequenceLength int

	// SLA targets (DGDR only)
	TargetTTFTMs float64
	TargetITLMs  float64
	TargetTPOTMs float64

	// Disaggregated serving
	DisaggEnabled               bool
	PrefillReplicas             int
	DecodeReplicas              int
	PrefillGPUType              string
	DecodeGPUType               string
	PrefillGPUCountPerReplica   int
	DecodeGPUCountPerReplica    int
	PrefillTensorParallelSize   int
	DecodeTensorParallelSize    int
	PrefillPipelineParallelSize int
	DecodePipelineParallelSize  int
	PrefillBackendImage         string
	DecodeBackendImage          string
	SearchStrategy              string // AIConfigurator: rapid or thorough

	// DGD-specific (direct deploy)
	FrontendReplicas int               // Frontend HTTP replicas
	WorkerCommand    string            // Custom worker cmd override
	DynamoNamespace  string            // Dynamo service discovery namespace
	RouterMode       string            // "random" or "kv"
	EnvVars          map[string]string // Extra env vars for workers

	// Advanced
	MaxBatchSize      int
	MaxSequenceLength int
	Dtype             string
	AutoApply         *bool
	ExtraArgs         map[string]string
	PrefillExtraArgs  map[string]string
	DecodeExtraArgs   map[string]string
}
