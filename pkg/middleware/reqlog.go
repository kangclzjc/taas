package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLogger is an enhanced request logger that logs request body (truncated)
// and response status for debugging purposes.
func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Read and restore body for logging
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		c.Next()

		latency := time.Since(start)

		fields := []zap.Field{
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Duration("latency", latency),
			zap.Int("body_size", len(bodyBytes)),
		}

		// Log body preview for non-GET requests (truncated to 200 chars)
		if c.Request.Method != "GET" && len(bodyBytes) > 0 {
			preview := string(bodyBytes)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			fields = append(fields, zap.String("body_preview", preview))
		}

		if c.Writer.Status() >= 400 {
			logger.Warn("request", fields...)
		} else {
			logger.Info("request", fields...)
		}
	}
}
