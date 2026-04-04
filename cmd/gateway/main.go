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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/auth"
	"github.com/taas-platform/taas/internal/billing"
	dynamoClient "github.com/taas-platform/taas/internal/dynamo"
	"github.com/taas-platform/taas/internal/model"
	"github.com/taas-platform/taas/internal/monitoring"
	"github.com/taas-platform/taas/internal/proxy"
	"github.com/taas-platform/taas/internal/quota"
	"github.com/taas-platform/taas/internal/token"
	"github.com/taas-platform/taas/pkg/config"
	"github.com/taas-platform/taas/pkg/middleware"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync() //nolint:errcheck

	cfg, err := config.Load("gateway")
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Database ────────────────────────────────────────────────
	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer dbPool.Close()

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
	if cfg.NATSUrl != "" {
		nc, natsErr := nats.Connect(cfg.NATSUrl)
		if natsErr != nil {
			logger.Fatal("failed to connect to nats", zap.Error(natsErr))
		}
		defer nc.Close()
		js, jsErr := nc.JetStream()
		if jsErr != nil {
			logger.Fatal("failed to init jetstream", zap.Error(jsErr))
		}
		usagePublisher = billing.NewPublisher(js)
	}

	// ── Services ───────────────────────────────────────────────
	metrics := monitoring.NewMetrics("taas")

	jwtSvc := auth.NewJWTService(cfg.JWTSigningKey, cfg.JWTExpirySeconds, cfg.RefreshTokenExpiryDays)

	authRepo := auth.NewRepository(dbPool)
	authHandler := auth.NewHandler(authRepo, jwtSvc, logger)

	tokenRepo := token.NewRepository(dbPool)
	tokenValidator := token.NewValidator(rdb, tokenRepo.LookupForValidation)
	tokenSvc := token.NewService(tokenRepo, tokenValidator, logger)
	tokenHandler := token.NewHandler(tokenSvc, logger)

	modelRepo := model.NewPGRepository(dbPool)
	modelSvc := model.NewService(modelRepo)
	sharingService := model.NewSharingService(dbPool)
	modelHandler := model.NewHandler(modelSvc, sharingService, logger)

	rateLimiter := quota.NewRateLimiter(rdb)

	costCalc := billing.NewCostCalculator()
	dc := dynamoClient.NewClient(cfg.DynamoFrontendURL)
	proxyHandler := proxy.NewHandler(dc, usagePublisher, costCalc, metrics, logger)

	billingHandler := billing.NewHandler(dbPool, logger)

	// ── Router ─────────────────────────────────────────────────
	router := gin.New()
	router.Use(
		middleware.Logger(logger),
		middleware.Recovery(logger),
		middleware.RequestID(),
		middleware.Cors(cfg.CORSAllowedOrigins),
	)

	// Public endpoints
	router.GET("/health", healthHandler)
	router.GET("/health/ready", func(c *gin.Context) {
		if pingErr := dbPool.Ping(c.Request.Context()); pingErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "error": "database"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Auth routes (public)
	authGroup := router.Group("/auth")
	authHandler.RegisterRoutes(authGroup)

	// JWT-authenticated routes
	jwtAuth := router.Group("")
	jwtAuth.Use(auth.JWTMiddleware(jwtSvc))
	{
		tokenHandler.RegisterRoutes(jwtAuth.Group("/tokens"))
		modelHandler.RegisterRoutes(jwtAuth.Group("/models"))
		billingHandler.RegisterRoutes(jwtAuth.Group("/usage"))
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
