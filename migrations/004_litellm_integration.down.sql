-- Revert LiteLLM integration columns
DROP INDEX IF EXISTS idx_api_tokens_litellm_key;
ALTER TABLE api_tokens DROP COLUMN IF EXISTS litellm_key_token;

DROP INDEX IF EXISTS idx_deployments_litellm_model;
ALTER TABLE deployments DROP COLUMN IF EXISTS litellm_model_id;
