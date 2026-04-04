#!/bin/bash
# TaaS Database Seed Script
# Usage: ./scripts/seed.sh [DATABASE_URL]

DB_URL="${1:-postgres://taas:taas@localhost:15432/taas?sslmode=disable}"

echo "Seeding TaaS database at $DB_URL"

psql "$DB_URL" <<'SQL'
-- Create test org and user (password: TestPass1@Secure)
INSERT INTO users (id, org_id, email, password_hash, role, is_active) VALUES
  ('11111111-1111-1111-1111-111111111111', 'aaaa0000-0000-0000-0000-000000000001', 'admin@taas.io', '$2a$10$...hash...', 'owner', true),
  ('22222222-2222-2222-2222-222222222222', 'aaaa0000-0000-0000-0000-000000000001', 'user@taas.io', '$2a$10$...hash...', 'member', true)
ON CONFLICT (email) DO NOTHING;

-- Create test models
INSERT INTO models (id, org_id, owner_user_id, name, slug, description, framework, format, storage_uri, parameter_count, context_length, is_public, status) VALUES
  ('m0000001-0000-0000-0000-000000000001', 'aaaa0000-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'LLaMA 3 70B', 'llama-3-70b', 'Meta LLaMA 3 70B', 'pytorch', 'safetensors', 's3://models/llama-3-70b', 70000000000, 8192, false, 'ready'),
  ('m0000002-0000-0000-0000-000000000002', 'aaaa0000-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'LLaMA 3 8B', 'llama-3-8b', 'Meta LLaMA 3 8B', 'pytorch', 'safetensors', 's3://models/llama-3-8b', 8000000000, 8192, true, 'ready'),
  ('m0000003-0000-0000-0000-000000000003', 'aaaa0000-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Mistral 7B', 'mistral-7b', 'Mistral 7B v0.3', 'pytorch', 'safetensors', 's3://models/mistral-7b', 7000000000, 32768, true, 'ready')
ON CONFLICT DO NOTHING;

SELECT 'Seed complete: ' || (SELECT count(*) FROM users) || ' users, ' || (SELECT count(*) FROM models) || ' models';
SQL

echo "Done!"
