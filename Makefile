.PHONY: all build test lint fmt clean docker-build docker-push helm-lint deploy-dev

# Project settings
MODULE      := github.com/taas-platform/taas
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -X $(MODULE)/pkg/version.Version=$(VERSION) \
               -X $(MODULE)/pkg/version.Commit=$(COMMIT) \
               -X $(MODULE)/pkg/version.BuildTime=$(BUILD_TIME)

# Docker settings
REGISTRY    ?= ghcr.io/taas-platform
TAG         ?= $(VERSION)

# Services
SERVICES    := gateway auth token-manager billing

# Tools
GOLANGCI    := golangci-lint
HELM        := helm
KUBECTL     := kubectl

##@ General

all: lint test build ## Run lint, test, and build

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

fmt: ## Format Go code
	go fmt ./...
	goimports -w .

lint: ## Run linters
	$(GOLANGCI) run ./...

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy go modules
	go mod tidy

generate: ## Run go generate
	go generate ./...

##@ Build

build: $(addprefix build-, $(SERVICES)) ## Build all services

build-%: ## Build a specific service (e.g. make build-gateway)
	@echo "Building $*..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags "$(LDFLAGS)" \
		-o bin/$* \
		./cmd/$*/...

##@ Testing

test: ## Run unit tests
	go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Show test coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-integration: ## Run integration tests (requires running dependencies)
	go test -v -tags=integration ./test/...

test-e2e: ## Run end-to-end tests
	go test -v -tags=e2e -timeout=10m ./test/e2e/...

test-load: ## Run load tests
	cd test/load && k6 run load_test.js

##@ Docker

docker-build: $(addprefix docker-build-, $(SERVICES)) ## Build all Docker images

docker-build-%: ## Build Docker image for a specific service
	@echo "Building Docker image for $*..."
	docker build \
		--build-arg SERVICE=$* \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t $(REGISTRY)/taas-$*:$(TAG) \
		-t $(REGISTRY)/taas-$*:latest \
		-f deploy/docker/Dockerfile.go .

docker-push: $(addprefix docker-push-, $(SERVICES)) ## Push all Docker images

docker-push-%:
	docker push $(REGISTRY)/taas-$*:$(TAG)
	docker push $(REGISTRY)/taas-$*:latest

docker-build-python: ## Build Python service images
	docker build -t $(REGISTRY)/taas-dynamo-operator:$(TAG) -f deploy/docker/Dockerfile.python python/dynamo_operator
	docker build -t $(REGISTRY)/taas-model-registry:$(TAG)  -f deploy/docker/Dockerfile.python python/model_registry
	docker build -t $(REGISTRY)/taas-sla-monitor:$(TAG)     -f deploy/docker/Dockerfile.python python/sla_monitor

##@ Helm

helm-lint: ## Lint Helm charts
	$(HELM) lint deploy/helm/taas

helm-template: ## Render Helm templates
	$(HELM) template taas deploy/helm/taas --values deploy/helm/taas/values-dev.yaml

helm-install-dev: ## Install/upgrade TaaS in dev namespace
	$(HELM) upgrade --install taas deploy/helm/taas \
		--namespace taas-dev \
		--create-namespace \
		--values deploy/helm/taas/values-dev.yaml \
		--set image.tag=$(TAG)

helm-uninstall-dev: ## Uninstall TaaS from dev namespace
	$(HELM) uninstall taas --namespace taas-dev

##@ Database

db-migrate-up: ## Run database migrations
	migrate -path db/migrations -database "$(DATABASE_URL)" up

db-migrate-down: ## Rollback last migration
	migrate -path db/migrations -database "$(DATABASE_URL)" down 1

db-seed: ## Seed development database
	go run scripts/seed/main.go

##@ Local Development

dev-deps: ## Start local development dependencies (PostgreSQL, Redis, NATS)
	docker compose -f deploy/docker/docker-compose.dev.yaml up -d

dev-deps-down: ## Stop local development dependencies
	docker compose -f deploy/docker/docker-compose.dev.yaml down

dev-gateway: build-gateway ## Run gateway locally
	./bin/gateway --config configs/gateway.dev.yaml

dev-auth: build-auth ## Run auth service locally
	./bin/auth --config configs/auth.dev.yaml

##@ Cleanup

clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html
	go clean -cache

clean-all: clean ## Remove build artifacts and Docker images
	docker rmi $(addprefix $(REGISTRY)/taas-, $(addsuffix :$(TAG), $(SERVICES))) 2>/dev/null || true
