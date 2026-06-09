package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGRepository implements Repository using PostgreSQL.
type PGRepository struct {
	db *pgxpool.Pool
}

func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{db: db}
}

func (r *PGRepository) CreateModel(ctx context.Context, m *Model) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO models (id, org_id, owner_user_id, name, slug, description, framework, format,
		 storage_uri, storage_size_bytes, parameter_count, context_length, is_public, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		m.ID, m.OrgID, m.OwnerUserID, m.Name, m.Slug, m.Description, m.Framework, m.Format,
		m.StorageURI, m.StorageBytes, m.ParameterCount, m.ContextLength, m.IsPublic, m.Status,
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *PGRepository) GetModel(ctx context.Context, id uuid.UUID) (*Model, error) {
	m := &Model{}
	err := r.db.QueryRow(ctx,
		`SELECT id, org_id, owner_user_id, name, slug, description, framework, format,
		 storage_uri, storage_size_bytes, parameter_count, context_length, is_public, status,
		 created_at, updated_at
		 FROM models WHERE id = $1`, id,
	).Scan(&m.ID, &m.OrgID, &m.OwnerUserID, &m.Name, &m.Slug, &m.Description, &m.Framework, &m.Format,
		&m.StorageURI, &m.StorageBytes, &m.ParameterCount, &m.ContextLength, &m.IsPublic, &m.Status,
		&m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (r *PGRepository) GetModelBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Model, error) {
	m := &Model{}
	err := r.db.QueryRow(ctx,
		`SELECT id, org_id, owner_user_id, name, slug, description, framework, format,
		 storage_uri, storage_size_bytes, parameter_count, context_length, is_public, status,
		 created_at, updated_at
		 FROM models WHERE org_id = $1 AND slug = $2`, orgID, slug,
	).Scan(&m.ID, &m.OrgID, &m.OwnerUserID, &m.Name, &m.Slug, &m.Description, &m.Framework, &m.Format,
		&m.StorageURI, &m.StorageBytes, &m.ParameterCount, &m.ContextLength, &m.IsPublic, &m.Status,
		&m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (r *PGRepository) ListModels(ctx context.Context, filter ListModelsFilter) ([]*Model, int, error) {
	// Use window function to get total count in a single query (P2: N+1 optimization)
	query := `SELECT id, org_id, owner_user_id, name, slug, description, framework, format,
		 storage_uri, storage_size_bytes, parameter_count, context_length, is_public, status,
		 created_at, updated_at, COUNT(*) OVER() AS total_count FROM models WHERE 1=1`
	args := []any{}
	argIdx := 1

	addFilter := func(clause string, val any) {
		query += fmt.Sprintf(" AND %s $%d", clause, argIdx)
		args = append(args, val)
		argIdx++
	}

	if filter.OrgID != nil {
		addFilter("org_id =", *filter.OrgID)
	}
	if filter.IsPublic != nil {
		addFilter("is_public =", *filter.IsPublic)
	}
	if filter.Framework != nil {
		addFilter("framework =", string(*filter.Framework))
	}
	if filter.Status != nil {
		addFilter("status =", string(*filter.Status))
	}

	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var models []*Model
	var total int
	for rows.Next() {
		m := &Model{}
		if err := rows.Scan(&m.ID, &m.OrgID, &m.OwnerUserID, &m.Name, &m.Slug, &m.Description, &m.Framework, &m.Format,
			&m.StorageURI, &m.StorageBytes, &m.ParameterCount, &m.ContextLength, &m.IsPublic, &m.Status,
			&m.CreatedAt, &m.UpdatedAt, &total); err != nil {
			return nil, 0, err
		}
		models = append(models, m)
	}
	return models, total, rows.Err()
}

func (r *PGRepository) UpdateModel(ctx context.Context, m *Model) error {
	_, err := r.db.Exec(ctx,
		`UPDATE models SET name=$1, slug=$2, description=$3, is_public=$4, status=$5, updated_at=NOW()
		 WHERE id=$6`,
		m.Name, m.Slug, m.Description, m.IsPublic, m.Status, m.ID)
	return err
}

func (r *PGRepository) DeleteModel(ctx context.Context, id uuid.UUID) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM models WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("model not found")
	}
	return nil
}

func (r *PGRepository) CreateDeployment(ctx context.Context, d *Deployment) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO deployments (id, model_id, org_id, name, status, sla_tier, deploy_mode, replicas_min, replicas_max,
		 replicas_current, gpu_type, gpu_count_per_replica, num_gpus_per_node, vram_mb, backend, backend_image,
		 tensor_parallel_size, pipeline_parallel_size, input_sequence_length, output_sequence_length,
		 target_ttft_ms, target_itl_ms, target_tpot_ms, disagg_enabled, prefill_replicas, decode_replicas,
		 search_strategy, frontend_replicas, worker_command, dynamo_ns, router_mode, max_batch_size, max_sequence_length, dtype,
		 dynamo_service_name, dynamo_namespace, endpoint_url, error_message, deployed_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
		         $21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41)`,
		d.ID, d.ModelID, d.OrgID, d.Name, d.Status, d.SLATier, d.DeployMode, d.ReplicasMin, d.ReplicasMax,
		d.ReplicasCurrent, d.GPUType, d.GPUCountPerReplica, d.NumGPUsPerNode, d.VRAMMb, d.Backend, d.BackendImage,
		d.TensorParallelSize, d.PipelineParallelSize, d.InputSequenceLength, d.OutputSequenceLength,
		d.TargetTTFTMs, d.TargetITLMs, d.TargetTPOTMs, d.DisaggEnabled, d.PrefillReplicas, d.DecodeReplicas,
		d.SearchStrategy, d.FrontendReplicas, d.WorkerCommand, d.DynamoNS, d.RouterMode, d.MaxBatchSize, d.MaxSequenceLength, d.Dtype,
		d.DynamoServiceName, d.DynamoNamespace, d.EndpointURL, d.ErrorMessage, d.DeployedAt,
		d.CreatedAt, d.UpdatedAt)
	return err
}

func (r *PGRepository) GetDeployment(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	d := &Deployment{}
	err := r.db.QueryRow(ctx,
		`SELECT id, model_id, org_id, name, status, sla_tier, deploy_mode, replicas_min, replicas_max,
		 replicas_current, gpu_type, gpu_count_per_replica, num_gpus_per_node, vram_mb, backend, backend_image,
		 tensor_parallel_size, pipeline_parallel_size, input_sequence_length, output_sequence_length,
		 target_ttft_ms, target_itl_ms, target_tpot_ms, disagg_enabled, prefill_replicas, decode_replicas,
		 search_strategy, frontend_replicas, worker_command, dynamo_ns, router_mode, max_batch_size, max_sequence_length, dtype,
		 dynamo_service_name, dynamo_namespace, endpoint_url, litellm_model_id, error_message, deployed_at, created_at, updated_at
		 FROM deployments WHERE id = $1`, id,
	).Scan(&d.ID, &d.ModelID, &d.OrgID, &d.Name, &d.Status, &d.SLATier, &d.DeployMode, &d.ReplicasMin, &d.ReplicasMax,
		&d.ReplicasCurrent, &d.GPUType, &d.GPUCountPerReplica, &d.NumGPUsPerNode, &d.VRAMMb, &d.Backend, &d.BackendImage,
		&d.TensorParallelSize, &d.PipelineParallelSize, &d.InputSequenceLength, &d.OutputSequenceLength,
		&d.TargetTTFTMs, &d.TargetITLMs, &d.TargetTPOTMs, &d.DisaggEnabled, &d.PrefillReplicas, &d.DecodeReplicas,
		&d.SearchStrategy, &d.FrontendReplicas, &d.WorkerCommand, &d.DynamoNS, &d.RouterMode, &d.MaxBatchSize, &d.MaxSequenceLength, &d.Dtype,
		&d.DynamoServiceName, &d.DynamoNamespace, &d.EndpointURL, &d.LiteLLMModelID, &d.ErrorMessage, &d.DeployedAt,
		&d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

func (r *PGRepository) GetActiveDeployment(ctx context.Context, modelID uuid.UUID) (*Deployment, error) {
	d := &Deployment{}
	err := r.db.QueryRow(ctx,
		`SELECT id, model_id, org_id, name, status, sla_tier, deploy_mode, replicas_min, replicas_max,
		 replicas_current, gpu_type, gpu_count_per_replica, num_gpus_per_node, vram_mb, backend, backend_image,
		 tensor_parallel_size, pipeline_parallel_size, input_sequence_length, output_sequence_length,
		 target_ttft_ms, target_itl_ms, target_tpot_ms, disagg_enabled, prefill_replicas, decode_replicas,
		 search_strategy, frontend_replicas, worker_command, dynamo_ns, router_mode, max_batch_size, max_sequence_length, dtype,
		 dynamo_service_name, dynamo_namespace, endpoint_url, litellm_model_id, error_message, deployed_at, created_at, updated_at
		 FROM deployments WHERE model_id = $1 AND status IN ('running','deploying','pending')
		 ORDER BY created_at DESC LIMIT 1`, modelID,
	).Scan(&d.ID, &d.ModelID, &d.OrgID, &d.Name, &d.Status, &d.SLATier, &d.DeployMode, &d.ReplicasMin, &d.ReplicasMax,
		&d.ReplicasCurrent, &d.GPUType, &d.GPUCountPerReplica, &d.NumGPUsPerNode, &d.VRAMMb, &d.Backend, &d.BackendImage,
		&d.TensorParallelSize, &d.PipelineParallelSize, &d.InputSequenceLength, &d.OutputSequenceLength,
		&d.TargetTTFTMs, &d.TargetITLMs, &d.TargetTPOTMs, &d.DisaggEnabled, &d.PrefillReplicas, &d.DecodeReplicas,
		&d.SearchStrategy, &d.FrontendReplicas, &d.WorkerCommand, &d.DynamoNS, &d.RouterMode, &d.MaxBatchSize, &d.MaxSequenceLength, &d.Dtype,
		&d.DynamoServiceName, &d.DynamoNamespace, &d.EndpointURL, &d.LiteLLMModelID, &d.ErrorMessage, &d.DeployedAt,
		&d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

func (r *PGRepository) UpdateDeploymentStatus(ctx context.Context, id uuid.UUID, status DeploymentStatus, endpointURL, errorMsg string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE deployments SET status=$1, endpoint_url=$2, error_message=$3, updated_at=NOW()
		 WHERE id=$4`, status, endpointURL, errorMsg, id)
	return err
}

