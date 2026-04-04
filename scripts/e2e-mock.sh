#!/bin/bash
set -e

# TaaS End-to-End Test (with Mock Dynamo, no GPU required)
# Usage: ./scripts/e2e-mock.sh

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'
BOLD='\033[1m'

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
MOCK_URL="${MOCK_URL:-http://localhost:8090}"
PASS=0
FAIL=0
TOTAL=0

pass() { ((PASS++)); ((TOTAL++)); echo -e "  ${GREEN}✅ PASS${NC} — $1"; }
fail() { ((FAIL++)); ((TOTAL++)); echo -e "  ${RED}❌ FAIL${NC} — $1: $2"; }

check_status() {
    local desc="$1" expected="$2" actual="$3" body="$4"
    if [ "$actual" = "$expected" ]; then
        pass "$desc"
    else
        fail "$desc" "expected $expected, got $actual — $body"
    fi
}

echo -e "\n${BOLD}🧪 TaaS End-to-End Test Suite${NC}"
echo "   Gateway: $GATEWAY_URL"
echo "   Mock Dynamo: $MOCK_URL"
echo ""

# ────────────────────────────────────────────────────────────
echo -e "${BOLD}1. Health Checks${NC}"
# ────────────────────────────────────────────────────────────

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/health")
check_status "GET /health" "200" "$STATUS"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$MOCK_URL/health")
check_status "Mock Dynamo health" "200" "$STATUS"

# ────────────────────────────────────────────────────────────
echo -e "\n${BOLD}2. Auth — Register & Login${NC}"
# ────────────────────────────────────────────────────────────

EMAIL="e2e-$(date +%s)@test.taas.io"
PASSWORD='TestPass1@Secure'

# Register
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"$EMAIL\", \"password\": \"$PASSWORD\"}")
BODY=$(echo "$RESP" | head -1)
STATUS=$(echo "$RESP" | tail -1)
check_status "Register new user" "201" "$STATUS"

ACCESS_TOKEN=$(echo "$BODY" | jq -r '.access_token // empty')
REFRESH_TOKEN=$(echo "$BODY" | jq -r '.refresh_token // empty')

if [ -n "$ACCESS_TOKEN" ]; then
    pass "Got access token"
else
    fail "Got access token" "token is empty"
fi

# Login
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"$EMAIL\", \"password\": \"$PASSWORD\"}")
STATUS=$(echo "$RESP" | tail -1)
check_status "Login" "200" "$STATUS"

# Login with wrong password
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"$EMAIL\", \"password\": \"WrongPass1@\"}")
STATUS=$(echo "$RESP" | tail -1)
check_status "Login wrong password → 401" "401" "$STATUS"

# Duplicate register
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"$EMAIL\", \"password\": \"$PASSWORD\"}")
STATUS=$(echo "$RESP" | tail -1)
check_status "Duplicate register → 409" "409" "$STATUS"

# Refresh token
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/auth/refresh" \
  -H "Content-Type: application/json" \
  -d "{\"refresh_token\": \"$REFRESH_TOKEN\"}")
BODY=$(echo "$RESP" | head -1)
STATUS=$(echo "$RESP" | tail -1)
check_status "Refresh token" "200" "$STATUS"
ACCESS_TOKEN=$(echo "$BODY" | jq -r '.access_token // empty')

# ────────────────────────────────────────────────────────────
echo -e "\n${BOLD}3. Token Management${NC}"
# ────────────────────────────────────────────────────────────

# Create API token
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/tokens" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "e2e-test-key", "rate_limit_rpm": 100, "rate_limit_tpm": 500000}')
BODY=$(echo "$RESP" | head -1)
STATUS=$(echo "$RESP" | tail -1)
check_status "Create API token" "201" "$STATUS"

API_KEY=$(echo "$BODY" | jq -r '.key // empty')
TOKEN_ID=$(echo "$BODY" | jq -r '.token.id // empty')

if [ -n "$API_KEY" ] && [[ "$API_KEY" == taas_* ]]; then
    pass "API key format correct (taas_...)"
else
    fail "API key format" "got: $API_KEY"
fi

# List tokens
RESP=$(curl -s -w "\n%{http_code}" "$GATEWAY_URL/tokens" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
STATUS=$(echo "$RESP" | tail -1)
check_status "List tokens" "200" "$STATUS"

# ────────────────────────────────────────────────────────────
echo -e "\n${BOLD}4. Inference (via Mock Dynamo)${NC}"
# ────────────────────────────────────────────────────────────

# Chat completions
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-8b",
    "messages": [{"role": "user", "content": "Hello, this is an e2e test!"}],
    "max_tokens": 100
  }')
