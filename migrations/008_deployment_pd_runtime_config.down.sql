ALTER TABLE deployments
  DROP COLUMN IF EXISTS decode_extra_args;

ALTER TABLE deployments
  DROP COLUMN IF EXISTS prefill_extra_args;

ALTER TABLE deployments
  DROP COLUMN IF EXISTS decode_backend_image;

ALTER TABLE deployments
  DROP COLUMN IF EXISTS prefill_backend_image;