func (r *PGRepository) StopDeployment(ctx context.Context, id uuid.UUID) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE deployments
		 SET status=$1, endpoint_url='', error_message='', replicas_current=0, updated_at=NOW()
		 WHERE id=$2`, DeploymentStopped, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("deployment not found")
	}
	return nil
}

func (r *PGRepository) ListDeployments(ctx context.Context, modelID uuid.UUID) ([]*Deployment, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, model_id, org_id, name, status, sla_tier, deploy_mode, replicas_min, replicas_max,
		 replicas_current, gpu_type, gpu_count_per_replica, num_gpus_per_node, vram_mb, backend, backend_image,
		 tensor_parallel_size, pipeline_parallel_size, input_sequence_length, output_sequence_length,
		 target_ttft_ms, target_itl_ms, target_tpot_ms, disagg_enabled, prefill_replicas, decode_replicas,
		 search_strategy, frontend_replicas, worker_command, dynamo_ns, router_mode, max_batch_size, max_sequence_length, dtype,
		 dynamo_service_name, dynamo_namespace, endpoint_url, litellm_model_id, error_message, deployed_at, created_at, updated_at
		 FROM deployments WHERE model_id = $1 ORDER BY created_at DESC`, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []*Deployment
	for rows.Next() {
		d := &Deployment{}
		if err := rows.Scan(&d.ID, &d.ModelID, &d.OrgID, &d.Name, &d.Status, &d.SLATier, &d.DeployMode, &d.ReplicasMin, &d.ReplicasMax,
			&d.ReplicasCurrent, &d.GPUType, &d.GPUCountPerReplica, &d.NumGPUsPerNode, &d.VRAMMb, &d.Backend, &d.BackendImage,
			&d.TensorParallelSize, &d.PipelineParallelSize, &d.InputSequenceLength, &d.OutputSequenceLength,
			&d.TargetTTFTMs, &d.TargetITLMs, &d.TargetTPOTMs, &d.DisaggEnabled, &d.PrefillReplicas, &d.DecodeReplicas,
			&d.SearchStrategy, &d.FrontendReplicas, &d.WorkerCommand, &d.DynamoNS, &d.RouterMode, &d.MaxBatchSize, &d.MaxSequenceLength, &d.Dtype,
			&d.DynamoServiceName, &d.DynamoNamespace, &d.EndpointURL, &d.LiteLLMModelID, &d.ErrorMessage, &d.DeployedAt,
			&d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

// SetDeploymentLiteLLMID stores the LiteLLM model ID for a deployment (for cleanup on stop).
func (r *PGRepository) SetDeploymentLiteLLMID(ctx context.Context, id uuid.UUID, litellmModelID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE deployments SET litellm_model_id = $1, updated_at = NOW() WHERE id = $2`,
		litellmModelID, id)
	return err
}
