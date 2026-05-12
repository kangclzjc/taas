# LiteLLM Migration Guide

## Overview

TaaS has been restructured into a clean **control plane + data plane** architecture:

- **TaaS** = Control Plane (manage models, deployments, tokens, orgs, billing)
- **LiteLLM Proxy** = Data Plane (handle all inference requests)

TaaS dynamically registers Dynamo endpoints into LiteLLM when deployments start, and removes them when deployments stop. No static model configuration needed.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                      Clients                         │
│   (SDKs, CLI, Applications)                          │
├──────────────┬──────────────────────────────────────┤
│  Management  │         Inference                     │
│  Requests    │         Requests                      │
▼              ▼                                       │
┌──────────┐  ┌──────────────────────┐                │
│   TaaS    │  │   LiteLLM Proxy      │                │
│  Gateway  │  │   (:4000)            │                │
│  (:8080)  │  │                      │                │
│           │  │  ✓ Virtual Key Auth  │                │
│ ✓ Auth    │──│  ✓ Rate Limiting     │                │
│ ✓ Tokens ←──→  ✓ Cost Tracking     │                │
│ ✓ Models ←──→  ✓ Model Routing     │                │
│ ✓ Billing │  │  ✓ Failover         │                │
│ ✓ Orgs    │  └─────────┬───────────┘                │
│ ✓ Dashboard│            │                            │
└─────┬──────┘            │                            │
      │                   ▼                            │
      │           ┌───────────────┐                    │
      │           │ NVIDIA Dynamo │                    │
      └──────────→│ (GPU Inference)│                   │
   Deploy via     └───────────────┘                    │
   K8s Operator                                        │
└─────────────────────────────────────────────────────┘
```

## How It Works

### Model Deployment Flow
```
1. User calls TaaS: POST /models/:id/deploy
2. TaaS creates Deployment record (status: "pending")
3. TaaS publishes NATS event → Dynamo Operator
4. Dynamo Operator creates K8s DynamoWorker CRD
5. Dynamo starts, reports endpoint_url via NATS
6. TaaS receives event, updates Deployment (status: "running", endpoint_url)
7. TaaS calls LiteLLM: POST /model/new (registers endpoint)
8. ✅ Clients can now call the model via LiteLLM
```

### Model Stop/Delete Flow
```
1. User calls TaaS: DELETE /models/:id or stop deployment
2. TaaS calls LiteLLM: POST /model/delete (removes from routing)
3. TaaS publishes NATS event → Dynamo Operator cleans up K8s
```

### Token (API Key) Flow
```
1. User calls TaaS: POST /tokens
2. TaaS creates token record
3. TaaS calls LiteLLM: POST /key/generate (creates virtual key)
4. Returns LiteLLM key (sk-...) to user
5. User uses this key to call LiteLLM for inference
```

## Components

| Component | Role | Port |
|-----------|------|------|
| **LiteLLM Proxy** | Data plane — all inference traffic | 4000 |
| **TaaS Gateway** | Control plane — management API + dashboard | 8080 |
| **Dynamo Operator** | K8s operator for Dynamo CRDs | - |
| **Model Registry** | Model metadata storage | - |
| **SLA Monitor** | SLA metrics & alerting | - |

## What TaaS Manages

- **Dynamo deployments**: Create, scale, stop GPU inference services
- **LiteLLM models**: Register/remove Dynamo endpoints (via `/model/new`, `/model/delete`)
- **LiteLLM virtual keys**: Create/revoke API keys (via `/key/generate`, `/key/delete`)
- **Users & organizations**: JWT auth, RBAC, multi-tenancy
- **Billing**: Usage records from LiteLLM webhooks + NATS events
- **Dashboard**: Web UI for all the above

## What LiteLLM Handles

- **Inference routing**: Forward requests to correct Dynamo endpoint
- **API key validation**: Virtual keys with per-key rate limits
- **Rate limiting**: RPM/TPM per key, per team
- **Cost tracking**: Per-model token pricing
- **Failover**: Retry across multiple deployments
- **Caching**: Redis-based response cache
- **Observability**: Metrics, logging, webhook callbacks

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LITELLM_ENABLED` | Enable LiteLLM integration | `false` |
| `LITELLM_PROXY_URL` | LiteLLM Proxy base URL | - |
| `LITELLM_MASTER_KEY` | LiteLLM admin API key | - |
| `LITELLM_WEBHOOK_SECRET` | Shared secret for webhook auth | - |

### LiteLLM Config (`litellm/config.yaml`)
Minimal config — models are managed dynamically. Only contains:
- Database URL (shared with TaaS)
- Redis cache settings
- Webhook callback URL
- Router settings (retry, timeout, failover)

## Database Migration

Run migration `004_litellm_integration`:
```sql
-- api_tokens: LiteLLM virtual key reference
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS litellm_key_token TEXT DEFAULT '';

-- deployments: LiteLLM model ID for cleanup
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS litellm_model_id TEXT DEFAULT '';
```

## Backward Compatibility

- `LITELLM_ENABLED=false` (default): Legacy mode, uses the original Go proxy layer
- `LITELLM_ENABLED=true`: LiteLLM mode, `/v1` inference routes are NOT on the gateway

## Development

```bash
# Start everything
docker compose up -d

# Services:
#   LiteLLM Proxy:    http://localhost:4000  (inference API)
#   TaaS Gateway:     http://localhost:8080  (management API + dashboard)
#   PostgreSQL:       localhost:5432
#   Redis:            localhost:6379
#   NATS:             localhost:4222

# 1. Register & login
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "Admin123!", "name": "Admin"}'

# 2. Create a model
curl -X POST http://localhost:8080/models \
  -H "Authorization: Bearer <jwt>" \
  -d '{"name": "LLaMA 3 8B", "slug": "llama-3-8b", "framework": "pytorch"}'

# 3. Deploy it (TaaS → Dynamo Operator → Dynamo → registers in LiteLLM)
curl -X POST http://localhost:8080/models/<id>/deploy \
  -H "Authorization: Bearer <jwt>" \
  -d '{"name": "prod-llama", "replicas_min": 1}'

# 4. Create an API key (auto-syncs to LiteLLM virtual key)
curl -X POST http://localhost:8080/tokens \
  -H "Authorization: Bearer <jwt>" \
  -d '{"name": "my-app-key", "rate_limit_rpm": 60}'

# 5. Use the key for inference (goes to LiteLLM → Dynamo)
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-..." \
  -d '{"model": "llama-3-8b", "messages": [{"role": "user", "content": "Hello"}]}'
```
