-- Revert LiteLLM integration column
DROP INDEX IF EXISTS idx_api_tokens_litellm_key;
ALTER TABLE api_tokens DROP COLUMN IF EXISTS litellm_key_token;
