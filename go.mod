module github.com/taas-platform/taas

go 1.23

require (
	github.com/gin-gonic/gin v1.10.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.6.0
	github.com/nats-io/nats.go v1.37.0
	github.com/prometheus/client_golang v1.20.0
	github.com/redis/go-redis/v9 v9.6.1
	github.com/spf13/viper v1.19.0
	go.opentelemetry.io/otel v1.30.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.30.0
	go.opentelemetry.io/otel/sdk v1.30.0
	go.opentelemetry.io/otel/trace v1.30.0
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.27.0
	google.golang.org/grpc v1.67.0
)
