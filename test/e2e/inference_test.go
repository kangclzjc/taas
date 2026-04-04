//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

var (
	gatewayURL = getEnv("TAAS_GATEWAY_URL", "http://localhost:8080")
	testAPIKey = getEnv("TAAS_TEST_API_KEY", "")
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		path string
	}{
		{"/health"},
		{"/health/ready"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := http.Get(gatewayURL + tt.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tt.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestAuthFlow(t *testing.T) {
	// Register
	regBody := map[string]string{
		"email":    fmt.Sprintf("e2e-%d@test.taas.io", time.Now().Unix()),
		"password": "Test1234!@#",
		"name":     "E2E Test User",
		"org_name": "E2E Test Org",
	}
	token := registerAndLogin(t, regBody)

	// Verify token works
	req, _ := http.NewRequest(http.MethodGet, gatewayURL+"/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestInferenceE2E(t *testing.T) {
	if testAPIKey == "" {
		t.Skip("TAAS_TEST_API_KEY not set, skipping inference test")
	}

	body := map[string]any{
		"model": "llama-3-8b",
		"messages": []map[string]string{
			{"role": "user", "content": "Say hello in one word."},
		},
		"max_tokens": 10,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, gatewayURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", testAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := result["choices"]; !ok {
		t.Errorf("expected 'choices' in response, got: %v", result)
	}
}

func registerAndLogin(t *testing.T, regBody map[string]string) string {
	t.Helper()

	b, _ := json.Marshal(regBody)
	resp, err := http.Post(gatewayURL+"/auth/register", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", resp.StatusCode)
	}

	loginBody := map[string]string{
		"email":    regBody["email"],
		"password": regBody["password"],
	}
	lb, _ := json.Marshal(loginBody)
	loginResp, err := http.Post(gatewayURL+"/auth/login", "application/json", bytes.NewReader(lb))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer loginResp.Body.Close()

	var loginResult struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(loginResp.Body).Decode(&loginResult) //nolint:errcheck
	return loginResult.AccessToken
}
