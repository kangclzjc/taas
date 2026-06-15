-- Persist per-component P/D hardware and parallelism so the UI can display
-- the fixed profiled serving plan that was actually sent to the Dynamo operator.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS prefill_gpu_count_per_replica INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS decode_gpu_count_per_replica INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS prefill_tensor_parallel_size INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS decode_tensor_parallel_size INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS prefill_pipeline_parallel_size INT NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS decode_pipeline_parallel_size INT NOT NULL DEFAULT 0;

UPDATE deployments
SET prefill_gpu_count_per_replica = CASE
        WHEN prefill_gpu_count_per_replica = 0 THEN gpu_count_per_replica
        ELSE prefill_gpu_count_per_replica
    END,
    decode_gpu_count_per_replica = CASE
        WHEN decode_gpu_count_per_replica = 0 THEN gpu_count_per_replica
        ELSE decode_gpu_count_per_replica
    END,
    prefill_tensor_parallel_size = CASE
        WHEN prefill_tensor_parallel_size = 0 THEN tensor_parallel_size
        ELSE prefill_tensor_parallel_size
    END,
    decode_tensor_parallel_size = CASE
        WHEN decode_tensor_parallel_size = 0 THEN tensor_parallel_size
        ELSE decode_tensor_parallel_size
    END,
    prefill_pipeline_parallel_size = CASE
        WHEN prefill_pipeline_parallel_size = 0 THEN pipeline_parallel_size
        ELSE prefill_pipeline_parallel_size
    END,
    decode_pipeline_parallel_size = CASE
        WHEN decode_pipeline_parallel_size = 0 THEN pipeline_parallel_size
        ELSE decode_pipeline_parallel_size
    END
WHERE disagg_enabled = TRUE;
