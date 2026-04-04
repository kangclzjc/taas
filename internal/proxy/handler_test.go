package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/billing"
	"github.com/taas-platform/taas/internal/dynamo"
	"github.com/taas-platform/taas/internal/monitoring"
	"github.com/taas-platform/taas/internal/token"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestHandler creates a proxy Handler wired to a mock Dynamo httptest server.
func newTestHandler(dynamoURL string) *Handler {
	dc := dynamo.NewClient(dynamoURL)
	costCalc := billing.NewCostCalculator(billing.PricingConfig{
		Prices: map[string][2]float64{
			"llama-3-8b":  {0.10, 0.20},
			"llama-3-70b": {0.50, 1.00},
		},
		DefaultPrice: [2]float64{0.50, 1.00},
	})
	logger := zap.NewNop()
	// metrics and publisher are nil — handler checks for nil before using them
	return NewHandler(dc, nil, costCalc, nil, logger)
}

// newTestHandlerWithMetrics creates a proxy Handler with metrics enabled.
func newTestHandlerWithMetrics(dynamoURL string) *Handler {
	dc := dynamo.NewClient(dynamoURL)
	costCalc := billing.NewCostCalculator(billing.PricingConfig{
		Prices: map[string][2]float64{
			"llama-3-8b":  {0.10, 0.20},
			"llama-3-70b": {0.50, 1.00},
		},
		DefaultPrice: [2]float64{0.50, 1.00},
	})
	logger := zap.NewNop()
	metrics := monitoring.NewMetrics("test_proxy")
	return NewHandler(dc, nil, costCalc, metrics, logger)
}

func setupRouter(h *Handler) *gin.Engine {
	r := gin.New()
	v1 := r.Group("/v1")
	h.RegisterRoutes(v1)
	return r
}

func setTokenInfo(req *http.Request) *http.Request {
	// We'll use gin's Set method via a middleware in tests instead
	return req
}

// tokenInfoMiddleware injects a fake token_info into gin context for testing.
func tokenInfoMiddleware(info *token.CachedTokenInfo) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("token_info", info)
		c.Set("request_id", "test-req-123")
		c.Next()
	}
}

func setupRouterWithAuth(h *Handler, info *token.CachedTokenInfo) *gin.Engine {
	r := gin.New()
	v1 := r.Group("/v1")
	if info != nil {
		v1.Use(tokenInfoMiddleware(info))
	}
	h.RegisterRoutes(v1)
	return r
}

func defaultTokenInfo() *token.CachedTokenInfo {
	return &token.CachedTokenInfo{
		TokenID:  "tok-test-123",
		UserID:   "user-1",
		OrgID:    "org-1",
		SLATier:  "standard",
		IsActive: true,
	}
}

