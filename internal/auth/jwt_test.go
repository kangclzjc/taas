package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIssueAccessToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-12345", 3600, 7)

	token, err := svc.IssueAccessToken("user-123", "org-456", "user@example.com", "owner", []string{"inference", "admin"})
	if err != nil {
		t.Fatalf("IssueAccessToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("expected UserID 'user-123', got '%s'", claims.UserID)
	}
	if claims.OrgID != "org-456" {
		t.Errorf("expected OrgID 'org-456', got '%s'", claims.OrgID)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("expected Email 'user@example.com', got '%s'", claims.Email)
	}
	if claims.Role != "owner" {
		t.Errorf("expected Role 'owner', got '%s'", claims.Role)
	}
	if claims.TokenType != "access" {
		t.Errorf("expected TokenType 'access', got '%s'", claims.TokenType)
	}
	if len(claims.Scopes) != 2 || claims.Scopes[0] != "inference" || claims.Scopes[1] != "admin" {
		t.Errorf("expected Scopes [inference, admin], got %v", claims.Scopes)
	}
	if claims.Issuer != "taas" {
		t.Errorf("expected Issuer 'taas', got '%s'", claims.Issuer)
	}
	if claims.Subject != "user-123" {
		t.Errorf("expected Subject 'user-123', got '%s'", claims.Subject)
	}
}

func TestIssueRefreshToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-12345", 3600, 7)

	token, err := svc.IssueRefreshToken("user-123", "org-456")
	if err != nil {
		t.Fatalf("IssueRefreshToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("expected UserID 'user-123', got '%s'", claims.UserID)
	}
	if claims.OrgID != "org-456" {
		t.Errorf("expected OrgID 'org-456', got '%s'", claims.OrgID)
	}
	if claims.TokenType != "refresh" {
		t.Errorf("expected TokenType 'refresh', got '%s'", claims.TokenType)
	}
	if claims.Email != "" {
		t.Errorf("expected empty Email for refresh token, got '%s'", claims.Email)
	}
	if claims.Role != "" {
		t.Errorf("expected empty Role for refresh token, got '%s'", claims.Role)
	}
}

func TestValidateToken_Expired(t *testing.T) {
	// Use 0 seconds expiry -> token expires immediately
	svc := NewJWTService("test-secret-key-12345", 0, 0)

	token, err := svc.IssueAccessToken("user-123", "org-456", "user@example.com", "owner", []string{"*"})
	if err != nil {
		t.Fatalf("IssueAccessToken failed: %v", err)
	}

	// Wait a moment for expiry
	time.Sleep(10 * time.Millisecond)

	_, err = svc.ValidateToken(token)
	if err == nil {
		t.Fatal("expected ValidateToken to fail with expired token")
	}
	if !strings.Contains(err.Error(), "expired") && !strings.Contains(err.Error(), "exp") {
		t.Errorf("expected expiry-related error, got: %v", err)
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	svc1 := NewJWTService("secret-key-one-12345", 3600, 7)
	svc2 := NewJWTService("secret-key-two-99999", 3600, 7)

	token, err := svc1.IssueAccessToken("user-123", "org-456", "user@example.com", "owner", []string{"*"})
	if err != nil {
		t.Fatalf("IssueAccessToken failed: %v", err)
	}

	_, err = svc2.ValidateToken(token)
	if err == nil {
		t.Fatal("expected ValidateToken to fail with wrong signing key")
	}
}

func TestValidateToken_WrongAlgorithm(t *testing.T) {
	svc := NewJWTService("test-secret-key-12345", 3600, 7)

	// Manually create a token using RSA (non-HMAC) header but we can't actually sign
	// with RSA since we only have an HMAC key. But we can test by creating an unsigned
	// token with a different algorithm claim.
	// The validator checks t.Method.(*jwt.SigningMethodHMAC) so any non-HMAC method should fail.

	// Create a token with "none" algorithm
	claims := &Claims{
		UserID:    "user-123",
		OrgID:     "org-456",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    "taas",
		},
	}

	// Use jwt.SigningMethodNone (alg: "none")
	unsafeToken := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, err := unsafeToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to create none-signed token: %v", err)
	}

	_, err = svc.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected ValidateToken to reject non-HMAC token")
	}
	if !strings.Contains(err.Error(), "signing method") {
		t.Errorf("expected 'signing method' in error, got: %v", err)
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	svc := NewJWTService("test-secret-key-12345", 3600, 7)

	_, err := svc.ValidateToken("this-is-not-a-jwt")
	if err == nil {
		t.Fatal("expected ValidateToken to fail with malformed token")
	}
}

func TestValidateToken_EmptyString(t *testing.T) {
	svc := NewJWTService("test-secret-key-12345", 3600, 7)

	_, err := svc.ValidateToken("")
	if err == nil {
		t.Fatal("expected ValidateToken to fail with empty token")
	}
}
