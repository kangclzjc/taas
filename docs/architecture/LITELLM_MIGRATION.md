# LiteLLM Migration Guide

## Overview

TaaS has migrated its API proxy layer from a custom Go implementation to **LiteLLM Proxy**, an open-source AI Gateway. This reduces maintenance burden while gaining battle-tested features for API key management, rate limiting, cost tracking, and model routing.

## Architecture Change

### Before (Legacy Mode)
```
Client → TaaS Gateway (Go)
           ├─ API Key Auth (internal/token/validator.go)
           ├─ Rate Limiting (internal/quota/)
           ├─ Proxy Handler (internal/proxy/)
           └─ → NVIDIA Dynamo (GPU inference)
```

### After (LiteLLM Mode)
```
Client → LiteLLM Proxy (:4000)
           ├─ Virtual Key Auth
           ├─ RPM/TPM Rate Limiting
           ├─ Cost Tracking
           ├─ Model Routing
           └─ → TaaS Dynamo Bridge (:8090)
                  ├─ Tenant Header Injection
                  ├─ Usage Publishing (NATS)
                  └─ → NVIDIA Dynamo (GPU inference)

TaaS Gateway (:8080)
  ├─ Auth (JWT login/register)
  ├─ Token CRUD (syncs with LiteLLM virtual keys)
  ├─ Model Management
  ├─ Billing/Usage API
  ├─ Organization Management
  ├─ LiteLLM Webhook (/webhooks/litellm)
  └─ Web Dashboard
```

## What Moved to LiteLLM

| Feature | Before | After |
|---------|--------|-------|
| API key authentication | `internal/proxy/middleware.go` (APIKeyAuth) | LiteLLM virtual keys |
| Rate limiting (RPM/TPM) | `internal/quota/ratelimiter.go` | LiteLLM per-key limits |
| Cost calculation | `internal/billing/cost_calculator.go` | LiteLLM built-in cost tracking |
| Request routing | `internal/proxy/handler.go` | LiteLLM `model_list` config |
| Response caching | Not implemented | LiteLLM Redis cache |

## What Stays in TaaS

| Feature | Component |
|---------|-----------|
| User authentication (JWT) | `internal/auth/` |
| Token CRUD + LiteLLM sync | `internal/token/` + `internal/litellm/admin_client.go` |
| Model registry & deployment | `internal/model/` + `python/model_registry/` |
| Billing data persistence | `internal/billing/collector.go` + `internal/litellm/webhook.go` |
| Organization management | `internal/org/` |
| Dynamo integration | `internal/dynamo/` (via Dynamo Bridge) |
| SLA monitoring | `python/sla_monitor/` |
| Dynamo operator (K8s) | `python/dynamo_operator/` |
| Web dashboard | `web/` |

## New Components

### Dynamo Bridge (`cmd/dynamo-bridge/`)
Lightweight Go service between LiteLLM and NVIDIA Dynamo:
- Translates LiteLLM headers → Dynamo tenant headers
- Publishes usage events to NATS
- Preserves streaming support
- Port: 8090

### LiteLLM Webhook (`internal/litellm/webhook.go`)
Receives success/failure callbacks from LiteLLM Proxy and writes to `usage_records` table.
- Endpoint: `POST /webhooks/litellm`
- Auth: Bearer token (LITELLM_WEBHOOK_SECRET)

### LiteLLM Admin Client (`internal/litellm/admin_client.go`)
Communicates with LiteLLM's `/key/generate`, `/key/delete`, `/key/info` APIs to sync TaaS tokens as virtual keys.

### Token LiteLLM Service (`internal/token/litellm_service.go`)
Wraps the base token service to automatically sync Create/Revoke/Rotate operations with LiteLLM Proxy.

## Configuration

### Environment Variables (new)

| Variable | Description | Default |
|----------|-------------|---------|
| `LITELLM_ENABLED` | Enable LiteLLM integration | `false` |
| `LITELLM_PROXY_URL` | LiteLLM Proxy base URL | - |
| `LITELLM_MASTER_KEY` | LiteLLM admin API key | - |
| `LITELLM_WEBHOOK_SECRET` | Shared secret for webhook auth | - |
| `BRIDGE_INTERNAL_KEY` | Shared secret between LiteLLM and Dynamo Bridge | `internal-bridge-key` |

### LiteLLM Config (`litellm/config.yaml`)
Defines model routing, pricing, rate limits, and callback webhooks. See the file for details.

## Database Migration

Run migration `004_litellm_integration` to add the `litellm_key_token` column:
```sql
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS litellm_key_token TEXT DEFAULT '';
```

## Backward Compatibility

The system supports both modes:
- **`LITELLM_ENABLED=false`** (default): Legacy mode, uses the original Go proxy layer
- **`LITELLM_ENABLED=true`**: LiteLLM mode, /v1 routes are NOT registered on the gateway

Legacy mode is deprecated and will be removed in a future version.

## Development

```bash
# Start everything with LiteLLM
docker compose up -d

# Services:
#   LiteLLM Proxy:    http://localhost:4000  (inference API)
#   TaaS Gateway:     http://localhost:8080  (management API + dashboard)
#   Dynamo Bridge:    http://localhost:8090  (internal, LiteLLM → Dynamo)
#   Mock Dynamo:      http://localhost:9090  (dev only)
#   PostgreSQL:       localhost:5432
#   Redis:            localhost:6379
#   NATS:             localhost:4222

# Test inference through LiteLLM
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-your-virtual-key" \
  -H "Content-Type: application/json" \
  -d '{"model": "llama-3-8b", "messages": [{"role": "user", "content": "Hello"}]}'

# Create a virtual key via TaaS (auto-syncs to LiteLLM)
curl http://localhost:8080/tokens \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-key", "rate_limit_rpm": 60}'
```
