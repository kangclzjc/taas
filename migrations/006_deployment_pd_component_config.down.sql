ALTER TABLE deployments DROP COLUMN IF EXISTS decode_pipeline_parallel_size;
ALTER TABLE deployments DROP COLUMN IF EXISTS prefill_pipeline_parallel_size;
ALTER TABLE deployments DROP COLUMN IF EXISTS decode_tensor_parallel_size;
ALTER TABLE deployments DROP COLUMN IF EXISTS prefill_tensor_parallel_size;
ALTER TABLE deployments DROP COLUMN IF EXISTS decode_gpu_count_per_replica;
ALTER TABLE deployments DROP COLUMN IF EXISTS prefill_gpu_count_per_replica;
