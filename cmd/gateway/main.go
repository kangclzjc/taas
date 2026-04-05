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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/audit"
	"github.com/taas-platform/taas/internal/auth"
	"github.com/taas-platform/taas/internal/billing"
	dynamoClient "github.com/taas-platform/taas/internal/dynamo"
	"github.com/taas-platform/taas/internal/model"
	"github.com/taas-platform/taas/internal/monitoring"
	"github.com/taas-platform/taas/internal/org"
	"github.com/taas-platform/taas/internal/proxy"
	"github.com/taas-platform/taas/internal/quota"
	"github.com/taas-platform/taas/internal/telemetry"
	"github.com/taas-platform/taas/internal/token"
	"github.com/taas-platform/taas/pkg/config"
	"github.com/taas-platform/taas/pkg/middleware"
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
	poolConfig.MaxConns = int32(cfg.DBMaxOpenConns)  // max open connections (P3: clarified naming)
	poolConfig.MinConns = int32(cfg.DBMaxIdleConns)  // pre-warmed idle connections

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
	if cfg.NATSUrl != "" {
		var natsErr error
		nc, natsErr = nats.Connect(cfg.NATSUrl)
		if natsErr != nil {
			logger.Warn("failed to connect to nats, billing disabled", zap.Error(natsErr))
			nc = nil
		} else {
			defer nc.Close()
			js, jsErr := nc.JetStream()
			if jsErr != nil {
				logger.Warn("failed to init jetstream, billing disabled", zap.Error(jsErr))
			} else {
				usagePublisher = billing.NewPublisher(js)
			}
		}
	}

	// ── Services ───────────────────────────────────────────────
	metrics := monitoring.NewMetrics("taas")

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
	tokenHandler := token.NewHandler(tokenSvc, logger)

	modelRepo := model.NewPGRepository(dbPool)
	sharingService := model.NewSharingService(dbPool)
	modelSvc := model.NewService(modelRepo, sharingService)
	modelHandler := model.NewHandler(modelSvc, sharingService, logger)

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
		tokenHandler.RegisterRoutes(jwtAuth.Group("/tokens"))
		modelHandler.RegisterRoutes(jwtAuth.Group("/models"))
		billingHandler.RegisterRoutes(jwtAuth.Group("/usage"))
		orgHandler.RegisterRoutes(jwtAuth.Group("/organizations"))
	}

	// Admin-only routes (require owner or admin role)
	adminGroup := router.Group("/admin")
	adminGroup.Use(auth.JWTMiddleware(jwtSvc, blocklist))
	adminGroup.Use(auth.RequireRole("owner", "admin"))
	{
		// Admin endpoints can be added here
	}

	// API Key authenticated routes (inference)
	v1 := router.Group("/v1")
	v1.Use(proxy.APIKeyAuth(tokenValidator, logger))
	v1.Use(quota.RateLimitMiddleware(rateLimiter, metrics, logger))
	proxyHandler.RegisterRoutes(v1)

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
