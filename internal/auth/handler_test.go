package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testHandler wraps auth handler logic with in-memory storage for testing
// without requiring a PostgreSQL database.
type testHandler struct {
	store  map[string]*User // email -> user
	byID   map[uuid.UUID]*User
	jwt    *JWTService
	logger *zap.Logger
}

func newTestHandler() *testHandler {
	return &testHandler{
		store:  make(map[string]*User),
		byID:   make(map[uuid.UUID]*User),
		jwt:    NewJWTService("test-handler-secret-key-1234", 3600, 7),
		logger: zap.NewNop(),
	}
}

func (h *testHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request: " + err.Error()})
		return
	}

	if _, exists := h.store[req.Email]; exists {
		c.JSON(http.StatusConflict, gin.H{"code": "CONFLICT", "message": "email already registered"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.MinCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "message": "hashing failed"})
		return
	}

	user := &User{
		ID:           uuid.New(),
		OrgID:        uuid.New(),
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         "owner",
		IsActive:     true,
	}
	h.store[req.Email] = user
	h.byID[user.ID] = user

	accessToken, _ := h.jwt.IssueAccessToken(user.ID.String(), user.OrgID.String(), user.Email, user.Role, []string{"*"})
	refreshToken, _ := h.jwt.IssueRefreshToken(user.ID.String(), user.OrgID.String())

	c.JSON(http.StatusCreated, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	})
}

func (h *testHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request: " + err.Error()})
		return
	}

	user, exists := h.store[req.Email]
	if !exists || !CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "invalid email or password"})
		return
	}
	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"code": "FORBIDDEN", "message": "account is deactivated"})
		return
	}

	accessToken, _ := h.jwt.IssueAccessToken(user.ID.String(), user.OrgID.String(), user.Email, user.Role, []string{"*"})
	refreshToken, _ := h.jwt.IssueRefreshToken(user.ID.String(), user.OrgID.String())

	c.JSON(http.StatusOK, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	})
}

func (h *testHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "BAD_REQUEST", "message": "invalid request: " + err.Error()})
		return
	}

	claims, err := h.jwt.ValidateToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "invalid refresh token"})
		return
	}
	if claims.TokenType != "refresh" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "token is not a refresh token"})
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "invalid user id"})
		return
	}

	user, exists := h.byID[userID]
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "user not found"})
		return
	}

	accessToken, _ := h.jwt.IssueAccessToken(user.ID.String(), user.OrgID.String(), user.Email, user.Role, []string{"*"})
	refreshToken, _ := h.jwt.IssueRefreshToken(user.ID.String(), user.OrgID.String())

	c.JSON(http.StatusOK, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	})
}

func (h *testHandler) setupRouter() *gin.Engine {
	r := gin.New()
	auth := r.Group("/auth")
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/refresh", h.Refresh)
	return r
}

func TestRegister_Success(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	body, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "securepassword123",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp authResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("expected non-empty access_token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh_token")
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("expected token_type 'Bearer', got '%s'", resp.TokenType)
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("expected expires_in 3600, got %d", resp.ExpiresIn)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	body, _ := json.Marshal(map[string]string{
		"email":    "duplicate@example.com",
		"password": "securepassword123",
	})

	// First registration
	req1 := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("first register failed: status %d", w1.Code)
	}

	// Second registration with same email
	body2, _ := json.Marshal(map[string]string{
		"email":    "duplicate@example.com",
		"password": "anotherpassword123",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("expected status 409 for duplicate email, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	body, _ := json.Marshal(map[string]string{
		"email":    "not-an-email",
		"password": "securepassword123",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid email, got %d", w.Code)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	body, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "short",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for short password, got %d", w.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	// Register first
	regBody, _ := json.Marshal(map[string]string{
		"email":    "login@example.com",
		"password": "correctpassword1",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	r.ServeHTTP(regW, regReq)

	if regW.Code != http.StatusCreated {
		t.Fatalf("registration failed: %d", regW.Code)
	}

	// Now login
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "login@example.com",
		"password": "correctpassword1",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", loginW.Code, loginW.Body.String())
	}

	var resp authResponse
	if err := json.Unmarshal(loginW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse login response: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access_token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh_token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	// Register
	regBody, _ := json.Marshal(map[string]string{
		"email":    "wrong@example.com",
		"password": "correctpassword1",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	r.ServeHTTP(regW, regReq)

	// Login with wrong password
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "wrong@example.com",
		"password": "wrongpassword111",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for wrong password, got %d: %s", loginW.Code, loginW.Body.String())
	}
}

func TestLogin_NonExistentUser(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	loginBody, _ := json.Marshal(map[string]string{
		"email":    "nobody@example.com",
		"password": "somepassword12",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", loginW.Code)
	}
}

func TestRefresh_Success(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	// Register
	regBody, _ := json.Marshal(map[string]string{
		"email":    "refresh@example.com",
		"password": "securepassword1",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	r.ServeHTTP(regW, regReq)

	var regResp authResponse
	json.Unmarshal(regW.Body.Bytes(), &regResp)

	// Use refresh token
	refreshBody, _ := json.Marshal(map[string]string{
		"refresh_token": regResp.RefreshToken,
	})
	refreshReq := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(refreshBody))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshW := httptest.NewRecorder()
	r.ServeHTTP(refreshW, refreshReq)

	if refreshW.Code != http.StatusOK {
		t.Fatalf("expected status 200 for refresh, got %d: %s", refreshW.Code, refreshW.Body.String())
	}

	var resp authResponse
	if err := json.Unmarshal(refreshW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse refresh response: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected new access_token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected new refresh_token")
	}
}

func TestRefresh_WithAccessToken_ShouldFail(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	// Register
	regBody, _ := json.Marshal(map[string]string{
		"email":    "refresh2@example.com",
		"password": "securepassword1",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	r.ServeHTTP(regW, regReq)

	var regResp authResponse
	json.Unmarshal(regW.Body.Bytes(), &regResp)

	// Try using access token as refresh token — should fail
	refreshBody, _ := json.Marshal(map[string]string{
		"refresh_token": regResp.AccessToken,
	})
	refreshReq := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(refreshBody))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshW := httptest.NewRecorder()
	r.ServeHTTP(refreshW, refreshReq)

	if refreshW.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 when using access token for refresh, got %d", refreshW.Code)
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	h := newTestHandler()
	r := h.setupRouter()

	refreshBody, _ := json.Marshal(map[string]string{
		"refresh_token": "this-is-not-a-valid-token",
	})
	refreshReq := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(refreshBody))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshW := httptest.NewRecorder()
	r.ServeHTTP(refreshW, refreshReq)

	if refreshW.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", refreshW.Code)
	}
}
