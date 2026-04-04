package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
)

const RequestIDHeader = "X-Request-ID"

// RequestID injects a unique request ID into each request context and response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("request_id", id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// Logger logs each request with structured fields.
func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		fields := []zap.Field{
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Duration("latency", time.Since(start)),
			zap.String("user_agent", c.Request.UserAgent()),
		}
		if rid, ok := c.Get("request_id"); ok {
			fields = append(fields, zap.String("request_id", rid.(string)))
		}
		if uid, ok := c.Get("user_id"); ok {
			fields = append(fields, zap.String("user_id", uid.(string)))
		}

		if c.Writer.Status() >= 500 {
			log.Error("request", fields...)
		} else {
			log.Info("request", fields...)
		}
	}
}

// Recovery handles panics and returns a 500 error.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					zap.Any("error", r),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    taasErrors.CodeInternal,
					"message": "internal server error",
				})
			}
		}()
		c.Next()
	}
}

// Cors sets CORS headers.
func Cors(allowedOrigins []string) gin.HandlerFunc {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	allowAll := false
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
			break
		}
		originSet[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowAll {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if _, ok := originSet[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-API-Key")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// ErrorResponse writes a structured API error response.
func ErrorResponse(c *gin.Context, err error) {
	if ae, ok := taasErrors.As(err); ok {
		if rid, exists := c.Get("request_id"); exists {
			ae.WithRequestID(rid.(string))
		}
		c.AbortWithStatusJSON(ae.HTTPStatus, ae)
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"code":    taasErrors.CodeInternal,
		"message": "internal server error",
	})
}
