# TaaS Independent Code Review

**Reviewer:** Independent Agent (no prior context)  
**Date:** 2026-04-04  
**Scope:** Full codebase — Go backend, Python services, React frontend, infrastructure

---

## A. Architecture Assessment — 7/10

**Strengths:**
- Clean separation of concerns: `cmd/` → `internal/` → `pkg/` follows Go best practices
- Repository pattern correctly isolates DB from business logic
- Event-driven architecture (NATS JetStream) for usage tracking is a good choice
- OpenAI-compatible API design makes integration easy for users
- Multi-tenant design with org-level isolation

**Concerns:**
- **Monorepo gateway pattern**: All services (auth, token, model, billing, proxy) are wired into a single `cmd/gateway/main.go`. This is fine for an MVP but creates a deployment bottleneck. The architecture docs describe microservices but the implementation is a modular monolith.
- **No service mesh / inter-service auth**: The Python services (dynamo_operator, model_registry, sla_monitor) are separate FastAPI apps but there's no authentication between them and the Go gateway.
- **No async deployment pipeline**: `model.Deploy()` creates a DB record but doesn't actually trigger the Dynamo operator. The NATS event publishing for deployments isn't wired in the handler — it just returns "pending" status.

---

## B. Code Quality — 7.5/10

### Go (Good)
- Idiomatic Go: proper error wrapping with `%w`, context propagation, defer patterns
- Consistent handler structure: all handlers implement `RegisterRoutes()`
- Custom `APIError` type is well-designed with error codes, HTTP status, and cause chaining
- Structured logging (zap) used consistently

### Go (Issues)
- `uuid.MustParse()` used in handlers (model/handler.go) — will panic on bad input instead of returning 400
- `ListModels` in repository_pg.go builds SQL with `fmt.Sprintf` for WHERE clauses — not injection-vulnerable since values are parameterized, but the pattern is fragile and non-idiomatic
- The `proxy/usage.go` file contains `ExtractUsage()` which duplicates logic already in `dynamo/client.go`

### Python (Adequate)
- Clean FastAPI structure with async lifespan pattern
- Pydantic settings for configuration is good
- However, several modules reference sub-modules that are thin stubs (handlers.py, k8s_client.py reference classes but lack real Kubernetes API calls)

### Frontend (Basic but functional)
- Clean React with hooks, React Query for data fetching
- No error boundaries
- No loading states beyond what React Query provides
- CSS is comprehensive but no responsive breakpoints for mobile

---

## C. Security Review — 6/10

### ✅ Good
- Passwords hashed with bcrypt (DefaultCost = 10)
- API tokens stored as SHA-256 hashes, never in plaintext
- JWT signature algorithm verified (prevents alg=none attack)
- Rate limiting on inference endpoints (RPM + TPM)

### ⚠️ Concerns

1. **P0 — CORS wildcard in production**: Default config sets `cors_allowed_origins: ["*"]`. This allows any website to make authenticated API calls if a user's browser has a valid token.

2. **P0 — No JWT token blocklist**: Logout is a no-op (`c.JSON(200, "logged out")`). A stolen JWT remains valid until expiry. Refresh tokens cannot be invalidated server-side.

3. **P1 — Config validation too weak**: `validate()` only checks port. No validation for missing `database_url`, `jwt_signing_key`, `redis_url`. Server will crash at runtime with cryptic errors instead of failing at startup.

4. **P1 — No RBAC enforcement**: Role is stored in JWT claims but never checked. Any authenticated user can create/delete models, manage tokens, etc. The `role` field is decorative.

5. **P1 — No request body size limit**: `io.ReadAll(c.Request.Body)` in proxy handler reads unlimited body into memory. A malicious client could send gigabytes.

6. **P2 — No password complexity validation**: Only `min=8` length check. No uppercase/number/special char requirements.

7. **P2 — Timing attack on login**: `GetUserByEmail` returns nil for non-existent users. The response time differs between "user not found" (fast) and "wrong password" (slow bcrypt compare), leaking whether an email is registered.

---

## D. Production Readiness — 5/10

**Would I deploy this to production?** No. It's a solid MVP/prototype but needs hardening.

### Missing for production:
1. **Database migrations not auto-applied** — No migration runner in startup, no versioning beyond "001"
2. **No health check for external dependencies** — `/health/ready` checks DB but not Redis or NATS
3. **No graceful shutdown of NATS consumers** — Gateway defers `nc.Close()` but doesn't drain messages
4. **No TLS configuration** — Relies entirely on ingress/proxy for TLS
5. **No secrets management** — JWT signing key comes from env var. No Vault/KMS integration
6. **No connection pool monitoring** — DB pool created but no metrics on pool exhaustion
7. **Deployment flow is incomplete** — Creating a "deployment" only writes to DB, doesn't actually provision GPU resources
8. **No data retention policy** — `usage_records` table will grow unbounded

