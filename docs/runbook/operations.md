# TaaS Operations Runbook

## Health Check Endpoints

| Endpoint | Purpose | Expected |
|----------|---------|----------|
| `GET /health` | Liveness probe | `{"status": "ok"}` |
| `GET /health/ready` | Readiness probe (checks DB) | `{"status": "ready"}` |
| `GET /metrics` | Prometheus metrics | Prometheus text format |

## Common Issues

### Token Validation Failures

**Symptom:** Users get 401 "invalid API key" despite having a valid token.

**Diagnosis:**
```bash
# Check Redis connectivity
redis-cli -u $TAAS_REDIS_URL ping

# Check if token is cached (stale cache after revocation)
redis-cli -u $TAAS_REDIS_URL GET "token:<sha256_hash>"

# Check DB for token status
psql -c "SELECT id, prefix, is_active, expires_at FROM api_tokens WHERE prefix LIKE 'taas_Ab3k%';"
```

**Fix:** If stale cache, wait 5 minutes (cache TTL) or flush manually:
```bash
redis-cli -u $TAAS_REDIS_URL DEL "token:<hash>"
```

### Rate Limit Too Aggressive

**Symptom:** Legitimate users hitting 429 too often.

**Diagnosis:**
```bash
# Check current rate limit for a token
redis-cli -u $TAAS_REDIS_URL ZCARD "ratelimit:rpm:<token_id>"

# Check token's configured limits
psql -c "SELECT name, rate_limit_rpm, rate_limit_tpm FROM api_tokens WHERE id = '<token_id>';"
```

**Fix:** Update token limits:
```sql
UPDATE api_tokens SET rate_limit_rpm = 200, rate_limit_tpm = 1000000 WHERE id = '<token_id>';
```
Then flush Redis cache for that token.

### DB Connection Pool Exhaustion

**Symptom:** Requests fail with "too many connections" or timeouts.

**Diagnosis:**
```sql
SELECT count(*) FROM pg_stat_activity WHERE datname = 'taas';
SELECT state, count(*) FROM pg_stat_activity WHERE datname = 'taas' GROUP BY state;
```

**Fix:**
1. Increase pool size: `TAAS_DB_MAX_OPEN_CONNS=100`
2. Check for connection leaks (long-running idle connections)
3. Restart gateway pods: `kubectl rollout restart deployment/taas-gateway`

### High Inference Latency

**Symptom:** P99 latency exceeding SLA thresholds.

**Diagnosis:**
```promql
histogram_quantile(0.99, rate(taas_inference_latency_seconds_bucket[5m]))
```

**Fix:**
1. Check Dynamo worker health: `kubectl get pods -l app=dynamo-worker`
2. Scale up decode workers
3. Check GPU utilization: `nvidia-smi`
4. Verify no resource contention from other workloads

---

## Emergency Procedures

### Revoke All Tokens for an Org
```sql
UPDATE api_tokens SET is_active = false, updated_at = NOW() WHERE org_id = '<org_id>';
```
Then flush all token caches:
```bash
redis-cli -u $TAAS_REDIS_URL --scan --pattern "token:*" | xargs -L 1 redis-cli DEL
```

### Disable a Model
```sql
UPDATE models SET status = 'archived', updated_at = NOW() WHERE id = '<model_id>';
UPDATE deployments SET status = 'stopped', updated_at = NOW() WHERE model_id = '<model_id>';
```

### Block an Organization
```sql
UPDATE users SET is_active = false WHERE org_id = '<org_id>';
```
Active JWTs will continue working until expiry. For immediate block, add all active JTIs to the blocklist:
```bash
# Query active tokens and add to Redis blocklist
psql -t -c "SELECT id FROM users WHERE org_id = '<org_id>'" | while read uid; do
  redis-cli SET "jwt:blocked:$uid" 1 EX 3600
done
```

### Full System Pause (Circuit Breaker)
If the inference backend is overwhelmed:
```bash
# Scale gateway to 0 (stops all traffic)
kubectl scale deployment/taas-gateway --replicas=0

# Fix the issue, then restore
kubectl scale deployment/taas-gateway --replicas=2
```

---

## Log Analysis

### Key Log Fields
| Field | Description |
|-------|-------------|
| `request_id` | Unique per-request ID (also in X-Request-ID header) |
| `user_id` | Authenticated user |
| `org_id` | Organization |
| `path` | HTTP endpoint |
| `status` | HTTP status code |
| `latency` | Request duration |

### Common Error Patterns

```bash
# Find all 5xx errors in last hour
journalctl -u taas-gateway --since "1 hour ago" | grep '"status":5'

# Find rate-limited requests
journalctl -u taas-gateway --since "1 hour ago" | grep "RATE_LIMIT_EXCEEDED"

# Find token validation failures
journalctl -u taas-gateway --since "1 hour ago" | grep "token validation failed"

# Find Dynamo backend errors
journalctl -u taas-gateway --since "1 hour ago" | grep "dynamo forward error"
```

---

## Monitoring Alerts

### Recommended Alert Rules

```yaml
groups:
  - name: taas-alerts
    rules:
      - alert: TaaSHighErrorRate
        expr: sum(rate(taas_inference_requests_total{status="error"}[5m])) / sum(rate(taas_inference_requests_total[5m])) > 0.01
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "TaaS error rate > 1%"

      - alert: TaaSHighLatency
        expr: histogram_quantile(0.99, rate(taas_inference_latency_seconds_bucket[5m])) > 5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "TaaS P99 latency > 5s"

      - alert: TaaSRateLimitSpike
        expr: rate(taas_rate_limit_hits_total[5m]) > 10
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Rate limiting spike detected"

      - alert: TaaSGatewayDown
        expr: up{job="taas-gateway"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "TaaS gateway instance down"
```