func TestChatCompletions_Success(t *testing.T) {
	// Mock Dynamo backend returning a successful chat completion
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"id": "chatcmpl-abc",
			"object": "chat.completion",
			"choices": [{"message": {"role": "assistant", "content": "Hello, world!"}}],
			"usage": {"prompt_tokens": 12, "completion_tokens": 4, "total_tokens": 16}
		}`)
	}))
	defer dynamo.Close()

	h := newTestHandler(dynamo.URL)
	r := setupRouterWithAuth(h, defaultTokenInfo())

	body := `{"model":"llama-3-8b","messages":[{"role":"user","content":"Say hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["id"] != "chatcmpl-abc" {
		t.Errorf("expected id 'chatcmpl-abc', got '%v'", resp["id"])
	}
}

func TestChatCompletions_Streaming(t *testing.T) {
	// Mock Dynamo returning SSE streaming response
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		chunks := []string{
			`data: {"id":"chatcmpl-s1","choices":[{"delta":{"content":"Hi"}}]}`,
			`data: {"id":"chatcmpl-s1","choices":[{"delta":{"content":"!"}}]}`,
			`data: {"id":"chatcmpl-s1","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
		}
	}))
	defer dynamo.Close()

	h := newTestHandler(dynamo.URL)
	r := setupRouterWithAuth(h, defaultTokenInfo())

	body := `{"model":"llama-3-8b","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify SSE content type
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	// Verify SSE data was forwarded
	respBody := w.Body.String()
	if !strings.Contains(respBody, "chatcmpl-s1") {
		t.Error("expected streaming data to contain chatcmpl-s1")
	}
}

func TestChatCompletions_MissingAuth(t *testing.T) {
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not reach dynamo without auth")
	}))
	defer dynamo.Close()

	h := newTestHandler(dynamo.URL)
	// Don't inject token_info
	r := setupRouterWithAuth(h, nil)

	body := `{"model":"llama-3-8b","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	var errResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["code"] != "UNAUTHORIZED" {
		t.Errorf("expected error code UNAUTHORIZED, got %v", errResp["code"])
	}
}

func TestChatCompletions_BodyTooLarge(t *testing.T) {
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not reach dynamo with oversized body")
	}))
	defer dynamo.Close()

	h := newTestHandler(dynamo.URL)
	r := setupRouterWithAuth(h, defaultTokenInfo())

	// Create a body larger than 10MB
	largeBody := bytes.Repeat([]byte("x"), 11<<20) // 11MB
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatCompletions_DynamoError(t *testing.T) {
	// Mock Dynamo returning 500
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"internal error"}`)
	}))
	defer dynamo.Close()

	h := newTestHandler(dynamo.URL)
	r := setupRouterWithAuth(h, defaultTokenInfo())

	body := `{"model":"llama-3-8b","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}

	var errResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["code"] != "DYNAMO_ERROR" {
		t.Errorf("expected error code DYNAMO_ERROR, got %v", errResp["code"])
	}
}

func TestEmbeddings_Success(t *testing.T) {
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("expected path /v1/embeddings, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"object": "list",
			"data": [{"object": "embedding", "embedding": [0.1, 0.2, 0.3]}],
			"usage": {"prompt_tokens": 4, "completion_tokens": 0, "total_tokens": 4}
		}`)
	}))
	defer dynamo.Close()

	h := newTestHandler(dynamo.URL)
	r := setupRouterWithAuth(h, defaultTokenInfo())

	body := `{"model":"mistral-7b","input":"Hello world"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["object"] != "list" {
		t.Errorf("expected object 'list', got %v", resp["object"])
	}
}

func TestCompletions_Success(t *testing.T) {
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" {
			t.Errorf("expected path /v1/completions, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"id": "cmpl-xyz",
			"choices": [{"text": "world"}],
			"usage": {"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4}
		}`)
	}))
	defer dynamo.Close()

	h := newTestHandler(dynamo.URL)
	r := setupRouterWithAuth(h, defaultTokenInfo())

	body := `{"model":"llama-3-8b","prompt":"Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatCompletions_WithMetrics(t *testing.T) {
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"id":"chatcmpl-m1",
			"choices":[{"message":{"content":"ok"}}],
			"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}
		}`)
	}))
	defer dynamo.Close()

	h := newTestHandlerWithMetrics(dynamo.URL)
	r := setupRouterWithAuth(h, defaultTokenInfo())

	body := `{"model":"llama-3-8b","messages":[{"role":"user","content":"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// If we got here without panic, metrics recording worked
}

func TestGetTokenInfo_NilContext(t *testing.T) {
	// Test the getTokenInfo helper when no token_info is set
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := getTokenInfo(c)
	if info != nil {
		t.Error("expected nil when no token_info set")
	}
}

func TestGetTokenInfo_WrongType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_info", "not-a-CachedTokenInfo")

	info := getTokenInfo(c)
	if info != nil {
		t.Error("expected nil when token_info is wrong type")
	}
}

func TestGetTokenInfo_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	expected := &token.CachedTokenInfo{TokenID: "tok-1", OrgID: "org-1"}
	c.Set("token_info", expected)

	info := getTokenInfo(c)
	if info == nil {
		t.Fatal("expected non-nil token info")
	}
	if info.TokenID != "tok-1" {
		t.Errorf("expected TokenID 'tok-1', got '%s'", info.TokenID)
	}
}

func TestRegisterRoutes(t *testing.T) {
	dynamo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer dynamo.Close()

	h := newTestHandler(dynamo.URL)
	r := gin.New()
	v1 := r.Group("/v1")
	h.RegisterRoutes(v1)

	// Verify the routes exist by checking non-405 responses
	routes := r.Routes()
	expectedPaths := map[string]bool{
		"/v1/chat/completions": false,
		"/v1/completions":      false,
		"/v1/embeddings":       false,
	}
	for _, route := range routes {
		if _, ok := expectedPaths[route.Path]; ok {
			expectedPaths[route.Path] = true
		}
	}
	for path, found := range expectedPaths {
		if !found {
			t.Errorf("route %s not registered", path)
		}
	}
}
