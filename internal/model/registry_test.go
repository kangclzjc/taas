package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// mockRepo implements Repository for testing.
type mockRepo struct {
	models      map[uuid.UUID]*Model
	deployments map[uuid.UUID]*Deployment
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		models:      make(map[uuid.UUID]*Model),
		deployments: make(map[uuid.UUID]*Deployment),
	}
}

func (m *mockRepo) CreateModel(_ context.Context, model *Model) error {
	m.models[model.ID] = model
	return nil
}
func (m *mockRepo) GetModel(_ context.Context, id uuid.UUID) (*Model, error) {
	model, ok := m.models[id]
	if !ok {
		return nil, nil
	}
	return model, nil
}
func (m *mockRepo) GetModelBySlug(_ context.Context, _ uuid.UUID, _ string) (*Model, error) {
	return nil, nil
}
func (m *mockRepo) ListModels(_ context.Context, _ ListModelsFilter) ([]*Model, int, error) {
	return nil, 0, nil
}
func (m *mockRepo) UpdateModel(_ context.Context, _ *Model) error    { return nil }
func (m *mockRepo) DeleteModel(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockRepo) CreateDeployment(_ context.Context, d *Deployment) error {
	m.deployments[d.ID] = d
	return nil
}
func (m *mockRepo) GetDeployment(_ context.Context, id uuid.UUID) (*Deployment, error) {
	d, ok := m.deployments[id]
	if !ok {
		return nil, nil
	}
	return d, nil
}
func (m *mockRepo) GetActiveDeployment(_ context.Context, _ uuid.UUID) (*Deployment, error) {
	return nil, nil
}
func (m *mockRepo) UpdateDeploymentStatus(_ context.Context, _ uuid.UUID, _ DeploymentStatus, _, _ string) error {
	return nil
}
func (m *mockRepo) StopDeployment(_ context.Context, id uuid.UUID) error {
	if d, ok := m.deployments[id]; ok {
		d.Status = DeploymentStopped
		return nil
	}
	return fmt.Errorf("deployment not found")
}
func (m *mockRepo) ListDeployments(_ context.Context, _ uuid.UUID) ([]*Deployment, error) {
	return nil, nil
}

func (m *mockRepo) SetDeploymentLiteLLMID(_ context.Context, id uuid.UUID, litellmModelID string) error {
	if d, ok := m.deployments[id]; ok {
		d.LiteLLMModelID = litellmModelID
		return nil
	}
	return fmt.Errorf("deployment not found")
}

func TestDeploy_Success(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	orgID := uuid.New()
	modelID := uuid.New()
	repo.models[modelID] = &Model{
		ID:     modelID,
		OrgID:  orgID,
		Status: StatusReady,
	}

	d, err := svc.Deploy(context.Background(), modelID, orgID, DeployConfig{
		Name:        "test-deploy",
		SLATier:     SLAStandard,
		ReplicasMin: 1,
		ReplicasMax: 2,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if d.Status != DeploymentPending {
		t.Errorf("expected pending status, got %s", d.Status)
	}
	if d.ModelID != modelID {
		t.Errorf("expected model ID %s, got %s", modelID, d.ModelID)
	}
}

func TestDeploy_ComponentGPUTypeDefaultsAndOverrides(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	orgID := uuid.New()
	modelID := uuid.New()
	repo.models[modelID] = &Model{
		ID:     modelID,
		OrgID:  orgID,
		Status: StatusReady,
	}

	d, err := svc.Deploy(context.Background(), modelID, orgID, DeployConfig{
		Name:           "hetero-deploy",
		GPUType:        "l20",
		PrefillGPUType: "gb200",
		DecodeGPUType:  "h20",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if d.GPUType != "l20" {
		t.Fatalf("expected global gpu type l20, got %q", d.GPUType)
	}
	if d.PrefillGPUType != "gb200" {
		t.Fatalf("expected prefill gpu type gb200, got %q", d.PrefillGPUType)
	}
	if d.DecodeGPUType != "h20" {
		t.Fatalf("expected decode gpu type h20, got %q", d.DecodeGPUType)
	}

	d, err = svc.Deploy(context.Background(), modelID, orgID, DeployConfig{
		Name:    "default-deploy",
		GPUType: "h100_sxm",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if d.PrefillGPUType != "h100_sxm" || d.DecodeGPUType != "h100_sxm" {
		t.Fatalf("expected component gpu types to inherit h100_sxm, got prefill=%q decode=%q", d.PrefillGPUType, d.DecodeGPUType)
	}
}

func TestDeploy_ComponentRuntimeConfigPersisted(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	orgID := uuid.New()
	modelID := uuid.New()
	repo.models[modelID] = &Model{
		ID:     modelID,
		OrgID:  orgID,
		Status: StatusReady,
	}

	d, err := svc.Deploy(context.Background(), modelID, orgID, DeployConfig{
		Name:                "runtime-config",
		PrefillBackendImage: "nvcr.io/example/prefill:latest",
		DecodeBackendImage:  "nvcr.io/example/decode:latest",
		PrefillExtraArgs: map[string]string{
			"--prefill-only": "true",
		},
		DecodeExtraArgs: map[string]string{
			"--decode-only": "true",
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if d.PrefillBackendImage != "nvcr.io/example/prefill:latest" {
		t.Fatalf("expected prefill image to persist, got %q", d.PrefillBackendImage)
	}
	if d.DecodeBackendImage != "nvcr.io/example/decode:latest" {
		t.Fatalf("expected decode image to persist, got %q", d.DecodeBackendImage)
	}
	if d.PrefillExtraArgs["--prefill-only"] != "true" {
		t.Fatalf("expected prefill extra args to persist, got %#v", d.PrefillExtraArgs)
	}
	if d.DecodeExtraArgs["--decode-only"] != "true" {
		t.Fatalf("expected decode extra args to persist, got %#v", d.DecodeExtraArgs)
	}
}

func TestSetDeploymentExtraArgs(t *testing.T) {
	d := &Deployment{}

	err := setDeploymentExtraArgs(
		d,
		[]byte(`{"--prefill-only":"true"}`),
		[]byte(`{"--decode-only":"true"}`),
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if d.PrefillExtraArgs["--prefill-only"] != "true" {
		t.Fatalf("expected prefill extra args to decode, got %#v", d.PrefillExtraArgs)
	}
	if d.DecodeExtraArgs["--decode-only"] != "true" {
		t.Fatalf("expected decode extra args to decode, got %#v", d.DecodeExtraArgs)
	}
}

func TestDeploy_NotReady(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	orgID := uuid.New()
	modelID := uuid.New()
	repo.models[modelID] = &Model{
		ID:     modelID,
		OrgID:  orgID,
		Status: StatusUploading,
	}

	_, err := svc.Deploy(context.Background(), modelID, orgID, DeployConfig{Name: "test"})
	if err == nil {
		t.Fatal("expected error for non-ready model")
	}
}

func TestDeploy_WrongOrg(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	ownerOrg := uuid.New()
	otherOrg := uuid.New()
	modelID := uuid.New()
	repo.models[modelID] = &Model{
		ID:     modelID,
		OrgID:  ownerOrg,
		Status: StatusReady,
	}

	_, err := svc.Deploy(context.Background(), modelID, otherOrg, DeployConfig{Name: "test"})
	if err == nil {
		t.Fatal("expected error for wrong org")
	}
	fmt.Println("correctly rejected:", err)
}
