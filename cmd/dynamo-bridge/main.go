package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/billing"
	dynamoClient "github.com/taas-platform/taas/internal/dynamo"
	"github.com/taas-platform/taas/internal/monitoring"
	"github.com/taas-platform/taas/pkg/config"
	"github.com/taas-platform/taas/pkg/middleware"
)

// Dynamo Bridge: lightweight service between LiteLLM Proxy and NVIDIA Dynamo.
//
// Responsibilities:
//   - Accept OpenAI-compatible requests from LiteLLM
//   - Extract tenant metadata from LiteLLM headers (litellm_metadata)
//   - Inject Dynamo-specific tenant headers (X-Tenant-ID, X-Org-ID, etc.)
//   - Forward to NVIDIA Dynamo Frontend
//   - Publish usage events to NATS for billing
//
// This replaces the proxy + quota layers from the original TaaS gateway.
// LiteLLM handles: API key auth, rate limiting, cost tracking, virtual keys.
// Dynamo Bridge handles: Dynamo-specific header injection and usage telemetry.

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync() //nolint:errcheck

	cfg, err := config.Load("dynamo-bridge")
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── NATS JetStream (for usage events) ──────────────────────
	var usagePublisher *billing.Publisher
	if cfg.NATSUrl != "" {
		nc, natsErr := nats.Connect(cfg.NATSUrl)
		if natsErr != nil {
			logger.Warn("nats connection failed, usage publishing disabled", zap.Error(natsErr))
		} else {
			defer nc.Close()
			js, jsErr := nc.JetStream()
			if jsErr != nil {
				logger.Warn("jetstream init failed, usage publishing disabled", zap.Error(jsErr))
			} else {
				usagePublisher = billing.NewPublisher(js)
			}
		}
	}

	// ── Services ───────────────────────────────────────────────
	metrics := monitoring.NewMetrics("taas_bridge")
	costCalc := billing.NewCostCalculator(billing.PricingConfig{
		Prices: map[string][2]float64{
			"llama-3-8b":   {0.10, 0.20},
			"llama-3-70b":  {0.50, 1.00},
			"mistral-7b":   {0.10, 0.20},
			"mixtral-8x7b": {0.40, 0.80},
		},
		DefaultPrice: [2]float64{0.50, 1.00},
	})
	dc := dynamoClient.NewClient(cfg.DynamoFrontendURL)

	bridgeHandler := NewBridgeHandler(dc, usagePublisher, costCalc, metrics, logger)

	// ── Router ─────────────────────────────────────────────────
	router := gin.New()
	router.Use(
		middleware.SecurityHeaders(),
		middleware.RequestLogger(logger),
		middleware.Logger(logger),
		middleware.Recovery(logger),
		middleware.RequestID(),
	)

	// Health endpoints
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "dynamo-bridge"})
	})
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Bridge auth: validate shared secret from LiteLLM
	bridgeSecret := os.Getenv("BRIDGE_INTERNAL_KEY")
	if bridgeSecret == "" {
		bridgeSecret = "internal-bridge-key"
		logger.Warn("BRIDGE_INTERNAL_KEY not set, using default (insecure)")
	}

	// OpenAI-compatible inference routes — LiteLLM forwards here
	v1 := router.Group("/v1")
	v1.Use(bridgeAuthMiddleware(bridgeSecret, logger))
	{
		v1.POST("/chat/completions", bridgeHandler.ChatCompletions)
		v1.POST("/completions", bridgeHandler.Completions)
		v1.POST("/embeddings", bridgeHandler.Embeddings)
	}

	// ── Server ─────────────────────────────────────────────────
	port := cfg.Port
	if port == 0 {
		port = 8090
	}
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Match Dynamo inference timeout
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("dynamo-bridge starting", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down dynamo-bridge...")
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("forced shutdown", zap.Error(err))
	}
	logger.Info("dynamo-bridge stopped")
}

// bridgeAuthMiddleware validates the shared secret between LiteLLM and the bridge.
func bridgeAuthMiddleware(secret string, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		expected := "Bearer " + secret
		if auth != expected {
			logger.Warn("bridge auth failed", zap.String("remote", c.ClientIP()))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
