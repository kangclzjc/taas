-- Add LiteLLM integration columns.

-- api_tokens: stores the LiteLLM virtual key hash/token for correlation.
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS litellm_key_token TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_api_tokens_litellm_key ON api_tokens (litellm_key_token) WHERE litellm_key_token != '';

-- deployments: stores the LiteLLM model ID for cleanup when deployment stops.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS litellm_model_id TEXT DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_deployments_litellm_model ON deployments (litellm_model_id) WHERE litellm_model_id != '';
