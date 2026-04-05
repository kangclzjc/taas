package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/taas-platform/taas/internal/audit"
	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// dummyHash is a pre-computed bcrypt hash used to prevent timing attacks
// when a login attempt is made with a non-existent email.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-attack-dummy"), bcrypt.DefaultCost)

// Handler holds auth HTTP handlers.
type Handler struct {
	repo        UserRepository
	jwt         *JWTService
	logger      *zap.Logger
	blocklist   *Blocklist
	rateLimiter *LoginRateLimiter
	audit       *audit.Logger
}

// NewHandler creates a new auth handler. blocklist, rateLimiter, and auditLogger
// can be nil to disable those features.
func NewHandler(repo UserRepository, jwt *JWTService, logger *zap.Logger, blocklist *Blocklist, rateLimiter *LoginRateLimiter, auditLogger *audit.Logger) *Handler {
	return &Handler{
		repo:        repo,
		jwt:         jwt,
		logger:      logger,
		blocklist:   blocklist,
		rateLimiter: rateLimiter,
		audit:       auditLogger,
	}
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// Register creates a new user account.
func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	// Validate password complexity
	if err := ValidatePasswordComplexity(req.Password); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest(err.Error()))
		return
	}

	existing, err := h.repo.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		h.logger.Error("checking existing user", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to check user"))
		return
	}
	if existing != nil {
		middleware.ErrorResponse(c, taasErrors.Conflict("email already registered"))
		return
	}

	orgID := uuid.New() // new org per registration
	user, err := h.repo.CreateUser(c.Request.Context(), req.Email, req.Password, "owner", orgID)
	if err != nil {
		h.logger.Error("creating user", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to create user"))
		return
	}

	accessToken, err := h.jwt.IssueAccessToken(user.ID.String(), user.OrgID.String(), user.Email, user.Role, []string{"*"})
	if err != nil {
		h.logger.Error("issuing access token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to issue token"))
		return
	}

	refreshToken, err := h.jwt.IssueRefreshToken(user.ID.String(), user.OrgID.String())
	if err != nil {
		h.logger.Error("issuing refresh token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to issue token"))
		return
	}

	c.JSON(http.StatusCreated, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.jwt.accessExpiry.Seconds()),
	})
}

// Login authenticates a user and returns JWT tokens.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	// Check rate limit before attempting auth
	if h.rateLimiter != nil {
		allowed, remaining, err := h.rateLimiter.Check(c.Request.Context(), req.Email)
		if err != nil {
			h.logger.Error("rate limiter check failed, failing closed", zap.Error(err))
			// Fail closed: reject when rate limiter unavailable (security-critical path) (P1)
			middleware.ErrorResponse(c, taasErrors.Internal("authentication service temporarily unavailable"))
			return
		} else if !allowed {
			h.logAudit(c, audit.Event{
				Action:  "login",
				Status:  "denied",
				Details: map[string]string{"email": req.Email, "reason": "rate_limited"},
			})
			middleware.ErrorResponse(c, taasErrors.New(taasErrors.CodeTooManyRequests, "too many login attempts, please try again later", http.StatusTooManyRequests))
			return
		} else {
			_ = remaining // available for rate limit headers if desired
		}
	}

	user, err := h.repo.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		h.logger.Error("looking up user", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to authenticate"))
		return
	}
	if user == nil {
		// Perform dummy bcrypt comparison to prevent timing-based email enumeration (P0)
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
		h.logAudit(c, audit.Event{
			Action:  "login",
			Status:  "failed",
			Details: map[string]string{"email": req.Email, "reason": "invalid_credentials"},
		})
		middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid email or password"))
		return
	}
	if !CheckPassword(user.PasswordHash, req.Password) {
		h.logAudit(c, audit.Event{
			Action:  "login",
			Status:  "failed",
			Details: map[string]string{"email": req.Email, "reason": "invalid_credentials"},
		})
		middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid email or password"))
		return
	}
	if !user.IsActive {
		middleware.ErrorResponse(c, taasErrors.Forbidden("account is deactivated"))
		return
	}

	// Reset rate limiter on successful login
	if h.rateLimiter != nil {
		if err := h.rateLimiter.Reset(c.Request.Context(), req.Email); err != nil {
			h.logger.Error("failed to reset rate limiter", zap.Error(err))
		}
	}

	accessToken, err := h.jwt.IssueAccessToken(user.ID.String(), user.OrgID.String(), user.Email, user.Role, []string{"*"})
	if err != nil {
		h.logger.Error("issuing access token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to issue token"))
		return
	}

	refreshToken, err := h.jwt.IssueRefreshToken(user.ID.String(), user.OrgID.String())
	if err != nil {
		h.logger.Error("issuing refresh token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to issue token"))
		return
	}

	h.logAudit(c, audit.Event{
		UserID: user.ID.String(),
		OrgID:  user.OrgID.String(),
		Action: "login",
		Status: "success",
	})

	c.JSON(http.StatusOK, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.jwt.accessExpiry.Seconds()),
	})
}

