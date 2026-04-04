package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	taasErrors "github.com/taas-platform/taas/pkg/errors"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRequestID_Generated(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	rid := w.Header().Get(RequestIDHeader)
	if rid == "" {
		t.Fatal("expected X-Request-ID header to be generated")
	}
	// UUID format check: should have 36 chars (8-4-4-4-12)
	if len(rid) != 36 {
		t.Errorf("expected UUID-format request ID (36 chars), got %d chars: %s", len(rid), rid)
	}
}

func TestRequestID_Preserved(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	customID := "my-custom-request-id-123"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, customID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	rid := w.Header().Get(RequestIDHeader)
	if rid != customID {
		t.Errorf("expected preserved request ID '%s', got '%s'", customID, rid)
	}
}

func TestRequestID_SetInContext(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())

	var ctxID string
	r.GET("/test", func(c *gin.Context) {
		val, exists := c.Get("request_id")
		if exists {
			ctxID = val.(string)
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if ctxID == "" {
		t.Fatal("expected request_id to be set in gin context")
	}
}

func TestErrorResponse_APIError(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		ErrorResponse(c, taasErrors.BadRequest("invalid input"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["code"] != string(taasErrors.CodeBadRequest) {
		t.Errorf("expected code '%s', got '%v'", taasErrors.CodeBadRequest, body["code"])
	}
	if body["message"] != "invalid input" {
		t.Errorf("expected message 'invalid input', got '%v'", body["message"])
	}
	// Should include request_id from middleware
	if body["request_id"] == nil || body["request_id"] == "" {
		t.Error("expected request_id to be set in error response")
	}
}

func TestErrorResponse_GenericError(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		ErrorResponse(c, fmt.Errorf("something went wrong"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["code"] != string(taasErrors.CodeInternal) {
		t.Errorf("expected code '%s', got '%v'", taasErrors.CodeInternal, body["code"])
	}
	if body["message"] != "internal server error" {
		t.Errorf("expected generic message, got '%v'", body["message"])
	}
}

func TestErrorResponse_NotFound(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		ErrorResponse(c, taasErrors.NotFound("model"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}
