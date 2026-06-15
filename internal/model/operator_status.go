package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DeploymentRuntimeStatusProvider reads live runtime state for a deployment.
type DeploymentRuntimeStatusProvider interface {
	GetDeploymentRuntimeStatus(ctx context.Context, deploymentID uuid.UUID) (*DeploymentRuntimeStatus, error)
}

// DeploymentRuntimeStatus mirrors the read-only Kubernetes status summary exposed by the Dynamo operator.
type DeploymentRuntimeStatus struct {
	Available          bool                         `json:"available"`
	Found              bool                         `json:"found"`
	Namespace          string                       `json:"namespace,omitempty"`
	DGDName            string                       `json:"dgd_name,omitempty"`
	Generation         int64                        `json:"generation,omitempty"`
	ObservedGeneration int64                        `json:"observed_generation,omitempty"`
	Ready              bool                         `json:"ready"`
	State              string                       `json:"state,omitempty"`
	ReadyReason        string                       `json:"ready_reason,omitempty"`
	ReadyMessage       string                       `json:"ready_message,omitempty"`
	Services           map[string]DGDServiceRuntime `json:"services,omitempty"`
	ProfileConfigMaps  []string                     `json:"profile_config_maps,omitempty"`
	Error              string                       `json:"error,omitempty"`
}

// DGDServiceRuntime captures status.services.<component> from a DynamoGraphDeployment.
type DGDServiceRuntime struct {
	ComponentKind   string   `json:"component_kind,omitempty"`
	ComponentName   string   `json:"component_name,omitempty"`
	ComponentNames  []string `json:"component_names,omitempty"`
	Replicas        int      `json:"replicas"`
	ReadyReplicas   int      `json:"ready_replicas"`
	UpdatedReplicas int      `json:"updated_replicas"`
}

// OperatorStatusClient proxies runtime-status requests to the Python Dynamo operator.
type OperatorStatusClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewOperatorStatusClient(baseURL string) *OperatorStatusClient {
	return &OperatorStatusClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

func (c *OperatorStatusClient) GetDeploymentRuntimeStatus(ctx context.Context, deploymentID uuid.UUID) (*DeploymentRuntimeStatus, error) {
	if c == nil || c.baseURL == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/deployments/%s/k8s-status", c.baseURL, deploymentID.String()),
		nil,
	)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("operator status endpoint returned HTTP %d", resp.StatusCode)
	}
	var status DeploymentRuntimeStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}
