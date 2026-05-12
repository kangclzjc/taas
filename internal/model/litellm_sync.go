package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/litellm"
)

// defaultUpstreamModel mirrors python/dynamo_operator settings.nvidia_hf_model_default.
// It is the model that vLLM frontend serves when the model record has no explicit storage URI.
const defaultUpstreamModel = "Qwen/Qwen3-0.6B"

// upstreamModelName resolves the model identifier that the inference backend (vLLM/SGLang/...) actually
// serves under, mirroring python/dynamo_operator/k8s_nvidia_dgd.py:_hf_model. Plain HuggingFace ids look
// like "org/name"; URI-style storage (s3://, gs://, nim://) cannot be served as-is, so we fall back to
// the platform default to avoid 404s from the upstream when LiteLLM forwards the request.
func upstreamModelName(m *Model) string {
	uri := strings.TrimSpace(m.StorageURI)
	if uri == "" {
		return defaultUpstreamModel
	}
	if strings.Contains(uri, "://") {
		return defaultUpstreamModel
	}
	if !strings.Contains(uri, "/") {
		return defaultUpstreamModel
	}
	return uri
}

// LiteLLMService wraps the base model Service and syncs deployments with LiteLLM Proxy.
//
// When a model deployment reaches "running" status with an endpoint_url,
// this service registers the endpoint in LiteLLM so clients can call it.
// When a deployment is stopped/deleted, the model is removed from LiteLLM.
//
// Architecture:
//   TaaS (Deploy model) → Dynamo Operator (create K8s CRD) → Dynamo running
//   Dynamo running → NATS event → TaaS updates deployment status + endpoint_url
//   TaaS → LiteLLM /model/new (register endpoint) → Clients can now call the model
type LiteLLMService struct {
	*Service
	litellm *litellm.AdminClient
	repo    Repository
	logger  *zap.Logger
}

// NewLiteLLMService creates a model service that syncs with LiteLLM Proxy.
func NewLiteLLMService(base *Service, litellmClient *litellm.AdminClient, repo Repository, logger *zap.Logger) *LiteLLMService {
	return &LiteLLMService{
		Service: base,
		litellm: litellmClient,
		repo:    repo,
		logger:  logger,
	}
}

// OnDeploymentRunning is called when a Dynamo deployment becomes ready.
// It registers the Dynamo endpoint in LiteLLM Proxy as a model.
//
// Parameters:
//   - deploymentID: the TaaS deployment UUID
//   - endpointURL: the Dynamo Frontend HTTP endpoint (e.g., http://dynamo-svc:8000/v1)
func (s *LiteLLMService) OnDeploymentRunning(ctx context.Context, deploymentID uuid.UUID, endpointURL string) error {
	d, err := s.repo.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if d == nil {
		return fmt.Errorf("deployment %s not found", deploymentID)
	}

	m, err := s.repo.GetModel(ctx, d.ModelID)
	if err != nil {
		return fmt.Errorf("get model: %w", err)
	}
	if m == nil {
		return fmt.Errorf("model %s not found", d.ModelID)
	}

	upstream := upstreamModelName(m)
	req := litellm.AddModelRequest{
		ModelName: m.Slug,
		LiteLLMParams: litellm.ModelParams{
			Model:   "openai/" + upstream,
			APIBase: endpointURL,
			APIKey:  "sk-no-key-required",
			ExtraHeaders: map[string]string{
				"X-Tenant-ID":     d.OrgID.String(),
				"X-Org-ID":        d.OrgID.String(),
				"X-Deployment-ID": d.ID.String(),
				"X-SLA-Tier":      string(d.SLATier),
			},
		},
		ModelInfo: &litellm.ModelInfo{
			ID:          d.ID.String(),
			Description: fmt.Sprintf("TaaS deployment: %s (%s)", d.Name, m.Name),
			MaxTokens:   m.ContextLength,
		},
	}

	resp, err := s.litellm.AddModel(ctx, req)
	if err != nil {
		return fmt.Errorf("register model in LiteLLM: %w", err)
	}

	// Store LiteLLM model ID in the deployment for later cleanup
	if err := s.repo.SetDeploymentLiteLLMID(ctx, deploymentID, resp.ModelID); err != nil {
		s.logger.Error("failed to save LiteLLM model ID",
			zap.Error(err),
			zap.String("deployment_id", deploymentID.String()),
			zap.String("litellm_model_id", resp.ModelID),
		)
	}

	s.logger.Info("model registered in LiteLLM",
		zap.String("model_slug", m.Slug),
		zap.String("upstream_model", upstream),
		zap.String("endpoint", endpointURL),
		zap.String("litellm_model_id", resp.ModelID),
		zap.String("deployment_id", deploymentID.String()),
	)

	return nil
}

// OnDeploymentStopped is called when a Dynamo deployment is stopped or deleted.
// It removes the model from LiteLLM Proxy.
func (s *LiteLLMService) OnDeploymentStopped(ctx context.Context, deploymentID uuid.UUID) error {
	d, err := s.repo.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if d == nil {
		return fmt.Errorf("deployment %s not found", deploymentID)
	}

	if d.LiteLLMModelID == "" {
		s.logger.Warn("deployment has no LiteLLM model ID, skipping removal",
			zap.String("deployment_id", deploymentID.String()),
		)
		return nil
	}

	if err := s.litellm.DeleteModel(ctx, d.LiteLLMModelID); err != nil {
		return fmt.Errorf("remove model from LiteLLM: %w", err)
	}

	s.logger.Info("model removed from LiteLLM",
		zap.String("deployment_id", deploymentID.String()),
		zap.String("litellm_model_id", d.LiteLLMModelID),
	)

	return nil
}
