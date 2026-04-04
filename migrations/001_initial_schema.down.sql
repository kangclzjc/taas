-- 001_initial_schema.down.sql
-- Rollback: drop all TaaS tables in reverse dependency order

BEGIN;

DROP TABLE IF EXISTS usage_records;
DROP TABLE IF EXISTS model_shares;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS models;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS users;

COMMIT;
