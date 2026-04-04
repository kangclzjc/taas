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

	cfg, err := config.Load("auth")
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	router := gin.New()
	router.Use(
		middleware.Logger(logger),
		middleware.Recovery(logger),
		middleware.RequestID(),
	)

	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	router.GET("/health/ready", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })

	// TODO: register auth route handlers

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:        addr,
		Handler:     router,
		ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		logger.Info("auth service starting", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(ctx) //nolint:errcheck
	logger.Info("auth service stopped")
}
