#!/usr/bin/env bash
# Setup local development environment for TaaS.
# Run: bash scripts/setup-dev.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Setting up TaaS development environment..."

# Check dependencies
check_dep() {
  if ! command -v "$1" &>/dev/null; then
    echo "ERROR: $1 is not installed. Please install it first."
    exit 1
  fi
}

check_dep go
check_dep docker
check_dep kubectl
check_dep helm

# Start local deps
echo "==> Starting local dependencies (PostgreSQL, Redis, NATS)..."
docker compose -f "$ROOT_DIR/deploy/docker/docker-compose.dev.yaml" up -d

# Wait for postgres
echo "==> Waiting for PostgreSQL..."
until docker exec taas-postgres pg_isready -U taas &>/dev/null; do
  sleep 1
done
echo "    PostgreSQL ready."

# Wait for Redis
echo "==> Waiting for Redis..."
until docker exec taas-redis redis-cli -a redis_dev_secret ping &>/dev/null; do
  sleep 1
done
echo "    Redis ready."

# Install Go tools
echo "==> Installing Go tools..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Install Go dependencies
echo "==> Downloading Go modules..."
cd "$ROOT_DIR" && go mod download

echo ""
echo "==> Development environment ready!"
echo "    PostgreSQL: localhost:5432 (user: taas, password: taas_dev_secret)"
echo "    Redis:      localhost:6379 (password: redis_dev_secret)"
echo "    NATS:       localhost:4222"
echo "    Grafana:    http://localhost:3001 (admin/admin)"
echo "    Jaeger:     http://localhost:16686"
echo ""
echo "    Run 'make build' to build all services."
echo "    Run 'make test' to run tests."
