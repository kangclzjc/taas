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
	"go.uber.org/zap"

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

	router := gin.New()
	router.Use(
		middleware.Logger(logger),
		middleware.Recovery(logger),
		middleware.RequestID(),
		middleware.Cors(cfg.CORSAllowedOrigins),
	)

	// Health endpoints
	router.GET("/health", healthHandler)
	router.GET("/health/ready", readinessHandler)
	router.GET("/metrics", metricsHandler)

	// TODO: register route groups (auth, tokens, models, v1/*, usage, billing, admin)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("forced shutdown", zap.Error(err))
	}
	logger.Info("gateway stopped")
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func readinessHandler(c *gin.Context) {
	// TODO: check DB, Redis, downstream services
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func metricsHandler(c *gin.Context) {
	// TODO: delegate to prometheus handler
	c.Status(http.StatusOK)
}
