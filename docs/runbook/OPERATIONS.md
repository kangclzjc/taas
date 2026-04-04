# TaaS Operations Runbook

## On-Call Quick Reference

| Issue | Alert | First Action |
|-------|-------|--------------|
| Inference 5xx spike | `InferenceErrorRateHigh` | Check Dynamo worker status, `kubectl get dw -n taas-dynamo` |
| p99 latency breach | `SLAViolation` | Check queue depth, GPU utilization, scale up workers |
| Rate limit storm | `RateLimitHitsHigh` | Verify client behavior, check if token misconfigured |
| DB connection pool exhausted | `DBPoolExhausted` | Check slow queries, increase pool size, restart services |
| Redis OOM | `RedisMemoryHigh` | Check key TTLs, purge stale rate limit keys, increase memory |
| NATS lag | `NATSConsumerLag` | Restart billing consumer, check DB write throughput |

## Common Operations

### Scale a Dynamo Worker deployment
```bash
kubectl patch dynamoworker <name> -n taas-dynamo \
  --type=merge -p '{"spec":{"replicas":{"target":4}}}'
```

### Revoke a compromised API token
```bash
# Via API
curl -X DELETE https://api.taas.io/tokens/<token_id> \
  -H "Authorization: Bearer <admin_jwt>"

# Directly in Redis (immediate effect)
redis-cli -a $REDIS_PASSWORD DEL "token:<token_hash>"
```

### Drain a deployment (graceful)
```bash
# Undeploy via API — drains in-flight requests first
curl -X POST https://api.taas.io/models/<model_id>/undeploy \
  -H "Authorization: Bearer <admin_jwt>"
```

### View real-time GPU utilization
```bash
kubectl exec -it <worker-pod> -n taas-dynamo -- nvidia-smi dmon -s u
```

### Manual DB migration
```bash
export DATABASE_URL="postgresql://taas:$PG_PASSWORD@postgres:5432/taas"
migrate -path db/migrations -database "$DATABASE_URL" up
```

### Emergency circuit break (stop all inference)
```bash
# Set a global kill switch in Redis
redis-cli -a $REDIS_PASSWORD SET "circuit_breaker:inference" 1 EX 3600
# All Inference Proxy instances check this key on startup of each request
```

## SLA Tier Escalation

When a Professional/Enterprise deployment breaches SLA:
1. Alert fires: `SLAViolation{tier="professional"}`
2. SLA Monitor publishes to `sla.violation.<deployment_id>`
3. Dynamo Operator increases replicas to max
4. If still breaching after 5 minutes, page on-call
5. If hardware-limited, notify customer and apply SLA credit

## Monitoring Links

- Grafana: https://grafana.taas.io
- Alertmanager: https://alertmanager.taas.io
- Jaeger: https://tracing.taas.io
- NATS monitoring: https://nats.taas.io:8222

## Backup & Recovery

### PostgreSQL
- Automated WAL archiving to S3 every 5 minutes
- Daily full snapshot at 02:00 UTC
- PITR retention: 14 days
- Recovery: `pg_restore` from snapshot + WAL replay

### Redis
- RDB snapshots every 60 seconds to PVC
- For cache data: acceptable to lose (rebuild from DB)
- For session data: same

### Model Artifacts (S3)
- Versioned bucket with 30-day delete protection
- Cross-region replication enabled for Enterprise tier
