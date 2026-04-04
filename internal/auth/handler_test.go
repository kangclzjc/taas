package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupTestHandler creates a handler with in-memory storage for testing.
// Since Repository uses *pgxpool.Pool (concrete DB), we test at the HTTP handler
// level using a real handler wired to a real (test) DB or by relying on the
// handler's end-to-end behavior. Here we create the handler with a nil db
// and override the repo methods we need. But since the repo methods call db
// directly and aren't interfaces, we'll test the handler integration differently:
// We create a full test setup that exercises the JWT layer and checks HTTP codes.
//
// For handler tests we use a helper approach: set up gin test router, and
// use the jwt layer + test the handler's response behavior.

func setupRouter() (*gin.Engine, *JWTService) {
	jwtSvc := NewJWTService("handler-test-secret-key-1234", 3600, 7)
	logger := zap.NewNop()

	// We can't create a real Repository without a DB, so we'll test the handler
	// through full integration at the HTTP level using a mock Repository.
	// Since Repository is a struct with a db pool, we'll create a thin wrapper test.

	r := gin.New()
	return r, jwtSvc
	// Actual handler tests need the mock approach below
}

// mockDB implements in-memory user storage for testing.
// We'll use it by creating mock handler functions directly.

type inMemoryUsers struct {
	users map[string]*User // email -> user
}

func newInMemoryUsers() *inMemoryUsers {
	return &inMemoryUsers{users: make(map[string]*User)}
}

// testHandler wraps auth handler logic with in-memory storage.
type testHandler struct {
	store  *inMemoryUsers
	jwt    *JWTService
	logger *zap.Logger
}

func newTestHandler() *testHandler {
	return &testHandler{
		store:  newInMemoryUsers(),
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

	if _, exists := h.store.users[req.Email]; exists {
		c.JSON(http.StatusConflict, gin.H{"code": "CONFLICT", "message": "email already registered"})
		return
	}

	hash, _ := hashPassword(req.Password)
	user := &User{
		ID:           mustNewUUID(),
		OrgID:        mustNewUUID(),
		Email:        req.Email,
		PasswordHash: hash,
		Role:         "owner",
		IsActive:     true,
	}
	h.store.users[req.Email] = user

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

	user, exists := h.store.users[req.Email]
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

	// Find user by ID
	var user *User
	for _, u := range h.store.users {
		if u.ID.String() == claims.UserID {
			user = u
			break
		}
	}
	if user == nil {
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

func (h *testHandler) setupRoutes(r *gin.Engine) {
	auth := r.Group("/auth")
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/refresh", h.Refresh)
}

// Helper functions

func hashPassword(password string) (string, error) {
	// Use bcrypt via the existing repository package function
	// We import bcrypt directly here
	import_bcrypt_hash, err := bcryptHash(password)
	if err != nil {
		return "", err
	}
	return import_bcrypt_hash, nil
}

// We can't import bcrypt without a separate helper, so inline it.
// Actually let's use golang.org/x/crypto/bcrypt directly.

func mustNewUUID() uuid.UUID {
	return uuid.New()
}