### Docker/K8s (Mostly ready):
- Helm chart is comprehensive with HPA, NetworkPolicy
- docker-compose.dev.yaml is functional for local dev
- **Bug: CI workflow references `deploy/docker/Dockerfile.go`** which was renamed — CI will fail

---

## E. Test Quality — 5/10

### What's there:
- 7 packages have tests, all pass including race detection ✅
- Token validator tests are well-structured (cache hit/miss/revoked/expired)
- Auth JWT tests cover the important edge cases
- Cost calculator tests are correct

### Critical gap:
- **13.3% total test coverage** — this is very low
- **0% coverage on proxy, dynamo, monitoring, config** — the most critical production code paths (actual inference proxying) have zero tests
- **No integration tests** — all tests use mocks. No test actually hits a real Postgres or Redis
- **No test for the streaming SSE fix** — the P0 bug fix in dynamo/client.go has no test proving it works
- **handler_test.go mocks the repository inline** instead of using the existing Repository interface, leading to duplicate mock code
- **E2E test references `/auth/me` endpoint** which doesn't exist in the router

---

## F. Bugs & Issues Found

### P0 — Critical
1. **E2E test references non-existent endpoint**: `inference_test.go` calls `/auth/me` but no such route exists in `auth/handler.go RegisterRoutes()`. Test would fail against real server.
2. **~~Dockerfile.go naming~~**: Fixed during review — was preventing `go build`.

### P1 — Important
3. **`uuid.MustParse` panics**: `model/handler.go` uses `uuid.MustParse(orgID.(string))` — if JWT claims are malformed, this panics and crashes the goroutine (recovery middleware catches it as 500, but it's a crash path).
4. **Streaming response writes after headers**: In `proxy/handler.go`, streaming responses set headers then `c.Writer.Write(buf.Bytes())`. But the SSE data was already written to `buf` by `dynamo/client.go Forward()`. This means the response is buffered, not actually streamed to the client in real-time.
5. **CostCalculator.Calculate receives `path` not `modelID`**: The proxy handler calls `costCalc.Calculate(path, promptTokens, completionTokens)` where `path` is "/v1/chat/completions" — but the calculator's price map uses model IDs like "llama-3-8b". Cost will always use default pricing.
6. **docker-compose mounts migrations as initdb.d**: `../../migrations:/docker-entrypoint-initdb.d` — but migration files are `.up.sql` / `.down.sql`, and Postgres initdb only runs `.sql` files alphabetically. The `.down.sql` will run after `.up.sql`, dropping all tables.

### P2 — Minor
7. **Python services have no `__init__.py`** in some subdirectories — imports may fail depending on Python path configuration.
8. **Frontend stores access_token in localStorage** — vulnerable to XSS. HttpOnly cookies would be safer.
9. **No pagination in several list endpoints** — `ListByOrg` accepts limit/offset but handler hardcodes `limit=100, offset=0`.
10. **`proxy/usage.go` is dead code** — `ExtractUsage()` is never called anywhere.

---

## G. Overall Score: 6.5/10

### Justification:
This is a **competent MVP/prototype** that demonstrates understanding of the domain (multi-tenant inference platform) and makes mostly correct architectural choices. The Go code is well-structured and idiomatic. The security fundamentals are present (hashed passwords, hashed tokens, JWT validation).

However, it falls short of production quality due to:
- Very low test coverage (13.3%), especially on critical paths
- Several logic bugs (cost calculator, streaming, docker-compose)
- Missing RBAC enforcement
- Incomplete deployment pipeline (DB record only, no actual GPU provisioning)
- Security gaps (CORS wildcard, no token blocklist, no body size limits)

### Comparison:
- **vs. hackathon project**: Significantly better — clean architecture, proper error handling, comprehensive API spec
- **vs. startup MVP**: On par — good enough to demo, needs 2-4 weeks of hardening before real users
- **vs. professional production service**: Below standard — coverage too low, security gaps, incomplete features

### Priority fixes before production:
1. Fix CORS default (change from `*` to explicit origins)
2. Add request body size limits
3. Implement RBAC middleware
4. Fix CostCalculator model ID bug
5. Fix docker-compose migration ordering
6. Add integration tests for proxy/dynamo path
7. Implement JWT token blocklist for logout/revocation
8. Replace `uuid.MustParse` with error-returning parse in handlers

---

*Review conducted by examining all 108 files in the repository. Build verified: `go build ./...` passes. Tests verified: `go test -race ./...` passes. Coverage measured: 13.3%.*
