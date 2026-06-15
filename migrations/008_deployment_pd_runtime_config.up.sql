ALTER TABLE deployments
  ADD COLUMN IF NOT EXISTS prefill_backend_image TEXT NOT NULL DEFAULT '';

ALTER TABLE deployments
  ADD COLUMN IF NOT EXISTS decode_backend_image TEXT NOT NULL DEFAULT '';

ALTER TABLE deployments
  ADD COLUMN IF NOT EXISTS prefill_extra_args JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE deployments
  ADD COLUMN IF NOT EXISTS decode_extra_args JSONB NOT NULL DEFAULT '{}'::jsonb;
