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
func (m *mockRepo) UpdateModel(_ context.Context, _ *Model) error { return nil }
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
