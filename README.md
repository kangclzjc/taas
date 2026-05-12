# TaaS — Token as a Service

[![CI](https://github.com/taas-platform/taas/actions/workflows/ci.yml/badge.svg)](https://github.com/taas-platform/taas/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/taas-platform/taas)](https://goreportcard.com/report/github.com/taas-platform/taas)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**TaaS** is a multi-tenant inference platform that turns
[NVIDIA Dynamo](https://developer.nvidia.com/dynamo) into a self-service product
backed by [LiteLLM Proxy](https://docs.litellm.ai/). Platform teams get a
control plane (orgs, users, models, deployments, billing) and an OpenAI-wire
compatible data plane (auth, rate-limit, spend tracking) — without writing the
glue.

What you can do end-to-end, today:

- **Click "Deploy" in the UI** to provision an NVIDIA Dynamo
  `DynamoGraphDeployment` (DGD or DGDR) on a Kubernetes cluster — no YAML.
- The deployment is automatically registered as a model in LiteLLM the moment
  it goes ready.
- Issue **scoped API keys** (LiteLLM virtual keys) from the TaaS UI — usable as
  drop-in `Authorization: Bearer …` for any OpenAI SDK.
- Per-key budgets, RPM/TPM limits, models allow-list, spend tracking — all in
  LiteLLM, all driven from the TaaS control plane.

---

## Architecture

TaaS splits cleanly into a **control plane** (TaaS' own services) and a
**data plane** (LiteLLM in front of NVIDIA Dynamo). The control plane never
sits on the inference hot path; the two planes communicate asynchronously
through NATS JetStream.

### Components

```mermaid
flowchart LR
    subgraph clients["Clients"]
        UI["Web UI<br/>(admin)"]
        SDK["OpenAI SDK / curl<br/>(inference)"]
    end

    subgraph control["Control Plane"]
        GW["TaaS Gateway<br/>(Go)"]
        OP["Dynamo Operator<br/>(Python)"]
        NATS[("NATS JetStream<br/>stream: TAAS_EVENTS")]
    end

    subgraph data["Data Plane"]
        LL["LiteLLM Proxy"]
        DGD["NVIDIA Dynamo<br/>Frontend + vLLM/SGLang<br/>workers"]
        GPU["GPU Pool"]
    end

    PG[("PostgreSQL<br/>taas + litellm DBs")]

    UI -- "JWT / cookies" --> GW
    SDK -- "Bearer sk-…" --> LL

    GW -- "publish<br/>model.deploy.requested" --> NATS
    NATS -- "deliver" --> OP
    OP -- "publish<br/>deployment.status.updated" --> NATS
    NATS -- "deliver" --> GW

    OP -- "create / watch CR" --> DGD
    GW -- "POST /key /model /team<br/>(admin)" --> LL

    GW -- "SQL" --> PG
    LL -- "SQL" --> PG

    LL -- "openai/{model}" --> DGD
    DGD --> GPU
```

NATS sits between Gateway and Operator on purpose — they never call each
other directly. Gateway publishes intents, Operator publishes status; both
sides are independent processes that can restart at any time without losing
work.

### Deploying a model (one-time, async)

```mermaid
sequenceDiagram
    autonumber
    participant U as User (Web UI)
    participant GW as TaaS Gateway
    participant N as NATS JetStream
    participant OP as Dynamo Operator
    participant K as Kubernetes (DGD CR)
    participant LL as LiteLLM

    U->>GW: POST /api/models/{id}/deploy
    GW->>GW: insert deployment row (status=pending)
    GW->>N: publish "model.deploy.requested"
    GW-->>U: 200 OK (pending)

    N->>OP: deliver "model.deploy.requested"
    OP->>K: create DynamoGraphDeployment CR
    OP->>K: ensure worker discovery Service
    K-->>OP: scheduled, frontend + worker ready

    OP->>N: publish "deployment.status.updated" (running, endpoint_url)
    N->>GW: deliver
    GW->>GW: update deployment row (status=running)
    GW->>LL: POST /model/new (api_base, model, headers)
    LL-->>GW: 200 OK
    Note over U,LL: Model now callable as model={slug} via virtual key
```

### Calling the model (every request, sync)

```mermaid
sequenceDiagram
    autonumber
    participant C as Client
    participant LL as LiteLLM Proxy
    participant DGD as Dynamo Frontend
    participant W as vLLM Worker

    C->>LL: POST /v1/chat/completions<br/>Authorization: Bearer sk-…
    LL->>LL: validate virtual key<br/>(models, budget, RPM/TPM)
    LL->>DGD: forward (api_base, model="openai/{hf-id}")
    DGD->>W: route (KV-aware) to prefill/decode workers
    W-->>DGD: stream tokens
    DGD-->>LL: stream response
    LL-->>C: stream response
    LL->>LL: record spend on virtual key (and its team)
```

The Gateway is **not** in the inference hot path — once a deployment is
registered with LiteLLM, every `/v1/*` call goes Client → LiteLLM → Dynamo
without touching TaaS Go services.

---

## Screenshots

| | |
|---|---|
| **Sign in** — JWT cookie auth, with self-service registration link | ![Sign in](docs/images/login.png) |
| **Dashboard** — per-org usage, request counts, spend, active tokens | ![Dashboard](docs/images/dashboard.png) |
| **Models** — list + filter; per-row Deploy / Delete actions | ![Models](docs/images/models.png) |
| **Deploy a model** — DGDR (auto-optimize) vs DGD (direct deploy), backend (vLLM/SGLang/TRT-LLM), GPU type, TP/PP, replicas — translated to NVIDIA DGD CR | ![Deploy form](docs/images/deploy-form.png) |
| **API Tokens** — TaaS-issued, synced to LiteLLM as virtual keys with per-key budget / RPM | ![API Tokens](docs/images/tokens.png) |

> Screenshots are kept in [`docs/images/`](docs/images/). Some pages
> (Login, Models, Tokens) still show the previous theme; the Deploy
> form reflects the current NVIDIA-inspired design system.

---

## Key Features

| | |
|---|---|
| **One-click DGD/DGDR from UI** | Web form translates to NVIDIA `DynamoGraphDeployment` (direct or auto-profile) — no kubectl, no YAML |
| **Model source selector** | HuggingFace id / NIM ref / custom storage URI; the operator passes the right value to vLLM |
| **Auto LiteLLM registration** | A deployment going `running` triggers `POST /model/new` with the right `api_base` and `model` — clients can call it immediately |
| **Self-service API keys** | TaaS API Tokens page issues LiteLLM virtual keys (with model allow-list, budget, RPM/TPM) — `sk-…` strings drop straight into OpenAI SDK |
| **Multi-tenant by default** | TaaS Organization → LiteLLM Team mapping; per-org spend, isolation, sharing across orgs |
| **Async, decoupled control plane** | Gateway never blocks on Kubernetes — all deploy actions fan out via NATS JetStream |
| **Full local k8s install** | `scripts/deploy-local-k8s.sh` brings up gateway + web + LiteLLM + Postgres + Redis + NATS + (optional) NVIDIA Dynamo on a single-node cluster |
| **OpenAI-wire compatible** | Inference endpoint is LiteLLM's, so any OpenAI SDK works against TaaS deployments verbatim |

---

## Tech Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Control-plane API** | Go 1.23, Gin | Auth, orgs/users, models, deployments, tokens, billing |
| **Inference data plane** | [LiteLLM Proxy](https://docs.litellm.ai/) | Virtual keys, models DB, request routing, spend metering |
| **Inference backend** | [NVIDIA Dynamo](https://developer.nvidia.com/dynamo) (vLLM / SGLang / TRT-LLM) | Disaggregated prefill/decode, KV-aware routing |
| **Operator** | Python 3.11 + FastAPI + `kubernetes-asyncio` | Consumes deploy events, creates DGD CRs, watches state |
| **Auth** | JWT (HS256) cookies, bcrypt, RBAC (owner/admin/member/viewer) | TaaS user authentication |
| **Database** | PostgreSQL 16 (`taas` DB + dedicated `litellm` DB) | All persistent state |
| **Cache / rate limit** | Redis 7 | Session, rate limit windows, deployment endpoint cache |
| **Message bus** | NATS JetStream (stream `TAAS_EVENTS`) | `model.deploy.requested`, `deployment.status.updated`, usage events |
| **Observability** | Prometheus, OpenTelemetry, Grafana, Jaeger | Metrics, traces, dashboards |
| **Web dashboard** | React 18 + TypeScript + Vite, served by nginx | Admin UI, model deploy form, API key management |
| **Infrastructure** | Helm chart (single chart, multiple values flavors), Docker | Single-cluster install for local + cloud |

---

## Quick Start

### Option 1 — Local Kubernetes (recommended; full stack)

This brings up everything (gateway + web + LiteLLM + Postgres + Redis +
embedded NATS) on a single-node cluster.

```bash
# Prereqs: a working kubectl context (kind / k3s / minikube / your cluster),
#          docker on the same host, and helm.
git clone https://github.com/taas-platform/taas.git
cd taas
bash scripts/deploy-local-k8s.sh
```

The script will print the port-forward / SSH-tunnel hints at the end. Typical
local setup:

```bash
kubectl port-forward -n taas-local svc/taas-local-web      30080:8080 &
kubectl port-forward -n taas-local svc/taas-local-litellm  14000:4000 &

# From your laptop (e.g. macOS) over SSH to the dev box:
ssh -N -L 3001:127.0.0.1:30080 -L 4001:127.0.0.1:14000 user@dev-box

# Then open:
#   TaaS UI :   http://127.0.0.1:3001
#   LiteLLM UI: http://127.0.0.1:4001/ui
```

To also bring up the **NVIDIA Dynamo platform** on the same cluster (CRDs +
controller, so you can deploy real DGDs):

```bash
bash scripts/install-nvidia-dynamo.sh   # one-shot helm install of nvidia-dynamo
L20_GPU_DEV=1 bash scripts/deploy-local-k8s.sh
```

The `L20_GPU_DEV=1` flavor uses `values-local-gpu-dev.yaml`, which enables
the operator's K8s mode and points it at the `dynamo` namespace.

### Option 2 — Minimal dev (Docker Compose, no LiteLLM, no GPU)

For people just hacking on the gateway / Go code:

```bash
docker compose -f deploy/docker/docker-compose.dev.yaml up -d
make build && ./bin/gateway
```

This path uses the mock Dynamo backend and skips LiteLLM. It is **not**
representative of the production data path.

---

## Deploying a model from the UI

The end-to-end happy path (after `deploy-local-k8s.sh` is up) takes ~3 minutes
once the GPU has the model weights cached.

### 1. Sign up / log in

Open the TaaS UI, click **Create one** on the login screen, register with
email + password (an organization is auto-created for you).

### 2. Create a model

`Models → + Create model`. Fields:

| Field | What to put |
|---|---|
| **Model name** | Human-readable, e.g. `Qwen3 8B` |
| **Slug** | Lowercase-hyphen, e.g. `qwen3-8b`. This is the `model:` clients pass to LiteLLM |
| **Source** | `HuggingFace` (recommended) / `NIM` / `Custom URI` |
| **Source value** | For HF: `Qwen/Qwen3-8B`. For NIM: `meta/llama-3.1-8b-instruct`. For Custom URI: `s3://…` |
| **Description** | Optional |

This step does **not** allocate any GPU — it just registers the model in TaaS.

### 3. Deploy a DGD or DGDR

Open the model detail page, click **+ Deploy**.

- **Direct Deploy (DGD)** — explicit TP/PP, replicas, image. Fast bring-up,
  no profiling. Good when you already know your config.
- **Auto-Optimized (DGDR)** — TaaS gives Dynamo a workload profile (ISL/OSL)
  + SLA targets (TTFT, ITL/TPOT). Dynamo's `AIConfigurator` picks an
  optimal config, then deploys. Slower to start, optimal once running.

Common knobs the form exposes: GPU type, GPUs per replica, TP / PP, replicas
min/max, backend (vLLM / SGLang / TRT-LLM), disaggregated serving toggle
(prefill_replicas / decode_replicas), router mode (random / kv).

Behind the scenes:

```
[UI Deploy click]
   → POST /api/models/{id}/deploy
   → TaaS Gateway: insert deployment row (status=pending)
   → NATS publish "model.deploy.requested"
   → Operator: create DynamoGraphDeployment CR
   → NVIDIA Dynamo controller: schedule frontend + worker pods
   → Worker registers itself (DynamoWorkerMetadata + EndpointSlice)
   → Operator watch sees Ready=True
   → NATS publish "deployment.status.updated" status=running
   → Gateway: status→running, endpoint_url→<frontend svc URL>
   → Gateway: POST /model/new → LiteLLM (auto)
```

The model detail page now shows the **"How to call this model"** card with
a copy-able curl + Python example, plus an editable `LiteLLM Base` field
that adapts to where you're calling from (dev box `:14000`, SSH tunnel
`:4001`, in-cluster service URL).

### 4. Issue an API key

`API Tokens → + Create Token`. Pick the model(s) the key may call, optional
budget and RPM. The popup shows the `sk-…` once — copy it.

### 5. Call the model

```bash
curl http://127.0.0.1:4001/v1/chat/completions \
  -H "Authorization: Bearer sk-…<your virtual key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-8b",
    "messages": [{"role":"user","content":"Hello"}]
  }'
```

Or in Python:

```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:4001/v1",
                api_key="sk-…<your virtual key>")
print(client.chat.completions.create(
    model="qwen3-8b",
    messages=[{"role":"user","content":"Hello"}],
).choices[0].message.content)
```

---

## Authentication model

TaaS layers three different kinds of credentials. They live at different
layers and have different blast radii — keep them straight.

| Credential | Where issued | Where used | Scope / blast radius |
|---|---|---|---|
| **JWT cookie** | TaaS Gateway `/auth/login` | Browser → TaaS UI | A logged-in user session; can manage their own org's resources |
| **TaaS API Token = LiteLLM Virtual Key** | TaaS UI `API Tokens` (or `POST /tokens`) | Client → LiteLLM `/v1/*` | One key, scoped to a model allow-list + budget + RPM/TPM |
| **LiteLLM Master Key** | Helm secret `secrets.litellmMasterKey` | Operators → LiteLLM admin endpoints (`/key`, `/model`, `/team`) | Full LiteLLM control — never give to clients |
| **Provider key (optional)** | Whoever owns the upstream (OpenAI, Anthropic, …) | Set when you `POST /model/new` an upstream-managed model | Real money — encrypted in DB by `master_key`, or referenced via `os.environ/<VAR>` |

For self-hosted vLLM (the default in `deploy-local-k8s.sh`), the provider
key is a placeholder — vLLM doesn't enforce it.

---

## API overview

Two distinct surfaces.

### TaaS Gateway (Go) — control plane

| Group | Endpoints | Purpose |
|---|---|---|
| Auth | `POST /auth/{register,login,refresh,logout,change-password}`, `GET /auth/me` | User session management |
| Organizations | `CRUD /organizations`, `/{id}/members` | Multi-tenancy — maps 1:1 to a LiteLLM Team |
| Models | `CRUD /models`, `POST /models/{id}/deploy`, `/undeploy` | Register a model, deploy / undeploy as DGD/DGDR |
| Deployments | `GET /models/{id}/deployments` | Per-model deployment status, endpoints |
| Tokens | `CRUD /tokens`, `POST /tokens/{id}/rotate` | Issue/rotate/revoke TaaS API Tokens (= LiteLLM virtual keys) |
| Sharing | `GET/POST/DELETE /models/{id}/shares` | Share a model across organizations |
| Usage | `GET /usage/{summary,by-model,by-token,timeseries}` | Org-scoped usage analytics |
| Billing | `GET /billing/{current,invoices}`, `/invoices/{id}` | Period and invoice views |
| Admin | `GET /admin/users`, `PUT /admin/users/{id}/quota`, `GET /admin/system/health` | Platform admin |

All control-plane endpoints take JWT (cookie or `Authorization: Bearer`).
Full reference: [docs/api-guide.md](docs/api-guide.md).

### LiteLLM Proxy — data plane

| Group | Examples | Auth |
|---|---|---|
| Inference | `POST /v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `GET /v1/models` | Virtual key (`sk-…`) |
| Admin | `POST /key/{generate,update,delete}`, `/model/{new,update,delete}`, `/team/{new,update}` | Master key |
| Health / introspection | `GET /health/{liveliness,readiness}`, `/key/info` | Mixed |

LiteLLM docs: <https://docs.litellm.ai/docs/proxy/quick_start>.

---

## Project structure

```
taas/
├── api/openapi.yaml                  # OpenAPI 3.0 (TaaS Gateway only)
├── cmd/
│   ├── gateway/                      # Main Go service: HTTP + NATS publisher/consumer
│   ├── auth/  token-manager/  billing/  # Single-binary microservices (optional)
├── internal/
│   ├── auth/        token/   model/        # Domain services
│   ├── litellm/                              # LiteLLM admin client + sync helpers
│   ├── natsutil/                             # JetStream stream + subscribe helpers
│   ├── proxy/  dynamo/                      # Inference proxy (legacy path) + Dynamo HTTP client
│   ├── billing/  quota/   audit/   monitoring/
├── pkg/                              # Shared: config, errors, middleware
├── proto/taas.proto                  # gRPC contract
├── python/
│   ├── dynamo_operator/              # NATS-driven DGD/DGDR operator
│   │   ├── main.py                   # Lifespan: nats subscribe + DGD watcher
│   │   ├── k8s_nvidia_dgd.py         # NvidiaDgdClient: build/apply CR, ensure svc, watch
│   │   └── config.py                 # Settings: CRD group/version, runtime image, …
│   ├── model_registry/  sla_monitor/
├── migrations/
│   ├── 001_initial_schema.{up,down}.sql
│   ├── 002_audit_log.{up,down}.sql
│   ├── 003_organizations.{up,down}.sql
│   ├── 004_litellm_integration.{up,down}.sql
│   └── 004_models_hf_model.{up,down}.sql
├── deploy/
│   ├── docker/
│   │   ├── Dockerfile                # Go services
│   │   ├── Dockerfile.python         # Operator / model registry / sla monitor
│   │   ├── Dockerfile.web            # nginx + vite build
│   │   ├── nginx.web.default.conf
│   │   └── docker-compose.dev.yaml
│   ├── helm/taas/                    # Single chart, many values flavors
│   │   ├── values.yaml
│   │   ├── values-local.yaml         # Single-node, CPU-only dev
│   │   ├── values-local-gpu-dev.yaml # Local + real Dynamo on GPU
│   │   ├── values-fullstack-l20.yaml # Single-host L20 demo
│   │   ├── values-nvidia-dynamo.yaml # Upstream NVIDIA Dynamo flavor
│   │   └── templates/                # Operator, gateway, web, LiteLLM, NATS, …
│   ├── extras/                       # Standalone DGD/DGDR sample manifests
│   └── k8s/                          # Raw CRDs / namespace
├── web/                              # React + TS dashboard
│   └── src/
│       ├── pages/                    # Login, Register, Dashboard, Models, ModelDetail,
│       │                             # Tokens, Usage, Organizations, Profile
│       ├── components/               # DeployForm (DGD/DGDR), Toast, Skeleton, …
│       ├── api/client.ts             # Typed API client
│       └── vite-env.d.ts             # VITE_API_URL / VITE_LITELLM_PUBLIC_URL
├── scripts/
│   ├── setup-dev.sh                  # Install tools, start deps
│   ├── deploy-local-k8s.sh           # One-shot local K8s install (this README's main path)
│   ├── install-nvidia-dynamo.sh      # Install upstream nvidia-dynamo helm chart
│   ├── bind-taas-dynamo-frontend.sh  # Wire gateway to a pre-existing Dynamo Frontend
│   ├── e2e-mock.sh   seed.sh
├── test/{e2e,load}/
├── docs/{api-guide.md, deployment.md, architecture/, runbook/}
├── Makefile  go.mod  go.sum  .golangci.yml
└── .github/workflows/ci.yml
```

---

## Development setup

### Prerequisites

- **Go** 1.23+
- **Node.js** 20+ (for `web/`)
- **Docker** (or Podman) and **Docker Compose**
- **kubectl** + **Helm** for the k8s path
- **Python** 3.11+ for the operator (managed via `uv pip install -r requirements.txt`)
- **golangci-lint** (installed automatically by `scripts/setup-dev.sh`)

### Bootstrap

```bash
bash scripts/setup-dev.sh   # installs tools, starts deps, downloads modules
make build                  # build all Go binaries
make lint test              # lint + unit tests
```

### Run gateway against local deps

```bash
export TAAS_DATABASE_URL="postgres://taas:taas@localhost:5432/taas?sslmode=disable"
export TAAS_REDIS_URL="redis://localhost:6379"
export TAAS_NATS_URL="nats://localhost:4222"
export TAAS_JWT_SIGNING_KEY="dev-secret-key-at-least-32-chars-long"
./bin/gateway
```

### Web

```bash
cd web && npm ci && npm run dev    # vite dev server with /api proxy to gateway
```

### Useful Make targets

| Target | Description |
|--------|-------------|
| `make build` | Build all Go service binaries |
| `make docker-build` | Build all Docker images (gateway + python services) |
| `make docker-up` / `down` | Start / stop the docker-compose dev stack |
| `make helm-lint` / `helm-template` | Lint / dry-run the chart |
| `make migrate-up` | Run DB migrations against `TAAS_DATABASE_URL` |
| `make test` / `test-coverage` / `test-load` | Unit tests, HTML coverage, k6 load |
| `make dev-deps` | Postgres + Redis + NATS only (no app services) |

---

## Configuration

All gateway / operator config is via `TAAS_*` and `LITELLM_*` environment
variables (or the helm `values-*.yaml`).

### Gateway

| Variable | Default | Notes |
|---|---|---|
| `TAAS_PORT` | `8080` | HTTP listen port |
| `TAAS_DATABASE_URL` | required | Postgres DSN |
| `TAAS_REDIS_URL` | required | Redis URL |
| `TAAS_NATS_URL` | optional | If set, deploy events + status flow via NATS JetStream |
| `TAAS_JWT_SIGNING_KEY` | required | ≥32 chars |
| `TAAS_JWT_EXPIRY_SECONDS` | `3600` | Access token TTL |
| `TAAS_DYNAMO_FRONTEND_URL` | optional | Only needed for the legacy direct-proxy path; with LiteLLM enabled this is auto-resolved per deployment |
| `TAAS_CORS_ALLOWED_ORIGINS` | empty | Comma-separated origins allowed by the API |
| `TAAS_OTLP_ENDPOINT` | optional | OpenTelemetry collector endpoint |
| `TAAS_LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |

### LiteLLM integration (gateway side)

| Variable | Default | Notes |
|---|---|---|
| `LITELLM_ENABLED` | `false` | When `true`, gateway syncs models + tokens to LiteLLM |
| `LITELLM_PROXY_URL` | derived | Internal URL of LiteLLM (`http://<release>-litellm:4000` in helm) |
| `LITELLM_MASTER_KEY` | required when enabled | LiteLLM admin key — same value used by helm `secrets.litellmMasterKey` |
| `LITELLM_WEBHOOK_SECRET` | optional | Reserved for future inbound webhooks |

### Dynamo Operator

| Variable | Default | Notes |
|---|---|---|
| `TAAS_OPERATOR_K8S_ENABLED` | `false` | When `false`, operator just publishes a mock `running` event for dev |
| `TAAS_OPERATOR_CRD_MODE` | `nvidia_dgd` | Currently only NVIDIA DGD is implemented |
| `TAAS_DYNAMO_NAMESPACE` | `dynamo` | Namespace where DGD CRs are created |
| `TAAS_NVIDIA_DGD_RUNTIME_IMAGE` | `nvcr.io/nvidia/ai-dynamo/vllm-runtime:1.0.1` | Worker image |
| `TAAS_NVIDIA_DGD_HF_SECRET_NAME` | `hf-token-secret` | K8s Secret with `HF_TOKEN` for gated models |
| `TAAS_NVIDIA_HF_MODEL_DEFAULT` | `Qwen/Qwen3-0.6B` | Fallback when a model has no source value |

Full reference (including all NVIDIA-specific knobs and the full LiteLLM
config map): [docs/deployment.md](docs/deployment.md#environment-variables-reference).

---

## Deployment

```bash
# Add subchart repos (once)
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update
helm dep update deploy/helm/taas

# Pick a flavor and install
helm upgrade --install taas-local deploy/helm/taas \
  -n taas-local --create-namespace \
  -f deploy/helm/taas/values-local.yaml \
  --set secrets.jwtSigningKey="$(openssl rand -base64 48)" \
  --set secrets.dbPassword="$(openssl rand -base64 24)" \
  --set secrets.litellmMasterKey="sk-litellm-master-$(openssl rand -hex 8)"
```

Available `values-*.yaml` flavors:

| Flavor | What it gives you |
|---|---|
| `values-local.yaml` | Single-node CPU dev: gateway + web + LiteLLM + Postgres + Redis + embedded NATS, mock Dynamo (no GPU) |
| `values-local-gpu-dev.yaml` | Above + operator in K8s mode, talks to a real `dynamo` namespace |
| `values-fullstack-l20.yaml` | Single-host L20 demo (real GPU, prometheus stack) |
| `values-nvidia-dynamo.yaml` | Upstream NVIDIA Dynamo flavor |
| `values-dev.yaml` | CI / staging-style cloud cluster |

For real deployment notes, secret rotation, observability dashboards and
runbooks: [docs/deployment.md](docs/deployment.md) and
[docs/runbook/operations.md](docs/runbook/operations.md).

---

## Documentation

- **[API Guide](docs/api-guide.md)** — control-plane API with curl examples
- **[Deployment Guide](docs/deployment.md)** — production K8s deployment
- **[Operations Runbook](docs/runbook/operations.md)** — troubleshooting, on-call
- **[Architecture](docs/architecture/ARCHITECTURE.md)** — system design + data flows
- **[Data Model](docs/architecture/DATA_MODEL.md)** — DB schema + Redis structures

---

## Contributing

1. Fork and create a feature branch from `master`
2. `bash scripts/setup-dev.sh`
3. Make the change. Follow existing patterns; keep gateway logic in `internal/`
4. `make lint test` (and add tests for new behavior)
5. Commit with [Conventional Commits](https://www.conventionalcommits.org/):
   `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `style:`, `optimize:`
6. Open a PR with a clear description and the user-visible impact

### Code guidelines

- Go: passes `golangci-lint` with the project `.golangci.yml`
- All exported functions get doc comments
- New features need unit tests; bug fixes need regression tests
- DB changes ship `up` *and* `down` migrations
- API changes update `api/openapi.yaml`

### Reporting issues

GitHub Issues. Include:

- Steps to reproduce
- Expected vs actual behavior
- TaaS version, Go version, environment (cluster type, GPU, …)

---

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
