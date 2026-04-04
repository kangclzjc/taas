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
	ID              uuid.UUID  `db:"id"`
	OrgID           uuid.UUID  `db:"org_id"`
	OwnerUserID     uuid.UUID  `db:"owner_user_id"`
	Name            string     `db:"name"`
	Slug            string     `db:"slug"`
	Description     string     `db:"description"`
	Framework       Framework  `db:"framework"`
	Format          string     `db:"format"`
	StorageURI      string     `db:"storage_uri"`
	StorageBytes    int64      `db:"storage_size_bytes"`
	ParameterCount  int64      `db:"parameter_count"`
	ContextLength   int        `db:"context_length"`
	IsPublic        bool       `db:"is_public"`
	Status          Status     `db:"status"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
}

// Deployment represents a running model deployment.
type Deployment struct {
	ID                  uuid.UUID        `db:"id"`
	ModelID             uuid.UUID        `db:"model_id"`
	OrgID               uuid.UUID        `db:"org_id"`
	Name                string           `db:"name"`
	Status              DeploymentStatus `db:"status"`
	SLATier             SLATier          `db:"sla_tier"`
	ReplicasMin         int              `db:"replicas_min"`
	ReplicasMax         int              `db:"replicas_max"`
	ReplicasCurrent     int              `db:"replicas_current"`
	GPUType             string           `db:"gpu_type"`
	GPUCountPerReplica  int              `db:"gpu_count_per_replica"`
	MaxBatchSize        int              `db:"max_batch_size"`
	MaxSequenceLength   int              `db:"max_sequence_length"`
	DynamoServiceName   string           `db:"dynamo_service_name"`
	DynamoNamespace     string           `db:"dynamo_namespace"`
	EndpointURL         string           `db:"endpoint_url"`
	ErrorMessage        string           `db:"error_message"`
	DeployedAt          *time.Time       `db:"deployed_at"`
	CreatedAt           time.Time        `db:"created_at"`
	UpdatedAt           time.Time        `db:"updated_at"`
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
	ListDeployments(ctx context.Context, modelID uuid.UUID) ([]*Deployment, error)
}

// ListModelsFilter specifies query criteria for model listing.
type ListModelsFilter struct {
	OrgID      *uuid.UUID
	IsPublic   *bool
	Framework  *Framework
	Status     *Status
	Limit      int
	Offset     int
}

// Service provides business logic for model management.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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
		// Check share permissions
		return nil, fmt.Errorf("model not accessible by org %s", orgID)
	}

	d := &Deployment{
		ID:                uuid.New(),
		ModelID:           modelID,
		OrgID:             orgID,
		Name:              cfg.Name,
		Status:            DeploymentPending,
		SLATier:           cfg.SLATier,
		ReplicasMin:       cfg.ReplicasMin,
		ReplicasMax:       cfg.ReplicasMax,
		GPUType:           cfg.GPUType,
		GPUCountPerReplica: cfg.GPUCountPerReplica,
		MaxBatchSize:      cfg.MaxBatchSize,
		MaxSequenceLength: cfg.MaxSequenceLength,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := s.repo.CreateDeployment(ctx, d); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}
	return d, nil
}

// DeployConfig holds parameters for a new model deployment.
type DeployConfig struct {
	Name               string
	SLATier            SLATier
	ReplicasMin        int
	ReplicasMax        int
	GPUType            string
	GPUCountPerReplica int
	MaxBatchSize       int
	MaxSequenceLength  int
}
