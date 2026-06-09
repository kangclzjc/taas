-- Persist Dynamo deployment configuration so the UI and cleanup paths can
-- reflect Direct Deploy (DGD), backend, topology, and autoscaling settings.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS deploy_mode TEXT NOT NULL DEFAULT 'dgdr';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS num_gpus_per_node INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS vram_mb INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS backend TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS backend_image TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS tensor_parallel_size INT NOT NULL DEFAULT 1;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS pipeline_parallel_size INT NOT NULL DEFAULT 1;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS input_sequence_length INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS output_sequence_length INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS target_ttft_ms DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS target_itl_ms DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS target_tpot_ms DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS disagg_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS prefill_replicas INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS decode_replicas INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS search_strategy TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS frontend_replicas INT NOT NULL DEFAULT 1;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS worker_command TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS dynamo_ns TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS router_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS dtype TEXT NOT NULL DEFAULT '';

UPDATE deployments
SET deploy_mode = 'dgd',
    backend = CASE WHEN backend = '' THEN 'vllm' ELSE backend END,
    tensor_parallel_size = CASE WHEN tensor_parallel_size = 0 THEN 1 ELSE tensor_parallel_size END,
    pipeline_parallel_size = CASE WHEN pipeline_parallel_size = 0 THEN 1 ELSE pipeline_parallel_size END,
    disagg_enabled = TRUE,
    prefill_replicas = CASE WHEN prefill_replicas = 0 THEN 1 ELSE prefill_replicas END,
    decode_replicas = CASE WHEN decode_replicas = 0 THEN 1 ELSE decode_replicas END
WHERE deploy_mode = 'dgdr'
  AND endpoint_url LIKE '%dgd-%';
