package model

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// DeploymentPublisher emits deployment lifecycle requests to async workers/operators.
type DeploymentPublisher interface {
	PublishRequested(ctx context.Context, model *Model, d *Deployment, cfg DeployConfig) error
}

// NATSDeploymentPublisher publishes deployment requests to JetStream.
type NATSDeploymentPublisher struct {
	js     nats.JetStreamContext
	logger *zap.Logger
}

func NewNATSDeploymentPublisher(js nats.JetStreamContext, logger *zap.Logger) *NATSDeploymentPublisher {
	return &NATSDeploymentPublisher{js: js, logger: logger}
}

func (p *NATSDeploymentPublisher) PublishRequested(ctx context.Context, model *Model, d *Deployment, cfg DeployConfig) error {
	payload := map[string]any{
		"deployment_id": d.ID.String(),
		"model_id":      d.ModelID.String(),
		"org_id":        d.OrgID.String(),
		"model_name":    model.Slug,
		"storage_uri":   model.StorageURI,

		"deploy_mode": d.DeployMode,
		"sla_tier":    d.SLATier,

		"gpu_type":               d.GPUType,
		"gpu_count_per_replica":  d.GPUCountPerReplica,
		"num_gpus_per_node":      d.NumGPUsPerNode,
		"vram_mb":                d.VRAMMb,
		"replicas_min":           d.ReplicasMin,
		"replicas_max":           d.ReplicasMax,
		"backend":                d.Backend,
		"backend_image":          d.BackendImage,
		"tensor_parallel_size":   d.TensorParallelSize,
		"pipeline_parallel_size": d.PipelineParallelSize,
		"input_sequence_length":  d.InputSequenceLength,
		"output_sequence_length": d.OutputSequenceLength,
		"target_ttft_ms":         d.TargetTTFTMs,
		"target_itl_ms":          d.TargetITLMs,
		"target_tpot_ms":         d.TargetTPOTMs,
		"disagg_enabled":         d.DisaggEnabled,
		"prefill_replicas":       d.PrefillReplicas,
		"decode_replicas":        d.DecodeReplicas,
		"search_strategy":        d.SearchStrategy,
		"frontend_replicas":      d.FrontendReplicas,
		"worker_command":         d.WorkerCommand,
		"dynamo_namespace":       d.DynamoNS,
		"router_mode":            d.RouterMode,
		"max_batch_size":         d.MaxBatchSize,
		"max_sequence_length":    d.MaxSequenceLength,
		"dtype":                  d.Dtype,

		"env_vars":   cfg.EnvVars,
		"extra_args": cfg.ExtraArgs,
	}
	if cfg.AutoApply != nil {
		payload["auto_apply"] = *cfg.AutoApply
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal deployment payload: %w", err)
	}

	if _, err := p.js.Publish("model.deploy.requested", data); err != nil {
		return fmt.Errorf("publish model.deploy.requested: %w", err)
	}

	if p.logger != nil {
		p.logger.Info("published model.deploy.requested",
			zap.String("deployment_id", d.ID.String()),
			zap.String("model_id", d.ModelID.String()),
			zap.String("deploy_mode", d.DeployMode),
		)
	}
	return nil
}