// Refresh exchanges a valid refresh token for a new token pair.
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid request: "+err.Error()))
		return
	}

	claims, err := h.jwt.ValidateToken(req.RefreshToken)
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid refresh token"))
		return
	}
	if claims.TokenType != "refresh" {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("token is not a refresh token"))
		return
	}

	uid, parseErr := uuid.Parse(claims.UserID)
	if parseErr != nil {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid user id in token"))
		return
	}
	user, err := h.repo.GetUserByID(c.Request.Context(), uid)
	if err != nil || user == nil {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("user not found"))
		return
	}
	if !user.IsActive {
		middleware.ErrorResponse(c, taasErrors.Forbidden("account is deactivated"))
		return
	}

	accessToken, err := h.jwt.IssueAccessToken(user.ID.String(), user.OrgID.String(), user.Email, user.Role, []string{"*"})
	if err != nil {
		h.logger.Error("issuing access token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to issue token"))
		return
	}

	refreshToken, err := h.jwt.IssueRefreshToken(user.ID.String(), user.OrgID.String())
	if err != nil {
		h.logger.Error("issuing refresh token", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to issue token"))
		return
	}

	c.JSON(http.StatusOK, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.jwt.accessExpiry.Seconds()),
	})
}

// Logout revokes the current token by adding its JTI to the blocklist.
func (h *Handler) Logout(c *gin.Context) {
	// Extract the token from Authorization header
	header := c.GetHeader("Authorization")
	if header == "" {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("missing authorization header"))
		return
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid authorization format"))
		return
	}

	claims, err := h.jwt.ValidateToken(parts[1])
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid or expired token"))
		return
	}

	// Only access tokens can be used for logout (P2)
	if claims.TokenType != "access" {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("only access tokens can be used for logout"))
		return
	}

	if h.blocklist != nil && claims.ID != "" {
		// Calculate remaining TTL
		ttl := time.Until(claims.ExpiresAt.Time)
		if err := h.blocklist.Add(c.Request.Context(), claims.ID, ttl); err != nil {
			h.logger.Error("failed to add token to blocklist", zap.Error(err))
			middleware.ErrorResponse(c, taasErrors.Internal("failed to revoke token"))
			return
		}
	}

	h.logAudit(c, audit.Event{
		UserID: claims.UserID,
		OrgID:  claims.OrgID,
		Action: "logout",
		Status: "success",
	})

	c.JSON(http.StatusOK, gin.H{"message": "token revoked successfully"})
}

// logAudit is a helper that enriches the event with request info and logs it.
func (h *Handler) logAudit(c *gin.Context, event audit.Event) {
	if h.audit == nil {
		return
	}
	if event.IP == "" {
		event.IP = c.ClientIP()
	}
	if event.UserAgent == "" {
		event.UserAgent = c.Request.UserAgent()
	}
	h.audit.Log(c.Request.Context(), event)
}

// GetMe returns the authenticated user's profile.
func (h *Handler) GetMe(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		middleware.ErrorResponse(c, taasErrors.BadRequest("invalid user id"))
		return
	}

	user, err := h.repo.GetUserByID(c.Request.Context(), uid)
	if err != nil || user == nil {
		middleware.ErrorResponse(c, taasErrors.NotFound("user"))
		return
	}

	c.JSON(http.StatusOK, user)
}

// RegisterRoutes sets up auth routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/register", h.Register)
	rg.POST("/login", h.Login)
	rg.POST("/refresh", h.Refresh)
	rg.POST("/logout", h.Logout)
}

// RegisterProtectedRoutes sets up auth routes that require JWT authentication.
func (h *Handler) RegisterProtectedRoutes(rg *gin.RouterGroup) {
	rg.GET("/me", h.GetMe)
}
