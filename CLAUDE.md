# CLAUDE.md - TaaS Project Guidelines

## Project Overview
TaaS (Token as a Service) — A multi-tenant model inference platform built on NVIDIA Dynamo.
Users can deploy models, get API tokens, share inference services, with SLA guarantees and usage monitoring.

## Architecture Principles
- **Microservices**: Each domain has its own service
- **API-first**: OpenAPI specs before implementation
- **Multi-tenant isolation**: Strict tenant boundaries for security and SLA
- **Observability**: Metrics, logs, traces from day one
- **Cloud-native**: Kubernetes-first deployment

## Tech Stack
- **Language**: Go (API Gateway, Auth, Token Management), Python (Dynamo integration, ML ops)
- **Framework**: Go — gin/echo, Python — FastAPI
- **Database**: PostgreSQL (primary), Redis (cache/rate-limit)
- **Message Queue**: NATS JetStream
- **Monitoring**: Prometheus + Grafana + OpenTelemetry
- **Inference**: NVIDIA Dynamo (orchestrating SGLang/vLLM/TensorRT-LLM)
- **Container**: Docker + Kubernetes + Helm
- **Auth**: JWT + OAuth2 + API Key management

## Project Structure
```
taas/
├── CLAUDE.md                 # This file
├── README.md
├── docs/
│   ├── architecture/         # Architecture design docs
│   ├── api/                  # OpenAPI specs
│   └── runbook/              # Operations runbook
├── proto/                    # Protobuf/gRPC definitions
├── api/                      # OpenAPI specs (yaml)
├── cmd/                      # Go service entry points
│   ├── gateway/              # API Gateway
│   ├── auth/                 # Auth service
│   ├── token-manager/        # Token management service
│   └── billing/              # Billing service
├── internal/                 # Go internal packages
│   ├── auth/                 # Authentication & authorization
│   ├── token/                # Token CRUD & validation
│   ├── model/                # Model registry & lifecycle
│   ├── quota/                # Quota & rate limiting
│   ├── billing/              # Usage tracking & billing
│   ├── monitoring/           # Metrics & alerting
│   └── dynamo/               # Dynamo client integration
├── pkg/                      # Shared Go packages
│   ├── middleware/            # HTTP middleware
│   ├── errors/               # Error types
│   └── config/               # Configuration
├── python/                   # Python services
│   ├── dynamo_operator/      # Dynamo deployment operator
│   ├── model_registry/       # Model upload & validation
│   └── sla_monitor/          # SLA monitoring & alerting
├── web/                      # Frontend (React + TypeScript)
│   ├── src/
│   │   ├── pages/
│   │   ├── components/
│   │   └── api/
│   └── package.json
├── deploy/                   # Deployment configs
│   ├── helm/                 # Helm charts
│   ├── docker/               # Dockerfiles
│   └── k8s/                  # Raw K8s manifests
├── scripts/                  # Build & dev scripts
├── test/                     # Integration tests
│   ├── e2e/
│   └── load/
├── go.mod
├── go.sum
└── Makefile
```

## Coding Standards
- Go: follow standard Go project layout, use `internal/` for private packages
- Python: use type hints, follow PEP 8, use pydantic for models
- All APIs must have OpenAPI spec before implementation
- Every service must expose health check and metrics endpoints
- Use structured logging (JSON format)
- Error codes must be documented and consistent

## Key Design Decisions
1. **Go for API layer**: Performance, strong typing, excellent HTTP/gRPC support
2. **Python for ML ops**: Dynamo SDK is Python-native, ML ecosystem
3. **NATS over Kafka**: Simpler ops, sufficient for our event volume
4. **PostgreSQL**: Proven, supports JSONB for flexible metadata
5. **Token = API Key**: Each token is a scoped API key with embedded tenant info

## Multi-Agent Workflow
This project uses multiple specialized agents:
- **Architect**: Designs system architecture and interfaces
- **Backend Dev**: Implements Go/Python services
- **Frontend Dev**: Implements React frontend
- **QA Engineer**: Writes tests and validates
- **Code Reviewer**: Reviews PRs and suggests improvements

Each agent should read this file before starting work.
