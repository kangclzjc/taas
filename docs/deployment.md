# TaaS Production Deployment Guide

## Prerequisites

- Kubernetes cluster (1.28+) with GPU nodes
- NVIDIA GPU Operator installed
- PostgreSQL 16+
- Redis 7+
- NATS Server 2.10+ with JetStream enabled
- Helm 3.12+
- `kubectl` configured for your cluster

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TAAS_PORT` | No | 8080 | HTTP listen port |
| `TAAS_ENV` | No | production | Environment (production/staging/development) |
| `TAAS_DATABASE_URL` | **Yes** | — | PostgreSQL connection string |
| `TAAS_REDIS_URL` | **Yes** | — | Redis connection string |
| `TAAS_REDIS_PASSWORD` | No | — | Redis password (if separate from URL) |
| `TAAS_NATS_URL` | No | — | NATS server URL (enables async billing) |
| `TAAS_JWT_SIGNING_KEY` | **Yes** | — | JWT HMAC signing key (≥32 chars) |
| `TAAS_JWT_EXPIRY_SECONDS` | No | 3600 | Access token TTL |
| `TAAS_REFRESH_TOKEN_EXPIRY_DAYS` | No | 30 | Refresh token TTL |
| `TAAS_CORS_ALLOWED_ORIGINS` | No | [] | Allowed CORS origins (comma-separated) |
| `TAAS_DYNAMO_FRONTEND_URL` | No | — | NVIDIA Dynamo Frontend endpoint |
| `TAAS_OTLP_ENDPOINT` | No | — | OpenTelemetry collector endpoint |
| `TAAS_LOG_LEVEL` | No | info | Log level (debug/info/warn/error) |

## Step 1: Database Setup

```bash
# Create database
createdb -h $PG_HOST -U postgres taas

# Run migrations
export TAAS_DATABASE_URL="postgres://taas:password@$PG_HOST:5432/taas?sslmode=require"
make migrate-up
```

## Step 2: Install via Helm

```bash
# Add the TaaS Helm repository (or use local chart)
helm install taas deploy/helm/taas \
  --namespace taas-system \
  --create-namespace \
  -f deploy/helm/taas/values.yaml \
  --set gateway.env.TAAS_DATABASE_URL="$TAAS_DATABASE_URL" \
  --set gateway.env.TAAS_REDIS_URL="redis://redis:6379" \
  --set gateway.env.TAAS_NATS_URL="nats://nats:4222" \
  --set gateway.env.TAAS_JWT_SIGNING_KEY="$(openssl rand -base64 48)" \
  --set gateway.env.TAAS_DYNAMO_FRONTEND_URL="http://dynamo-frontend:8000" \
  --set gateway.env.TAAS_CORS_ALLOWED_ORIGINS="https://dashboard.taas.io"
```

## Step 3: TLS / Ingress

The Helm chart includes an Ingress resource. Configure TLS:

```yaml
# values-prod.yaml
gateway:
  ingress:
    enabled: true
    className: nginx
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
    hosts:
      - host: api.taas.io
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: taas-api-tls
        hosts:
          - api.taas.io
```

## Step 4: Monitoring

### Prometheus Scraping

The gateway exposes metrics at `/metrics`. Add scrape config:

```yaml
scrape_configs:
  - job_name: taas-gateway
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        regex: taas-gateway
        action: keep
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
        target_label: __address__
```

### Key Metrics to Alert On

| Metric | Condition | Severity |
|--------|-----------|----------|
| `taas_inference_latency_seconds{quantile="0.99"}` | > SLA threshold | Critical |
| `taas_rate_limit_hits_total` | Spike | Warning |
| `taas_inference_requests_total{status="error"}` | > 1% of total | Critical |
| `taas_inference_active_requests` | Near max capacity | Warning |

## Step 5: Scaling

### Horizontal Pod Autoscaler

Included in Helm chart. Tune thresholds:

```yaml
gateway:
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 20
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80
```

### Database Connection Pool

Default: 25 open, 5 idle. For high traffic:

```bash
TAAS_DB_MAX_OPEN_CONNS=100
TAAS_DB_MAX_IDLE_CONNS=25
```

## Backup & Recovery

### Database
```bash
# Backup
pg_dump -h $PG_HOST -U taas taas | gzip > taas_backup_$(date +%Y%m%d).sql.gz

# Restore
gunzip -c taas_backup_20260404.sql.gz | psql -h $PG_HOST -U taas taas
```

### Redis
Redis data is cache-only (token cache, rate limit counters). No backup needed — cache repopulates automatically from PostgreSQL.
