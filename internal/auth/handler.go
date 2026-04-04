package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	taasErrors "github.com/taas-platform/taas/pkg/errors"
	"github.com/taas-platform/taas/pkg/middleware"
)

// Handler holds auth HTTP handlers.
type Handler struct {
	repo   *Repository
	jwt    *JWTService
	logger *zap.Logger
}

func NewHandler(repo *Repository, jwt *JWTService, logger *zap.Logger) *Handler {
	return &Handler{repo: repo, jwt: jwt, logger: logger}
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

	user, err := h.repo.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		h.logger.Error("looking up user", zap.Error(err))
		middleware.ErrorResponse(c, taasErrors.Internal("failed to authenticate"))
		return
	}
	if user == nil || !CheckPassword(user.PasswordHash, req.Password) {
		middleware.ErrorResponse(c, taasErrors.Unauthorized("invalid email or password"))
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

	user, err := h.repo.GetUserByID(c.Request.Context(), uuid.MustParse(claims.UserID))
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

// Logout is a no-op for stateless JWT (client discards token).
// In production, you'd add the token JTI to a blocklist.
func (h *Handler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// RegisterRoutes sets up auth routes on the given router group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/register", h.Register)
	rg.POST("/login", h.Login)
	rg.POST("/refresh", h.Refresh)
	rg.POST("/logout", h.Logout)
}
