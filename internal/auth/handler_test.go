package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Mock UserRepository
// ---------------------------------------------------------------------------

type mockUserRepo struct {
	users map[string]*User     // email -> user
	byID  map[uuid.UUID]*User  // id -> user
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users: make(map[string]*User),
		byID:  make(map[uuid.UUID]*User),
	}
}

func (m *mockUserRepo) CreateUser(_ context.Context, email, password, role string, orgID uuid.UUID) (*User, error) {
	if _, exists := m.users[email]; exists {
		// The real repo would return a DB constraint error, but the handler
		// checks GetUserByEmail first. Return a new user for the normal path.
		return nil, nil
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	u := &User{
		ID:           uuid.New(),
		OrgID:        orgID,
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	m.users[email] = u
	m.byID[u.ID] = u
	return u, nil
}

func (m *mockUserRepo) GetUserByEmail(_ context.Context, email string) (*User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockUserRepo) GetUserByID(_ context.Context, id uuid.UUID) (*User, error) {
	u, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const testJWTKey = "test-handler-secret-key-1234567890-long-enough"

func newTestHandlerReal() (*Handler, *mockUserRepo) {
	repo := newMockUserRepo()
	jwt := NewJWTService(testJWTKey, 3600, 7)
	logger := zap.NewNop()
	h := NewHandler(repo, jwt, logger, nil, nil, nil)
	return h, repo
}

func setupRouterReal(h *Handler) *gin.Engine {
	r := gin.New()
	auth := r.Group("/auth")
	h.RegisterRoutes(auth)
	// Protected routes need a middleware to set user_id/org_id.
	// We'll use the real JWTMiddleware for integration-like tests.
	protected := r.Group("/auth", JWTMiddleware(h.jwt, nil))
	h.RegisterProtectedRoutes(protected)
	return r
}

// registerUser is a helper that registers a user and returns the response body.
func registerUser(t *testing.T, router *gin.Engine, email, password string) authResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("registerUser: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp authResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("registerUser: unmarshal: %v", err)
	}
	return resp
}

// ---------------------------------------------------------------------------
// Tests using real Handler with mock UserRepository
// ---------------------------------------------------------------------------

func TestHandler_Register_Success(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	body, _ := json.Marshal(map[string]string{
		"email":    "new@example.com",
		"password": "Str0ng!Pass99",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp authResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.AccessToken == "" {
		t.Error("expected non-empty access_token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh_token")
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("expected Bearer, got %s", resp.TokenType)
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("expected 3600, got %d", resp.ExpiresIn)
	}
}

func TestHandler_Register_DuplicateEmail(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	registerUser(t, r, "dup@example.com", "Str0ng!Pass99")

	body, _ := json.Marshal(map[string]string{
		"email":    "dup@example.com",
		"password": "Str0ng!Pass99",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Register_InvalidEmail(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	body, _ := json.Marshal(map[string]string{
		"email":    "not-an-email",
		"password": "Str0ng!Pass99",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandler_Register_WeakPassword(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	// Passes gin's min=8 binding but fails complexity (no special char, etc.)
	body, _ := json.Marshal(map[string]string{
		"email":    "user@example.com",
		"password": "nouppercase1!",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for weak password, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Login_Success(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	registerUser(t, r, "login@example.com", "Str0ng!Pass99")

	body, _ := json.Marshal(map[string]string{
		"email":    "login@example.com",
		"password": "Str0ng!Pass99",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp authResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("expected tokens in login response")
	}
}

func TestHandler_Login_WrongPassword(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	registerUser(t, r, "wrong@example.com", "Str0ng!Pass99")

	body, _ := json.Marshal(map[string]string{
		"email":    "wrong@example.com",
		"password": "Wrong!Pass999",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Login_NonExistentUser(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	body, _ := json.Marshal(map[string]string{
		"email":    "nobody@example.com",
		"password": "Str0ng!Pass99",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandler_Login_InactiveUser(t *testing.T) {
	h, repo := newTestHandlerReal()
	r := setupRouterReal(h)

	registerUser(t, r, "inactive@example.com", "Str0ng!Pass99")

	// Deactivate the user directly in the mock
	repo.users["inactive@example.com"].IsActive = false

	body, _ := json.Marshal(map[string]string{
		"email":    "inactive@example.com",
		"password": "Str0ng!Pass99",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for inactive user, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Refresh_Success(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	regResp := registerUser(t, r, "refresh@example.com", "Str0ng!Pass99")

	body, _ := json.Marshal(map[string]string{
		"refresh_token": regResp.RefreshToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp authResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("expected new tokens from refresh")
	}
}

func TestHandler_Refresh_WithAccessToken_Fails(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	regResp := registerUser(t, r, "refresh2@example.com", "Str0ng!Pass99")

	body, _ := json.Marshal(map[string]string{
		"refresh_token": regResp.AccessToken, // wrong token type
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when using access token for refresh, got %d", w.Code)
	}
}

func TestHandler_Refresh_InvalidToken(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	body, _ := json.Marshal(map[string]string{
		"refresh_token": "this-is-invalid",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandler_GetMe(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	regResp := registerUser(t, r, "me@example.com", "Str0ng!Pass99")

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+regResp.AccessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var user User
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatalf("unmarshal user: %v", err)
	}
	if user.Email != "me@example.com" {
		t.Errorf("expected email me@example.com, got %s", user.Email)
	}
	if user.Role != "owner" {
		t.Errorf("expected role owner, got %s", user.Role)
	}
}

func TestHandler_GetMe_NoAuth(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandler_Logout(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	regResp := registerUser(t, r, "logout@example.com", "Str0ng!Pass99")

	// Logout endpoint is registered on the non-protected group
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+regResp.AccessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Logout_NoAuth(t *testing.T) {
	h, _ := newTestHandlerReal()
	r := setupRouterReal(h)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
