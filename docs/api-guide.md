# TaaS API Guide

## Authentication

### Register a new user
```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "alice@example.com",
    "password": "MySecure1@Pass"
  }'
```
Response:
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

### Login
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "alice@example.com", "password": "MySecure1@Pass"}'
```

### Refresh token
```bash
curl -X POST http://localhost:8080/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "eyJhbGciOiJIUzI1NiIs..."}'
```

### Logout (revokes token)
```bash
curl -X POST http://localhost:8080/auth/logout \
  -H "Authorization: Bearer <access_token>"
```

---

## API Token Management

All token endpoints require JWT authentication.

### Create an API token
```bash
curl -X POST http://localhost:8080/tokens \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-key",
    "rate_limit_rpm": 100,
    "rate_limit_tpm": 500000,
    "sla_tier": "professional",
    "allowed_models": ["llama-3-70b"],
    "scopes": ["inference"]
  }'
```
Response (raw key shown **once**):
```json
{
  "token": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "production-key",
    "prefix": "taas_Ab3kX9mQ",
    "sla_tier": "professional",
    "is_active": true
  },
  "key": "taas_Ab3kX9mQr7Yw2pLm5nK8jH4vF6tG1cD0sE3qR9wU7iO"
}
```

### List tokens
```bash
curl http://localhost:8080/tokens \
  -H "Authorization: Bearer <access_token>"
```

### Revoke a token
```bash
curl -X DELETE http://localhost:8080/tokens/<token_id> \
  -H "Authorization: Bearer <access_token>"
```

### Rotate a token
```bash
curl -X POST http://localhost:8080/tokens/<token_id>/rotate \
  -H "Authorization: Bearer <access_token>"
```

---

## Model Management

### Register a model
```bash
curl -X POST http://localhost:8080/models \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "LLaMA 3 70B",
    "slug": "llama-3-70b",
    "framework": "pytorch",
    "storage_uri": "s3://models/llama-3-70b",
    "parameter_count": 70000000000,
    "context_length": 8192,
    "is_public": false
  }'
```

### Deploy a model
```bash
curl -X POST http://localhost:8080/models/<model_id>/deploy \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "llama-70b-prod",
    "sla_tier": "professional",
    "replicas_min": 2,
    "replicas_max": 8,
    "gpu_type": "A100_80GB",
    "gpu_count_per_replica": 4,
    "max_batch_size": 64
  }'
```

### Share a model
```bash
curl -X POST http://localhost:8080/models/<model_id>/share \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "target_org_id": "org-uuid-here",
    "permission": "deploy"
  }'
```

---

## Inference (OpenAI-compatible)

Use your **API token** (not JWT) for inference endpoints.

### Chat completions
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer taas_Ab3kX9mQ..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-70b",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is NVIDIA Dynamo?"}
    ],
    "max_tokens": 200,
    "temperature": 0.7
  }'
```

### Streaming
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer taas_Ab3kX9mQ..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-70b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### Embeddings
```bash
curl -X POST http://localhost:8080/v1/embeddings \
  -H "Authorization: Bearer taas_Ab3kX9mQ..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-70b",
    "input": "The quick brown fox"
  }'
```

---

## Usage & Billing

### Usage summary
```bash
curl http://localhost:8080/usage/summary \
  -H "Authorization: Bearer <access_token>"
```
Response:
```json
{
  "total_requests": 15420,
  "total_prompt_tokens": 2340000,
  "total_completion_tokens": 890000,
  "total_tokens": 3230000,
  "total_cost_usd": 4.85,
  "avg_latency_ms": 245.3
}
```

### Usage by model
```bash
curl http://localhost:8080/usage/by-model \
  -H "Authorization: Bearer <access_token>"
```

### Usage by token
```bash
curl http://localhost:8080/usage/by-token \
  -H "Authorization: Bearer <access_token>"
```

---

## Rate Limit Headers

All inference responses include rate limit headers:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 87
```

When rate limited (HTTP 429):
```json
{"code": "RATE_LIMIT_EXCEEDED", "message": "rate limit exceeded"}
```
