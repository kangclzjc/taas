ALTER TABLE deployments
  DROP COLUMN IF EXISTS extra_args;

ALTER TABLE deployments
  DROP COLUMN IF EXISTS env_vars;
