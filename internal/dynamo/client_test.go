package dynamo

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestForward_NonStreaming(t *testing.T) {
	// Mock Dynamo server returning a non-streaming JSON response with usage
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("X-Tenant-ID") != "tenant-1" {
			t.Errorf("expected X-Tenant-ID 'tenant-1', got '%s'", r.Header.Get("X-Tenant-ID"))
		}
		if r.Header.Get("X-Org-ID") != "org-1" {
			t.Errorf("expected X-Org-ID 'org-1', got '%s'", r.Header.Get("X-Org-ID"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"choices": [{"message": {"role": "assistant", "content": "Hello!"}}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	meta := TenantMetadata{
		TenantID:  "tenant-1",
		OrgID:     "org-1",
		TokenID:   "tok-1",
		SLATier:   "standard",
		RequestID: "req-123",
	}

	var buf bytes.Buffer
	result, err := client.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"llama-3-8b","messages":[{"role":"user","content":"hi"}]}`), meta, &buf)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}

	if result.HTTPStatus != http.StatusOK {
		t.Errorf("expected status 200, got %d", result.HTTPStatus)
	}
	if result.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", result.PromptTokens)
	}
	if result.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", result.CompletionTokens)
	}
	if result.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", result.TotalTokens)
	}
	if result.Streamed {
		t.Error("expected Streamed=false")
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty buffer")
	}
	if !strings.Contains(buf.String(), "chatcmpl-123") {
		t.Error("expected buffer to contain response body")
	}
}

func TestForward_Streaming(t *testing.T) {
	// Mock Dynamo server returning SSE chunks
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"Hel"}}]}`,
			`data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"lo!"}}]}`,
			`data: {"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	meta := TenantMetadata{
		TenantID:  "tenant-1",
		OrgID:     "org-1",
		TokenID:   "tok-1",
		SLATier:   "premium",
		RequestID: "req-456",
	}

	var buf bytes.Buffer
	result, err := client.Forward(context.Background(), "/v1/chat/completions", []byte(`{"model":"llama-3-8b","messages":[{"role":"user","content":"hi"}],"stream":true}`), meta, &buf)
	if err != nil {
		t.Fatalf("Forward streaming: %v", err)
	}

	if !result.Streamed {
		t.Error("expected Streamed=true")
	}
	if result.PromptTokens != 8 {
		t.Errorf("expected 8 prompt tokens, got %d", result.PromptTokens)
	}
	if result.CompletionTokens != 3 {
		t.Errorf("expected 3 completion tokens, got %d", result.CompletionTokens)
	}
	if result.TotalTokens != 11 {
		t.Errorf("expected 11 total tokens, got %d", result.TotalTokens)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty buffer with streamed data")
	}
}

func TestForward_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"internal server error"}`)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	meta := TenantMetadata{TenantID: "t1", OrgID: "o1"}

	var buf bytes.Buffer
	_, err := client.Forward(context.Background(), "/v1/chat/completions", []byte(`{}`), meta, &buf)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "dynamo error 500") {
		t.Errorf("expected error containing 'dynamo error 500', got: %s", err.Error())
	}
}

func TestForward_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	// Override with a short timeout
	client.httpClient.Timeout = 100 * time.Millisecond

	meta := TenantMetadata{TenantID: "t1", OrgID: "o1"}

	var buf bytes.Buffer
	_, err := client.Forward(context.Background(), "/v1/chat/completions", []byte(`{}`), meta, &buf)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestForward_NonStreaming_NoUsage(t *testing.T) {
	// Server returns valid JSON but without usage field
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"chatcmpl-999","choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	meta := TenantMetadata{TenantID: "t1", OrgID: "o1"}

	var buf bytes.Buffer
	result, err := client.Forward(context.Background(), "/v1/embeddings", []byte(`{}`), meta, &buf)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if result.PromptTokens != 0 {
		t.Errorf("expected 0 prompt tokens, got %d", result.PromptTokens)
	}
	if result.CompletionTokens != 0 {
		t.Errorf("expected 0 completion tokens, got %d", result.CompletionTokens)
	}
}

func TestForward_4xxError(t *testing.T) {
	// 4xx errors should not be treated as dynamo errors — they get forwarded
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"invalid model","type":"invalid_request_error"}}`)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	meta := TenantMetadata{TenantID: "t1", OrgID: "o1"}

	var buf bytes.Buffer
	result, err := client.Forward(context.Background(), "/v1/chat/completions", []byte(`{}`), meta, &buf)
	if err != nil {
		t.Fatalf("Forward: unexpected error for 4xx: %v", err)
	}
	if result.HTTPStatus != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", result.HTTPStatus)
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8080")
	if client.baseURL != "http://localhost:8080" {
		t.Errorf("expected baseURL 'http://localhost:8080', got '%s'", client.baseURL)
	}
	if client.httpClient == nil {
		t.Error("expected non-nil httpClient")
	}
}
