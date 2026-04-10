-- Add LiteLLM integration column to api_tokens table.
-- This stores the LiteLLM virtual key hash/token for correlation.
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS litellm_key_token TEXT DEFAULT '';

-- Index for looking up TaaS tokens by their LiteLLM key reference
CREATE INDEX IF NOT EXISTS idx_api_tokens_litellm_key ON api_tokens (litellm_key_token) WHERE litellm_key_token != '';
