# TaaS — Token as a Service

[![CI](https://github.com/taas-platform/taas/actions/workflows/ci.yml/badge.svg)](https://github.com/taas-platform/taas/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/taas-platform/taas)](https://goreportcard.com/report/github.com/taas-platform/taas)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**TaaS** is a multi-tenant model inference platform built on [NVIDIA Dynamo](https://developer.nvidia.com/dynamo). It gives organizations a production-ready way to deploy LLMs behind scoped API tokens, enforce SLA guarantees, track per-token usage, and share inference services across teams — all through an API that is wire-compatible with the OpenAI API.

**Why TaaS exists:** Running LLMs in production requires more than just a GPU and a model. You need authentication, per-customer rate limiting, usage metering, billing, model lifecycle management, and SLA enforcement. TaaS wraps NVIDIA Dynamo's disaggregated prefill/decode architecture with a complete multi-tenant control plane so platform teams can offer "inference as a service" to internal or external customers.

## Architecture

```
                           ┌─────────────────────────────────────────────────────┐
                           │                   TaaS Platform                     │
                           │                                                     │
  ┌──────────┐             │  ┌─────────┐    ┌───────────────────────────────┐   │
  │  Client   │─── HTTPS ──┼─▶│   API   │───▶│  Auth · Token · Proxy · Billing│  │
  │ (SDK/CLI/ │             │  │ Gateway │    │       (Go microservices)      │   │
  │  WebUI)   │             │  │  (Gin)  │    └──────────────┬────────────────┘   │
  └──────────┘             │  └────┬────┘                    │                   │
                           │       │ /v1/*                   │                   │
                           │       ▼                         ▼                   │
                           │  ┌─────────┐    ┌───────────────────────────────┐   │
                           │  │  NVIDIA  │    │  Dynamo Operator · Model      │   │
                           │  │  Dynamo  │    │  Registry · SLA Monitor       │   │
                           │  │ Frontend │    │       (Python services)       │   │
                           │  └────┬────┘    └───────────────────────────────┘   │
                           │       │                                             │
                           │       ▼                                             │
                           │  ┌──────────────────────────────────┐               │
                           │  │  Prefill Workers │ Decode Workers │◀── GPU Pool  │
                           │  │          (KV Cache Manager)       │               │
                           │  └──────────────────────────────────┘               │
                           │                                                     │
                           │  ┌──────────┐ ┌───────┐ ┌──────────────┐           │
                           │  │PostgreSQL│ │ Redis │ │NATS JetStream│           │
                           │  └──────────┘ └───────┘ └──────────────┘           │
                           └─────────────────────────────────────────────────────┘
```

**Request flow:** Client sends an OpenAI-compatible request → API Gateway validates the API key (Redis cache + SHA-256 hash lookup) → enforces rate limit (sliding window in Redis) → proxies to NVIDIA Dynamo Frontend with tenant context headers (`X-Tenant-ID`, `X-SLA-Tier`, `X-Token-ID`) → Dynamo routes to prefill/decode workers → response streams back to client → Gateway publishes usage event to NATS JetStream → Billing Collector persists to PostgreSQL.

## Tech Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **API Gateway** | Go 1.23, Gin | HTTP routing, auth middleware, inference proxy |
| **Auth** | JWT (HS256), bcrypt, Redis blocklist | User authentication, RBAC (owner/admin/member/viewer) |
| **Token Management** | SHA-256 hashing, Redis cache | API key lifecycle (create, rotate, revoke, scope) |
| **Rate Limiting** | Redis sorted sets (sliding window) | Per-token RPM/TPM enforcement |
| **Inference Backend** | NVIDIA Dynamo | Disaggregated prefill/decode, KV cache, GPU scheduling |
| **Usage & Billing** | NATS JetStream, PostgreSQL | Event-driven usage collection, invoice generation |
| **Database** | PostgreSQL 16 | Users, tokens, models, deployments, usage records |
| **Cache / Rate Limit** | Redis 7 | Token cache, rate limit counters, deployment endpoints |
| **Message Bus** | NATS JetStream | Usage events, deployment lifecycle, SLA violations |
| **Observability** | Prometheus, Grafana, Jaeger, OpenTelemetry | Metrics, dashboards, distributed tracing |
| **ML Operations** | Python (FastAPI) | Dynamo Operator, Model Registry, SLA Monitor |
| **Web Dashboard** | React, TypeScript, Vite | Admin UI for tokens, models, usage |
| **Infrastructure** | Kubernetes, Helm, Docker | Deployment, scaling, GPU scheduling |

## Quick Start

**Prerequisites:** Docker and Docker Compose installed.

```bash
# 1. Clone and start all services
git clone https://github.com/taas-platform/taas.git
cd taas
docker compose -f deploy/docker/docker-compose.dev.yaml up -d

# 2. Wait for healthy (takes ~30s)
until curl -s http://localhost:8080/health | grep -q ok; do sleep 2; done

# 3. Register a user (auto-creates an organization)
curl -s http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"SuperSecret123!"}' | jq .

# Save the access_token from the response:
export TOKEN="<access_token from response>"

# 4. Create an API token for inference
curl -s http://localhost:8080/tokens \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-first-key","rate_limit_rpm":100}' | jq .

# Save the key from the response (shown only once):
export API_KEY="<key from response>"

# 5. Make an inference call (OpenAI-compatible)
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-8b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 128
  }'
```

> **Note:** The dev environment uses a mock Dynamo backend. For real GPU inference, deploy on a Kubernetes cluster with NVIDIA GPUs — see [Deployment Guide](docs/deployment.md).

## API Overview

TaaS exposes a RESTful API grouped into the following endpoint families. Full details with curl examples are in the [API Guide](docs/api-guide.md).

| Group | Endpoints | Description |
|-------|----------|-------------|
| **Auth** | `POST /auth/register`, `/login`, `/refresh`, `/logout` | User registration, JWT login, token refresh, logout with blocklist |
| **Users** | `GET/PUT /users/me` | Get and update current user profile |
| **Organizations** | `CRUD /organizations`, `/organizations/{id}/members` | Create orgs, invite members, manage roles |
| **Models** | `CRUD /models`, `POST /models/{id}/deploy`, `/undeploy` | Register, deploy, undeploy, delete models |
| **Tokens** | `CRUD /tokens`, `POST /tokens/{id}/rotate` | Create scoped API keys, list, revoke, rotate |
| **Model Sharing** | `GET/POST /models/{id}/shares`, `DELETE .../shares/{id}` | Share models across organizations |
| **Inference** | `POST /v1/chat/completions`, `/completions`, `/embeddings`, `GET /v1/models` | OpenAI-compatible inference (streaming + non-streaming) |
| **Usage** | `GET /usage/summary`, `/by-model`, `/by-token`, `/timeseries` | Usage analytics by org, model, token, time period |
| **Billing** | `GET /billing/current`, `/invoices`, `/invoices/{id}` | Current billing period, invoice list and details |
| **Admin** | `GET /admin/users`, `PUT /admin/users/{id}/quota`, `GET /admin/system/health` | Platform admin: user management, quotas, system health |

The inference endpoints (`/v1/*`) are authenticated via API key (`taas_...`). All other endpoints use JWT Bearer tokens.

## Project Structure

```
taas/
├── api/
│   └── openapi.yaml              # OpenAPI 3.0 specification
├── cmd/
│   ├── gateway/main.go           # API Gateway entry point
│   ├── auth/main.go              # Auth service entry point
│   ├── token-manager/main.go     # Token Manager entry point
│   └── billing/main.go           # Billing service entry point
├── internal/
│   ├── auth/                     # Authentication, JWT, RBAC, rate limiting, blocklist
│   ├── token/                    # API token CRUD, validation, caching
│   ├── model/                    # Model registry, deployment, sharing
│   ├── proxy/                    # Inference proxy to Dynamo, SSE streaming
│   ├── billing/                  # Usage collection (NATS), cost calculation, handler
│   ├── dynamo/                   # NVIDIA Dynamo HTTP client
│   ├── quota/                    # Rate limiter (Redis sliding window)
│   ├── audit/                    # Structured audit logging
│   └── monitoring/               # Prometheus metrics definitions
├── pkg/
│   ├── config/                   # Configuration (Viper, TAAS_* env vars)
│   ├── errors/                   # Typed error handling
│   └── middleware/               # HTTP middleware (logging, recovery, CORS, request ID)
├── proto/
│   └── taas.proto                # gRPC protobuf definitions
├── python/
│   ├── dynamo_operator/          # K8s operator for Dynamo worker lifecycle
│   ├── model_registry/           # Model storage and validation
│   └── sla_monitor/              # SLA compliance monitoring
├── migrations/
│   ├── 001_initial_schema.up.sql
│   ├── 001_initial_schema.down.sql
│   ├── 002_audit_log.up.sql
│   └── 002_audit_log.down.sql
├── deploy/
│   ├── docker/
│   │   ├── Dockerfile
│   │   ├── Dockerfile.python
│   │   └── docker-compose.dev.yaml
│   ├── helm/taas/                # Helm chart (Chart.yaml, values.yaml, templates/)
│   └── k8s/                      # Raw K8s manifests (CRDs, namespace)
├── web/                          # React + TypeScript dashboard
│   ├── src/
│   │   ├── pages/                # Dashboard, Login, Models, Tokens, Usage
│   │   ├── components/           # Shared UI components
│   │   └── api/client.ts         # API client
│   └── vite.config.ts
├── test/
│   ├── e2e/                      # End-to-end tests
│   └── load/                     # k6 load tests (auth, inference, rate limiting)
├── scripts/
│   └── setup-dev.sh              # Dev environment bootstrap
├── docs/
│   ├── architecture/             # Architecture, data model, Dynamo integration
│   ├── api-guide.md              # API usage guide with curl examples
│   ├── deployment.md             # Production deployment guide
│   └── runbook/operations.md     # Operations runbook
├── Makefile                      # Build, test, lint, Docker, Helm targets
├── go.mod / go.sum
└── .github/workflows/ci.yml     # CI pipeline (lint, test, build, e2e)
```

## Development Setup

### Prerequisites

- **Go** 1.23+
- **Docker** & Docker Compose
- **kubectl** and **Helm** (for K8s deployment)
- **golangci-lint** (installed automatically by `setup-dev.sh`)

### Local Development

```bash
# Automated setup (installs tools, starts deps, downloads modules)
bash scripts/setup-dev.sh

# Or manually:
# 1. Start dependencies
make docker-up

# 2. Build all services
make build

# 3. Run the gateway locally
export TAAS_DATABASE_URL="postgres://taas:taas@localhost:5432/taas?sslmode=disable"
export TAAS_REDIS_URL="redis://localhost:6379"
export TAAS_NATS_URL="nats://localhost:4222"
export TAAS_JWT_SIGNING_KEY="dev-secret-key-at-least-32-chars-long"
./bin/gateway
```

### Running Tests

```bash
make test              # Unit tests with race detector
make test-coverage     # Generate HTML coverage report
make test-e2e          # End-to-end tests (requires running services)
make test-load         # k6 load tests
make lint              # golangci-lint
```

### Useful Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build all Go service binaries |
| `make build-gateway` | Build only the gateway |
| `make docker-up` | Start full dev environment (all services + deps) |
| `make docker-down` | Stop dev environment and remove volumes |
| `make docker-build` | Build all Docker images |
| `make helm-lint` | Lint Helm charts |
| `make helm-template` | Render Helm templates (dry run) |
| `make migrate-up` | Run database migrations |
| `make clean` | Remove build artifacts |

## Deployment

TaaS ships with a Helm chart for Kubernetes deployment. See the full [Deployment Guide](docs/deployment.md).

### Quick Helm Install

```bash
# Add dependency repos
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Install TaaS
helm upgrade --install taas deploy/helm/taas \
  --namespace taas \
  --create-namespace \
  --set secrets.jwtSigningKey="$(openssl rand -base64 48)" \
  --set secrets.dbPassword="<your-db-password>" \
  --set secrets.redisPassword="<your-redis-password>" \
  --set gateway.ingress.hosts[0].host=api.yourdomain.com \
  --set image.tag=0.1.0
```

### Key Environment Variables

All configuration is via `TAAS_*` environment variables (or YAML config files):

| Variable | Description | Default |
|----------|-------------|---------|
| `TAAS_PORT` | HTTP listen port | `8080` |
| `TAAS_DATABASE_URL` | PostgreSQL connection string | *(required)* |
| `TAAS_REDIS_URL` | Redis connection string | *(required)* |
| `TAAS_NATS_URL` | NATS server URL | *(optional)* |
| `TAAS_JWT_SIGNING_KEY` | JWT signing key (≥32 chars) | *(required)* |
| `TAAS_JWT_EXPIRY_SECONDS` | Access token TTL | `3600` |
| `TAAS_DYNAMO_FRONTEND_URL` | NVIDIA Dynamo Frontend URL | *(required for inference)* |
| `TAAS_LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `TAAS_OTLP_ENDPOINT` | OpenTelemetry collector endpoint | *(optional)* |

Full environment variable reference: [docs/deployment.md](docs/deployment.md#environment-variables-reference)

## Contributing

We welcome contributions! Here's how to get started:

1. **Fork** the repository and create a feature branch from `master`
2. **Set up** local development: `bash scripts/setup-dev.sh`
3. **Make changes** — follow existing code style and patterns
4. **Test** your changes: `make lint test`
5. **Commit** with [Conventional Commits](https://www.conventionalcommits.org/) format:
   - `feat: add model versioning API`
   - `fix: correct rate limit window calculation`
   - `docs: update deployment guide`
6. **Open a Pull Request** with a clear description of the change

### Code Guidelines

- Go code must pass `golangci-lint` with the project's `.golangci.yml` config
- All exported functions need doc comments
- New features require unit tests; bug fixes require regression tests
- Database changes need up *and* down migration files
- API changes must update `api/openapi.yaml`

### Reporting Issues

Use GitHub Issues. Include:
- Steps to reproduce
- Expected vs. actual behavior
- TaaS version, Go version, and environment details

## Documentation

- **[API Guide](docs/api-guide.md)** — Complete API usage with curl examples
- **[Deployment Guide](docs/deployment.md)** — Production K8s deployment
- **[Operations Runbook](docs/runbook/operations.md)** — Troubleshooting and emergency procedures
- **[Architecture](docs/architecture/ARCHITECTURE.md)** — System design and data flows
- **[Data Model](docs/architecture/DATA_MODEL.md)** — Database schema and Redis structures

## License

Licensed under the [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0).

```
Copyright 2024 TaaS Platform Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
