package token

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Mock TokenRepository
// ---------------------------------------------------------------------------

type mockTokenRepo struct {
	tokens map[uuid.UUID]*Token
}

func newMockTokenRepo() *mockTokenRepo {
	return &mockTokenRepo{tokens: make(map[uuid.UUID]*Token)}
}

func (m *mockTokenRepo) Create(_ context.Context, t *Token) error {
	m.tokens[t.ID] = t
	return nil
}

func (m *mockTokenRepo) GetByHash(_ context.Context, hash string) (*Token, error) {
	for _, t := range m.tokens {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return nil, nil
}

func (m *mockTokenRepo) GetByID(_ context.Context, id uuid.UUID) (*Token, error) {
	t, ok := m.tokens[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockTokenRepo) ListByOrg(_ context.Context, orgID uuid.UUID, limit, offset int) ([]*Token, error) {
	var out []*Token
	for _, t := range m.tokens {
		if t.OrgID == orgID {
			out = append(out, t)
		}
	}
	// simple offset/limit
	if offset >= len(out) {
		return nil, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (m *mockTokenRepo) Revoke(_ context.Context, id uuid.UUID) error {
	t, ok := m.tokens[id]
	if !ok {
		return fmt.Errorf("token not found")
	}
	t.IsActive = false
	t.UpdatedAt = time.Now()
	return nil
}

func (m *mockTokenRepo) UpdateHash(_ context.Context, id uuid.UUID, newHash, newPrefix string) error {
	t, ok := m.tokens[id]
	if !ok {
		return fmt.Errorf("token not found")
	}
	t.TokenHash = newHash
	t.Prefix = newPrefix
	t.UpdatedAt = time.Now()
	return nil
}

func (m *mockTokenRepo) LookupForValidation(_ context.Context, hash string) (*CachedTokenInfo, error) {
	for _, t := range m.tokens {
		if t.TokenHash == hash {
			info := &CachedTokenInfo{
				TokenID:        t.ID.String(),
				UserID:         t.UserID.String(),
				OrgID:          t.OrgID.String(),
				AllowedModels:  t.AllowedModels,
				Scopes:         t.Scopes,
				RateLimitRPM:   t.RateLimitRPM,
				RateLimitTPM:   t.RateLimitTPM,
				BudgetLimitUSD: t.BudgetLimitUSD,
				SLATier:        t.SLATier,
				IsActive:       t.IsActive,
			}
			if t.ExpiresAt != nil {
				info.ExpiresAt = *t.ExpiresAt
			}
			return info, nil
		}
	}
	return nil, nil
}

func (m *mockTokenRepo) SetLiteLLMKeyToken(_ context.Context, id uuid.UUID, litellmKeyToken string) error {
	if t, ok := m.tokens[id]; ok {
		t.LiteLLMKeyToken = litellmKeyToken
		return nil
	}
	return fmt.Errorf("token not found")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setupTokenTest(t *testing.T) (*Handler, *mockTokenRepo, *gin.Engine, uuid.UUID, uuid.UUID) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(func() { mr.Close() })

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := newMockTokenRepo()
	validator := NewValidator(rdb, repo.LookupForValidation)
	logger := zap.NewNop()
	svc := NewService(repo, validator, logger)
	handler := NewHandler(svc, logger)

	userID := uuid.New()
	orgID := uuid.New()

	r := gin.New()
	// Middleware to inject user_id and org_id (simulating JWT auth)
	authed := r.Group("/tokens", func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Set("org_id", orgID.String())
		c.Next()
	})
	handler.RegisterRoutes(authed)

	return handler, repo, r, userID, orgID
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestTokenHandler_Create(t *testing.T) {
	_, _, r, _, _ := setupTokenTest(t)

	body, _ := json.Marshal(map[string]string{
		"name": "test-token",
	})
	req := httptest.NewRequest(http.MethodPost, "/tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Check the raw key is present
	key, ok := result["key"].(string)
	if !ok || key == "" {
		t.Error("expected non-empty 'key' in response")
	}
	if len(key) < 10 {
		t.Errorf("key looks too short: %s", key)
	}

	// Check the token object is present
	tokenObj, ok := result["token"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'token' object in response")
	}
	if tokenObj["name"] != "test-token" {
		t.Errorf("expected name 'test-token', got %v", tokenObj["name"])
	}
	if tokenObj["is_active"] != true {
		t.Errorf("expected is_active true, got %v", tokenObj["is_active"])
	}
}

func TestTokenHandler_List(t *testing.T) {
	_, _, r, _, _ := setupTokenTest(t)

	// Create two tokens
	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(map[string]string{
			"name": fmt.Sprintf("token-%d", i),
		})
		req := httptest.NewRequest(http.MethodPost, "/tokens", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create token %d: expected 201, got %d", i, w.Code)
		}
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/tokens", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	tokens, ok := resp["tokens"].([]interface{})
	if !ok {
		t.Fatal("expected 'tokens' array in response")
	}
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}
}

func TestTokenHandler_Revoke(t *testing.T) {
	_, repo, r, _, _ := setupTokenTest(t)

	// Create a token first
	body, _ := json.Marshal(map[string]string{"name": "to-revoke"})
	createReq := httptest.NewRequest(http.MethodPost, "/tokens", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	r.ServeHTTP(createW, createReq)

	var createResp map[string]interface{}
	json.Unmarshal(createW.Body.Bytes(), &createResp)
	tokenObj := createResp["token"].(map[string]interface{})
	tokenID := tokenObj["id"].(string)

	// Revoke
	req := httptest.NewRequest(http.MethodDelete, "/tokens/"+tokenID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify in repo
	tid, _ := uuid.Parse(tokenID)
	tok := repo.tokens[tid]
	if tok == nil {
		t.Fatal("token not found in repo")
	}
	if tok.IsActive {
		t.Error("expected token to be inactive after revocation")
	}
}

func TestTokenHandler_Rotate(t *testing.T) {
	_, repo, r, _, _ := setupTokenTest(t)

	// Create a token
	body, _ := json.Marshal(map[string]string{"name": "to-rotate"})
	createReq := httptest.NewRequest(http.MethodPost, "/tokens", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	r.ServeHTTP(createW, createReq)

	var createResp map[string]interface{}
	json.Unmarshal(createW.Body.Bytes(), &createResp)
	tokenObj := createResp["token"].(map[string]interface{})
	tokenID := tokenObj["id"].(string)

	// Record original hash
	tid, _ := uuid.Parse(tokenID)
	oldHash := repo.tokens[tid].TokenHash

	// Rotate
	req := httptest.NewRequest(http.MethodPost, "/tokens/"+tokenID+"/rotate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var rotateResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &rotateResp)
	newKey, ok := rotateResp["key"].(string)
	if !ok || newKey == "" {
		t.Error("expected non-empty 'key' in rotate response")
	}

	// Verify hash changed
	newHash := repo.tokens[tid].TokenHash
	if newHash == oldHash {
		t.Error("expected token hash to change after rotation")
	}
}

func TestTokenHandler_Revoke_NotFound(t *testing.T) {
	_, _, r, _, _ := setupTokenTest(t)

	fakeID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/tokens/"+fakeID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTokenHandler_Create_MissingName(t *testing.T) {
	_, _, r, _, _ := setupTokenTest(t)

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