BODY=$(echo "$RESP" | head -1)
STATUS=$(echo "$RESP" | tail -1)
check_status "Chat completions" "200" "$STATUS"

HAS_CHOICES=$(echo "$BODY" | jq 'has("choices")')
check_status "Response has choices" "true" "$HAS_CHOICES"

HAS_USAGE=$(echo "$BODY" | jq 'has("usage")')
check_status "Response has usage" "true" "$HAS_USAGE"

PROMPT_TOKENS=$(echo "$BODY" | jq '.usage.prompt_tokens // 0')
if [ "$PROMPT_TOKENS" -gt 0 ]; then
    pass "Usage tracking works (prompt_tokens=$PROMPT_TOKENS)"
else
    fail "Usage tracking" "prompt_tokens is 0"
fi

# Completions
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/v1/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "llama-3-8b", "prompt": "Once upon a time", "max_tokens": 50}')
STATUS=$(echo "$RESP" | tail -1)
check_status "Text completions" "200" "$STATUS"

# Embeddings
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/v1/embeddings" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "llama-3-8b", "input": "The quick brown fox"}')
STATUS=$(echo "$RESP" | tail -1)
check_status "Embeddings" "200" "$STATUS"

# ────────────────────────────────────────────────────────────
echo -e "\n${BOLD}5. Auth Enforcement${NC}"
# ────────────────────────────────────────────────────────────

# No auth → 401
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model": "x", "messages": [{"role": "user", "content": "test"}]}')
STATUS=$(echo "$RESP" | tail -1)
check_status "No API key → 401" "401" "$STATUS"

# Invalid API key → 401
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/v1/chat/completions" \
  -H "Authorization: Bearer taas_invalidkey12345" \
  -H "Content-Type: application/json" \
  -d '{"model": "x", "messages": [{"role": "user", "content": "test"}]}')
STATUS=$(echo "$RESP" | tail -1)
check_status "Invalid API key → 401" "401" "$STATUS"

# No JWT → 401 on management endpoints
RESP=$(curl -s -w "\n%{http_code}" "$GATEWAY_URL/tokens")
STATUS=$(echo "$RESP" | tail -1)
check_status "No JWT on /tokens → 401" "401" "$STATUS"

# ────────────────────────────────────────────────────────────
echo -e "\n${BOLD}6. Usage / Billing${NC}"
# ────────────────────────────────────────────────────────────

RESP=$(curl -s -w "\n%{http_code}" "$GATEWAY_URL/usage/summary" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
STATUS=$(echo "$RESP" | tail -1)
check_status "Usage summary" "200" "$STATUS"

RESP=$(curl -s -w "\n%{http_code}" "$GATEWAY_URL/usage/by-model" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
STATUS=$(echo "$RESP" | tail -1)
check_status "Usage by model" "200" "$STATUS"

# ────────────────────────────────────────────────────────────
echo -e "\n${BOLD}7. Cleanup — Token Revocation & Logout${NC}"
# ────────────────────────────────────────────────────────────

# Revoke API token
RESP=$(curl -s -w "\n%{http_code}" -X DELETE "$GATEWAY_URL/tokens/$TOKEN_ID" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
STATUS=$(echo "$RESP" | tail -1)
check_status "Revoke API token" "200" "$STATUS"

# Revoked token should fail
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "x", "messages": [{"role": "user", "content": "test"}]}')
STATUS=$(echo "$RESP" | tail -1)
check_status "Revoked API key → 401" "401" "$STATUS"

# Logout
RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/auth/logout" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
STATUS=$(echo "$RESP" | tail -1)
check_status "Logout" "200" "$STATUS"

# ────────────────────────────────────────────────────────────
echo -e "\n${BOLD}════════════════════════════════════════${NC}"
echo -e "${BOLD}  Results: $PASS passed, $FAIL failed (total $TOTAL)${NC}"
echo -e "${BOLD}════════════════════════════════════════${NC}"

if [ "$FAIL" -gt 0 ]; then
    echo -e "${RED}  ❌ SOME TESTS FAILED${NC}"
    exit 1
else
    echo -e "${GREEN}  ✅ ALL TESTS PASSED${NC}"
    exit 0
fi
