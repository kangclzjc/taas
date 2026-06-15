-- 001_initial_schema.up.sql
-- TaaS initial database schema
-- Generated from Go source: internal/auth, internal/token, internal/model, internal/billing

BEGIN;

-- ─── Extensions ────────────────────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── users ─────────────────────────────────────────────────────────────────────
-- Source: internal/auth/repository.go → User struct
CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id        UUID        NOT NULL,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'member',
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_org_id ON users (org_id);
CREATE INDEX idx_users_email  ON users (email);

-- ─── api_tokens ────────────────────────────────────────────────────────────────
-- Source: internal/token/repository.go → Token struct
CREATE TABLE api_tokens (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id           UUID        NOT NULL,
    name             TEXT        NOT NULL,
    token_hash       TEXT        NOT NULL UNIQUE,
    prefix           TEXT        NOT NULL,
    allowed_models   TEXT[]      DEFAULT '{}',
    scopes           TEXT[]      DEFAULT '{}',
    rate_limit_rpm   INT         NOT NULL DEFAULT 0,
    rate_limit_tpm   INT         NOT NULL DEFAULT 0,
    budget_limit_usd DECIMAL     NOT NULL DEFAULT 0,
    sla_tier         TEXT        NOT NULL DEFAULT 'standard',
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    expires_at       TIMESTAMPTZ,
    last_used_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_api_tokens_user_id    ON api_tokens (user_id);
CREATE INDEX idx_api_tokens_org_id     ON api_tokens (org_id);
CREATE INDEX idx_api_tokens_token_hash ON api_tokens (token_hash);
CREATE INDEX idx_api_tokens_prefix     ON api_tokens (prefix);

-- ─── models ────────────────────────────────────────────────────────────────────
-- Source: internal/model/repository_pg.go → Model struct, registry.go → Model struct
CREATE TABLE models (
    id                 UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id             UUID        NOT NULL,
    owner_user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name               TEXT        NOT NULL,
    slug               TEXT        NOT NULL,
    description        TEXT        NOT NULL DEFAULT '',
    framework          TEXT        NOT NULL,
    format             TEXT        NOT NULL DEFAULT '',
    storage_uri        TEXT        NOT NULL DEFAULT '',
    storage_size_bytes BIGINT      NOT NULL DEFAULT 0,
    parameter_count    BIGINT      NOT NULL DEFAULT 0,
    context_length     INT         NOT NULL DEFAULT 0,
    is_public          BOOLEAN     NOT NULL DEFAULT FALSE,
    status             TEXT        NOT NULL DEFAULT 'uploading',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_models_org_slug ON models (org_id, slug);
CREATE INDEX idx_models_org_id          ON models (org_id);
CREATE INDEX idx_models_status          ON models (status);
CREATE INDEX idx_models_framework       ON models (framework);
CREATE INDEX idx_models_is_public       ON models (is_public);

-- ─── deployments ───────────────────────────────────────────────────────────────
-- Source: internal/model/repository_pg.go → Deployment struct, registry.go → Deployment struct
CREATE TABLE deployments (
    id                     UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    model_id               UUID        NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    org_id                 UUID        NOT NULL,
    name                   TEXT        NOT NULL,
    status                 TEXT        NOT NULL DEFAULT 'pending',
    sla_tier               TEXT        NOT NULL DEFAULT 'standard',
    deploy_mode            TEXT        NOT NULL DEFAULT 'dgdr',
    replicas_min           INT         NOT NULL DEFAULT 1,
    replicas_max           INT         NOT NULL DEFAULT 1,
    replicas_current       INT         NOT NULL DEFAULT 0,
    gpu_type               TEXT        NOT NULL DEFAULT '',
    gpu_count_per_replica  INT         NOT NULL DEFAULT 1,
    num_gpus_per_node      INT         NOT NULL DEFAULT 0,
    vram_mb                INT         NOT NULL DEFAULT 0,
    backend                TEXT        NOT NULL DEFAULT '',
    backend_image          TEXT        NOT NULL DEFAULT '',
    tensor_parallel_size   INT         NOT NULL DEFAULT 1,
    pipeline_parallel_size INT         NOT NULL DEFAULT 1,
    input_sequence_length  INT         NOT NULL DEFAULT 0,
    output_sequence_length INT         NOT NULL DEFAULT 0,
    target_ttft_ms         DOUBLE PRECISION NOT NULL DEFAULT 0,
    target_itl_ms          DOUBLE PRECISION NOT NULL DEFAULT 0,
    target_tpot_ms         DOUBLE PRECISION NOT NULL DEFAULT 0,
    disagg_enabled         BOOLEAN     NOT NULL DEFAULT FALSE,
    prefill_replicas       INT         NOT NULL DEFAULT 0,
    decode_replicas        INT         NOT NULL DEFAULT 0,
    prefill_gpu_count_per_replica   INT NOT NULL DEFAULT 0,
    decode_gpu_count_per_replica    INT NOT NULL DEFAULT 0,
    prefill_tensor_parallel_size    INT NOT NULL DEFAULT 0,
    decode_tensor_parallel_size     INT NOT NULL DEFAULT 0,
    prefill_pipeline_parallel_size  INT NOT NULL DEFAULT 0,
    decode_pipeline_parallel_size   INT NOT NULL DEFAULT 0,
    prefill_backend_image           TEXT NOT NULL DEFAULT '',
    decode_backend_image            TEXT NOT NULL DEFAULT '',
    prefill_extra_args              JSONB NOT NULL DEFAULT '{}'::jsonb,
    decode_extra_args               JSONB NOT NULL DEFAULT '{}'::jsonb,
    search_strategy        TEXT        NOT NULL DEFAULT '',
    frontend_replicas      INT         NOT NULL DEFAULT 1,
    worker_command         TEXT        NOT NULL DEFAULT '',
    dynamo_ns              TEXT        NOT NULL DEFAULT '',
    router_mode            TEXT        NOT NULL DEFAULT '',
    max_batch_size         INT         NOT NULL DEFAULT 0,
    max_sequence_length    INT         NOT NULL DEFAULT 0,
    dtype                  TEXT        NOT NULL DEFAULT '',
    dynamo_service_name    TEXT        NOT NULL DEFAULT '',
    dynamo_namespace       TEXT        NOT NULL DEFAULT '',
    endpoint_url           TEXT        NOT NULL DEFAULT '',
    error_message          TEXT        NOT NULL DEFAULT '',
    deployed_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_deployments_model_id ON deployments (model_id);
CREATE INDEX idx_deployments_org_id   ON deployments (org_id);
CREATE INDEX idx_deployments_status   ON deployments (status);

-- ─── model_shares ──────────────────────────────────────────────────────────────
-- Source: internal/model/sharing.go → ModelShare struct
-- ON CONFLICT (model_id, target_org_id) DO UPDATE used in Grant()
CREATE TABLE model_shares (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    model_id      UUID        NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    owner_org_id  UUID        NOT NULL,
    target_org_id UUID        NOT NULL,
    permission    TEXT        NOT NULL DEFAULT 'read',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (model_id, target_org_id)
);

CREATE INDEX idx_model_shares_model_id      ON model_shares (model_id);
CREATE INDEX idx_model_shares_target_org_id ON model_shares (target_org_id);

-- ─── usage_records ─────────────────────────────────────────────────────────────
-- Source: internal/billing/collector.go → record() INSERT statement
-- ON CONFLICT (request_id) DO NOTHING used for deduplication
CREATE TABLE usage_records (
    request_id        TEXT        NOT NULL UNIQUE,
    token_id          TEXT        NOT NULL,
    user_id           TEXT        NOT NULL,
    org_id            TEXT        NOT NULL,
    model_id          TEXT        NOT NULL DEFAULT '',
    deployment_id     TEXT        NOT NULL DEFAULT '',
    prompt_tokens     INT         NOT NULL DEFAULT 0,
    completion_tokens INT         NOT NULL DEFAULT 0,
    total_tokens      INT         NOT NULL DEFAULT 0,
    latency_ms        INT         NOT NULL DEFAULT 0,
    ttft_ms           INT         NOT NULL DEFAULT 0,
    status            TEXT        NOT NULL DEFAULT 'success',
    error_code        TEXT        NOT NULL DEFAULT '',
    cost_usd          DECIMAL     NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_usage_records_org_id     ON usage_records (org_id);
CREATE INDEX idx_usage_records_token_id   ON usage_records (token_id);
CREATE INDEX idx_usage_records_created_at ON usage_records (created_at);
CREATE INDEX idx_usage_records_model_id   ON usage_records (model_id);

COMMIT;
