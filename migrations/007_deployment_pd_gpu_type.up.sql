ALTER TABLE deployments ADD COLUMN IF NOT EXISTS prefill_gpu_type TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS decode_gpu_type TEXT NOT NULL DEFAULT '';

UPDATE deployments
SET prefill_gpu_type = CASE
        WHEN prefill_gpu_type = '' THEN COALESCE(NULLIF(gpu_type, ''), 'l20')
        ELSE prefill_gpu_type
    END,
    decode_gpu_type = CASE
        WHEN decode_gpu_type = '' THEN COALESCE(NULLIF(gpu_type, ''), 'l20')
        ELSE decode_gpu_type
    END
WHERE disagg_enabled = TRUE;
