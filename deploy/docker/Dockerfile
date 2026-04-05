# syntax=docker/dockerfile:1.7
# Multi-stage Dockerfile for all Go services.
# Usage: docker build --build-arg SERVICE=gateway -t taas-gateway .

ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.20
ARG SERVICE=gateway

# ─── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:${GO_VERSION}-alpine AS builder

ARG SERVICE
ARG VERSION=dev
ARG COMMIT=unknown

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache dependency downloads
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

# Copy source and build
COPY . .
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w -X github.com/taas-platform/taas/pkg/version.Version=${VERSION} -X github.com/taas-platform/taas/pkg/version.Commit=${COMMIT}" \
      -trimpath \
      -o /out/service \
      ./cmd/${SERVICE}/...

# ─── Stage 2: Runtime ────────────────────────────────────────────────────────
FROM alpine:${ALPINE_VERSION} AS runtime

ARG SERVICE
LABEL org.opencontainers.image.title="taas-${SERVICE}"
LABEL org.opencontainers.image.source="https://github.com/taas-platform/taas"

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1001 taas && \
    adduser -D -u 1001 -G taas taas

WORKDIR /app
COPY --from=builder /out/service /app/service

USER taas

EXPOSE 8080 9090

ENTRYPOINT ["/app/service"]
