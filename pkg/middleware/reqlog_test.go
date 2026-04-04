package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLogger_GETRequest(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RequestLogger(logger))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if logs.Len() == 0 {
		t.Fatal("expected at least one log entry")
	}

	entry := logs.All()[0]
	if entry.Message != "request" {
		t.Errorf("expected message 'request', got '%s'", entry.Message)
	}

	// GET requests should not have body_preview
	for _, f := range entry.Context {
		if f.Key == "body_preview" {
			t.Error("GET requests should not have body_preview logged")
		}
	}
}

func TestRequestLogger_POSTRequest(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RequestLogger(logger))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	if logs.Len() == 0 {
		t.Fatal("expected at least one log entry")
	}

	entry := logs.All()[0]
	found := false
	for _, f := range entry.Context {
		if f.Key == "body_preview" {
			found = true
			if f.String != body {
				t.Errorf("expected body_preview '%s', got '%s'", body, f.String)
			}
		}
	}
	if !found {
		t.Error("POST requests should have body_preview logged")
	}
}

func TestRequestLogger_4xxStatus(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RequestLogger(logger))
	r.GET("/bad", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad"})
	})

	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if logs.Len() == 0 {
		t.Fatal("expected warn-level log entry for 4xx status")
	}
}

func TestRequestLogger_BodyTruncation(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	r := gin.New()
	r.Use(RequestLogger(logger))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Create a body larger than 200 chars
	longBody := bytes.Repeat([]byte("x"), 300)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(longBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if logs.Len() == 0 {
		t.Fatal("expected at least one log entry")
	}

	entry := logs.All()[0]
	for _, f := range entry.Context {
		if f.Key == "body_preview" {
			if len(f.String) > 210 { // 200 + "..."
				t.Errorf("body_preview should be truncated, got length %d", len(f.String))
			}
			if f.String[len(f.String)-3:] != "..." {
				t.Error("truncated body should end with '...'")
			}
		}
	}
}
