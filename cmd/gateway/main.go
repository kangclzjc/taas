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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/audit"
	"github.com/taas-platform/taas/internal/auth"
	"github.com/taas-platform/taas/internal/billing"
	litellmPkg "github.com/taas-platform/taas/internal/litellm"
	"github.com/taas-platform/taas/internal/model"
	"github.com/taas-platform/taas/internal/monitoring"
	"github.com/taas-platform/taas/internal/natsutil"
	"github.com/taas-platform/taas/internal/org"
	"github.com/taas-platform/taas/internal/telemetry"
	"github.com/taas-platform/taas/internal/token"
	"github.com/taas-platform/taas/pkg/config"
	"github.com/taas-platform/taas/pkg/middleware"

	// Deprecated: proxy and quota are now handled by LiteLLM Proxy + Dynamo Bridge.
	// These imports are kept for backward compatibility when LITELLM_ENABLED=false.
	dynamoClient "github.com/taas-platform/taas/internal/dynamo"
	"github.com/taas-platform/taas/internal/proxy"
	"github.com/taas-platform/taas/internal/quota"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync() //nolint:errcheck

	cfg, err := config.Load("gateway")
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── OpenTelemetry Tracing ──────────────────────────────────
	tp, err := telemetry.InitTracer(ctx, "taas-gateway", cfg.OTLPEndpoint)
	if err != nil {
		logger.Warn("failed to init tracer", zap.Error(err))
	}
	if tp != nil {
		defer func() {
			if shutdownErr := tp.Shutdown(context.Background()); shutdownErr != nil {
				logger.Error("tracer shutdown error", zap.Error(shutdownErr))
			}
		}()
	}

	// ── Database ────────────────────────────────────────────────
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("failed to parse database config", zap.Error(err))
	}
	poolConfig.MaxConns = int32(cfg.DBMaxOpenConns) // max open connections (P3: clarified naming)
	poolConfig.MinConns = int32(cfg.DBMaxIdleConns) // pre-warmed idle connections

	dbPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer dbPool.Close()

	// Register DB pool metrics with Prometheus
	prometheus.MustRegister(monitoring.NewDBPoolCollector(dbPool, "taas"))

	// ── Redis ──────────────────────────────────────────────────
	rdbOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Fatal("failed to parse redis url", zap.Error(err))
	}
	if cfg.RedisPassword != "" {
		rdbOpts.Password = cfg.RedisPassword
	}
	rdb := redis.NewClient(rdbOpts)
	defer rdb.Close()

	// ── NATS JetStream ─────────────────────────────────────────
	var usagePublisher *billing.Publisher
	var nc *nats.Conn
	var js nats.JetStreamContext
	if cfg.NATSUrl != "" {
		var natsErr error
		nc, natsErr = nats.Connect(cfg.NATSUrl)
		if natsErr != nil {
			logger.Warn("failed to connect to nats, billing disabled", zap.Error(natsErr))
			nc = nil
		} else {
			defer nc.Close()
			var jsErr error
			js, jsErr = nc.JetStream()
			if jsErr != nil {
				logger.Warn("failed to init jetstream, billing disabled", zap.Error(jsErr))
			} else {
				if streamErr := natsutil.EnsureTAASEventsStream(js); streamErr != nil {
					logger.Warn("failed to ensure jetstream stream", zap.Error(streamErr))
				}
				usagePublisher = billing.NewPublisher(js)
			}
		}
	}

	// ── Services ───────────────────────────────────────────────
	metrics := monitoring.NewMetrics("taas")

	// Determine if LiteLLM mode is enabled
	litellmEnabled := os.Getenv("LITELLM_ENABLED") == "true"
	litellmProxyURL := os.Getenv("LITELLM_PROXY_URL") // e.g., http://litellm:4000
	litellmMasterKey := os.Getenv("LITELLM_MASTER_KEY")
	litellmWebhookSecret := os.Getenv("LITELLM_WEBHOOK_SECRET")

	if litellmEnabled {
		logger.Info("LiteLLM integration enabled",
			zap.String("proxy_url", litellmProxyURL),
		)
	}

	jwtSvc := auth.NewJWTService(cfg.JWTSigningKey, cfg.JWTExpirySeconds, cfg.RefreshTokenExpiryDays)

	// Security: JWT blocklist for real token revocation
	blocklist := auth.NewBlocklist(rdb)

	// Security: Login rate limiting (5 attempts, 15min window, 30min lockout)
	loginRateLimiter := auth.NewLoginRateLimiter(rdb, 5, 15*time.Minute, 30*time.Minute)

	// Security: Audit logger
	auditLogger := audit.New(logger, dbPool)

	authRepo := auth.NewRepository(dbPool)
	authHandler := auth.NewHandler(authRepo, jwtSvc, logger, blocklist, loginRateLimiter, auditLogger)

	tokenRepo := token.NewRepository(dbPool)
	tokenValidator := token.NewValidator(rdb, tokenRepo.LookupForValidation)
	tokenSvc := token.NewService(tokenRepo, tokenValidator, logger)

	// LiteLLM admin client (shared by token and model services)
	var litellmAdmin *litellmPkg.AdminClient
	if litellmEnabled && litellmProxyURL != "" && litellmMasterKey != "" {
		litellmAdmin = litellmPkg.NewAdminClient(litellmProxyURL, litellmMasterKey, logger)
	}

	// Token handler: with or without LiteLLM sync
	var tokenHandler *token.Handler
	if litellmAdmin != nil {
		litellmTokenSvc := token.NewLiteLLMService(tokenSvc, litellmAdmin, logger)
		tokenHandler = token.NewLiteLLMHandler(litellmTokenSvc, logger)
		logger.Info("token service: LiteLLM virtual key sync enabled")
	} else {
		tokenHandler = token.NewHandler(tokenSvc, logger)
		if litellmEnabled {
			logger.Warn("LiteLLM enabled but missing LITELLM_PROXY_URL or LITELLM_MASTER_KEY, token sync disabled")
		}
	}

	modelRepo := model.NewPGRepository(dbPool)
	sharingService := model.NewSharingService(dbPool)
	modelSvc := model.NewService(modelRepo, sharingService)
	modelHandler := model.NewHandler(modelSvc, sharingService, logger)

	// Model LiteLLM sync: registers Dynamo endpoints in LiteLLM when deployments become ready
	var modelLiteLLMSvc *model.LiteLLMService
	if litellmAdmin != nil {
		modelLiteLLMSvc = model.NewLiteLLMService(modelSvc, litellmAdmin, modelRepo, logger)
		logger.Info("model service: LiteLLM model sync enabled")
	}

	if js != nil {
		modelSvc.SetDeploymentPublisher(model.NewNATSDeploymentPublisher(js, logger))
		_, subErr := model.StartDeploymentStatusConsumer(
			ctx,
			js,
			modelSvc,
			logger,
			func(hookCtx context.Context, deploymentID uuid.UUID, endpointURL string) error {
				if modelLiteLLMSvc == nil {
					return nil
				}
				return modelLiteLLMSvc.OnDeploymentRunning(hookCtx, deploymentID, endpointURL)
			},
		)
		if subErr != nil {
			logger.Warn("failed to start deployment status consumer", zap.Error(subErr))
		}
	} else {
		logger.Warn("NATS unavailable: model deploy requests will remain pending")
	}
	// Note: modelLiteLLMSvc.OnDeploymentRunning() should be called from the NATS
	// deployment.status.updated consumer when status == "running".
	// modelLiteLLMSvc.OnDeploymentStopped() when status == "stopped" or "failed".
	_ = modelLiteLLMSvc // available for NATS consumer wiring

	// Deprecated: These are only used when LITELLM_ENABLED=false (legacy mode).
	// In LiteLLM mode, rate limiting and proxy are handled by LiteLLM Proxy + Dynamo Bridge.
	rateLimiter := quota.NewRateLimiter(rdb)

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
	proxyHandler := proxy.NewHandler(dc, usagePublisher, costCalc, metrics, logger)

	billingHandler := billing.NewHandler(dbPool, logger)

	// LiteLLM webhook handler (receives usage callbacks from LiteLLM Proxy)
	var litellmWebhook *litellmPkg.WebhookHandler
	if litellmEnabled {
		if litellmWebhookSecret == "" {
			litellmWebhookSecret = "change-me-webhook-secret"
			logger.Warn("LITELLM_WEBHOOK_SECRET not set, using default (insecure)")
		}
		litellmWebhook = litellmPkg.NewWebhookHandler(dbPool, litellmWebhookSecret, logger)
	}

	// ── Organization ───────────────────────────────────────────
	orgRepo := org.NewRepository(dbPool)
	orgHandler := org.NewHandler(orgRepo, authRepo, logger)

	// ── Router ─────────────────────────────────────────────────
	router := gin.New()
	router.Use(
		middleware.SecurityHeaders(),
		middleware.RequestLogger(logger),
		middleware.Logger(logger),
		middleware.Recovery(logger),
		middleware.RequestID(),
		middleware.Cors(cfg.CORSAllowedOrigins),
	)

	// Public endpoints
	router.GET("/health", healthHandler)
	router.GET("/health/ready", func(c *gin.Context) {
		checks := make(map[string]string)
		allHealthy := true

		// DB check
		if pingErr := dbPool.Ping(c.Request.Context()); pingErr != nil {
			checks["database"] = "unhealthy: " + pingErr.Error()
			allHealthy = false
		} else {
			checks["database"] = "healthy"
		}

		// Redis check
		if pingErr := rdb.Ping(c.Request.Context()).Err(); pingErr != nil {
			checks["redis"] = "unhealthy: " + pingErr.Error()
			allHealthy = false
		} else {
			checks["redis"] = "healthy"
		}

		// NATS check (if configured)
		if nc != nil {
			if nc.IsConnected() {
				checks["nats"] = "healthy"
			} else {
				checks["nats"] = "unhealthy: not connected"
				allHealthy = false
			}
		}

		status := http.StatusOK
		if !allHealthy {
			status = http.StatusServiceUnavailable
		}

		c.JSON(status, gin.H{"status": checks, "healthy": allHealthy})
	})
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Auth routes (public)
	authGroup := router.Group("/auth")
	authHandler.RegisterRoutes(authGroup)

	// JWT-authenticated routes
	jwtAuth := router.Group("")
	jwtAuth.Use(auth.JWTMiddleware(jwtSvc, blocklist))
	{
		authHandler.RegisterProtectedRoutes(jwtAuth.Group("/auth"))

		// Token management: all authenticated users can list, but only members+ can create/delete (P2: RBAC)
		tokenGroup := jwtAuth.Group("/tokens")
		tokenGroup.GET("", tokenHandler.List)
		tokenGroup.POST("", auth.RequireWriteAccess(), tokenHandler.Create)
		tokenGroup.DELETE("/:id", auth.RequireWriteAccess(), tokenHandler.Delete)
		tokenGroup.POST("/:id/rotate", auth.RequireWriteAccess(), tokenHandler.Rotate)

		// Model management: all can list/view, but only members+ can create/delete/deploy/share (P2: RBAC)
		modelGroup := jwtAuth.Group("/models")
		modelGroup.GET("", modelHandler.ListModels)
		modelGroup.GET("/:id", modelHandler.GetModel)
		modelGroup.GET("/:id/deployments", modelHandler.ListDeployments)
		modelGroup.POST("", auth.RequireWriteAccess(), modelHandler.CreateModel)
		modelGroup.DELETE("/:id", auth.RequireWriteAccess(), modelHandler.DeleteModel)
		modelGroup.POST("/:id/deploy", auth.RequireWriteAccess(), modelHandler.DeployModel)
		modelGroup.POST("/:id/share", auth.RequireAdminAccess(), modelHandler.ShareModel)

		billingHandler.RegisterRoutes(jwtAuth.Group("/usage"))

		// Org management: all can list/view, admins+ can create/update/delete, owners can manage members (P2: RBAC)
		orgGroup := jwtAuth.Group("/organizations")
		orgGroup.GET("", orgHandler.ListOrgs)
		orgGroup.POST("", orgHandler.CreateOrg)
		orgGroup.GET("/:id", orgHandler.GetOrg)
		orgGroup.PUT("/:id", auth.RequireAdminAccess(), orgHandler.UpdateOrg)
		orgGroup.DELETE("/:id", auth.RequireRole("owner"), orgHandler.DeleteOrg)
		orgGroup.GET("/:id/members", orgHandler.ListMembers)
		orgGroup.POST("/:id/members", auth.RequireAdminAccess(), orgHandler.AddMember)
		orgGroup.DELETE("/:id/members/:userId", auth.RequireAdminAccess(), orgHandler.RemoveMember)
	}

	// Admin-only routes (require owner or admin role)
	adminGroup := router.Group("/admin")
	adminGroup.Use(auth.JWTMiddleware(jwtSvc, blocklist))
	adminGroup.Use(auth.RequireRole("owner", "admin"))
	{
		// Admin endpoints can be added here
	}

	if litellmEnabled {
		// ── LiteLLM Mode ───────────────────────────────────────
		// Inference requests go directly to LiteLLM Proxy (not through this gateway).
		// This gateway only handles: auth, token CRUD, model CRUD, billing, webhooks.

		// LiteLLM webhook endpoint (receives usage callbacks)
		if litellmWebhook != nil {
			webhookGroup := router.Group("/webhooks")
			litellmWebhook.RegisterRoutes(webhookGroup)
			logger.Info("LiteLLM webhook endpoint registered at /webhooks/litellm")
		}

		logger.Info("LiteLLM mode: /v1 inference routes NOT registered (handled by LiteLLM Proxy)")
	} else {
		// ── Legacy Mode (deprecated) ───────────────────────────
		// Direct proxy to Dynamo — will be removed in a future version.
		logger.Warn("running in legacy mode without LiteLLM — set LITELLM_ENABLED=true to use LiteLLM Proxy")

		v1 := router.Group("/v1")
		v1.Use(proxy.APIKeyAuth(tokenValidator, logger))
		v1.Use(quota.RateLimitMiddleware(rateLimiter, metrics, logger))
		proxyHandler.RegisterRoutes(v1)
	}

	// ── Server ─────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("gateway starting", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down gateway...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("forced shutdown", zap.Error(err))
	}
	logger.Info("gateway stopped")
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
