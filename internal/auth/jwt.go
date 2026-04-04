package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims is the JWT payload for TaaS.
type Claims struct {
	UserID    string   `json:"uid"`
	OrgID     string   `json:"oid"`
	Email     string   `json:"email"`
	Role      string   `json:"role"`
	Scopes    []string `json:"scp"`
	TokenType string   `json:"typ"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// JWTService handles JWT issuance and validation.
type JWTService struct {
	signingKey     []byte
	accessExpiry   time.Duration
	refreshExpiry  time.Duration
	signingMethod  jwt.SigningMethod
}

func NewJWTService(signingKey string, accessExpirySecs, refreshExpiryDays int) *JWTService {
	return &JWTService{
		signingKey:    []byte(signingKey),
		accessExpiry:  time.Duration(accessExpirySecs) * time.Second,
		refreshExpiry: time.Duration(refreshExpiryDays) * 24 * time.Hour,
		signingMethod: jwt.SigningMethodHS256,
	}
}

// IssueAccessToken creates a signed JWT access token.
func (s *JWTService) IssueAccessToken(userID, orgID, email, role string, scopes []string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:    userID,
		OrgID:     orgID,
		Email:     email,
		Role:      role,
		Scopes:    scopes,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessExpiry)),
			Issuer:    "taas",
			Subject:   userID,
		},
	}
	return jwt.NewWithClaims(s.signingMethod, claims).SignedString(s.signingKey)
}

// IssueRefreshToken creates a signed JWT refresh token.
func (s *JWTService) IssueRefreshToken(userID, orgID string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:    userID,
		OrgID:     orgID,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshExpiry)),
			Issuer:    "taas",
			Subject:   userID,
		},
	}
	return jwt.NewWithClaims(s.signingMethod, claims).SignedString(s.signingKey)
}

// ValidateToken parses and validates a JWT, returning its claims.
func (s *JWTService) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.signingKey, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}
