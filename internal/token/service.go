package token

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// base62 alphabet for token generation
	base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// tokenRandomLength is the number of random base62 characters in the token
	tokenRandomLength = 42
	// tokenPrefix is prepended to all TaaS API tokens
	tokenPrefix = "taas_"
)

// Service provides business logic for API token management.
type Service struct {
	repo      *Repository
	validator *Validator
	logger    *zap.Logger
}

func NewService(repo *Repository, validator *Validator, logger *zap.Logger) *Service {
	return &Service{repo: repo, validator: validator, logger: logger}
}

// CreateRequest holds parameters for creating a new token.
type CreateRequest struct {
	Name           string    `json:"name" binding:"required"`
	AllowedModels  []string  `json:"allowed_models,omitempty"`
	Scopes         []string  `json:"scopes,omitempty"`
	RateLimitRPM   int       `json:"rate_limit_rpm,omitempty"`
	RateLimitTPM   int       `json:"rate_limit_tpm,omitempty"`
	BudgetLimitUSD float64   `json:"budget_limit_usd,omitempty"`
	SLATier        string    `json:"sla_tier,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// CreateResult is returned on token creation — the raw token is shown only once.
type CreateResult struct {
	Token    *Token `json:"token"`
	RawToken string `json:"key"`
}

// Create generates a new API token, stores the hash, and returns the raw value once.
func (s *Service) Create(ctx context.Context, userID, orgID uuid.UUID, req CreateRequest) (*CreateResult, error) {
	rawToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generating token: %w", err)
	}

	hash := HashToken(rawToken)

	if req.RateLimitRPM == 0 {
		req.RateLimitRPM = 60
	}
	if req.RateLimitTPM == 0 {
		req.RateLimitTPM = 100000
	}
	if req.SLATier == "" {
		req.SLATier = "standard"
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"inference"}
	}

	t := &Token{
		ID:             uuid.New(),
		UserID:         userID,
		OrgID:          orgID,
		Name:           req.Name,
		TokenHash:      hash,
		Prefix:         rawToken[:len(tokenPrefix)+8],
		AllowedModels:  req.AllowedModels,
		Scopes:         req.Scopes,
		RateLimitRPM:   req.RateLimitRPM,
		RateLimitTPM:   req.RateLimitTPM,
		BudgetLimitUSD: req.BudgetLimitUSD,
		SLATier:        req.SLATier,
		IsActive:       true,
		ExpiresAt:      req.ExpiresAt,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("storing token: %w", err)
	}

	return &CreateResult{Token: t, RawToken: rawToken}, nil
}

// List returns tokens for an organization.
func (s *Service) List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*Token, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListByOrg(ctx, orgID, limit, offset)
}

// Revoke deactivates a token.
func (s *Service) Revoke(ctx context.Context, tokenID, orgID uuid.UUID) error {
	t, err := s.repo.GetByID(ctx, tokenID)
	if err != nil {
		return fmt.Errorf("fetching token: %w", err)
	}
	if t == nil {
		return fmt.Errorf("token not found")
	}
	if t.OrgID != orgID {
		return fmt.Errorf("token not found") // don't leak existence
	}

	if err := s.repo.Revoke(ctx, tokenID); err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}

	// Invalidate cache by hash
	_ = s.validator.Invalidate(ctx, t.TokenHash)
	return nil
}

// Rotate generates a new raw token value for an existing token record.
// Returns the new raw token (shown once).
func (s *Service) Rotate(ctx context.Context, tokenID, orgID uuid.UUID) (string, error) {
	t, err := s.repo.GetByID(ctx, tokenID)
	if err != nil {
		return "", fmt.Errorf("fetching token: %w", err)
	}
	if t == nil || t.OrgID != orgID {
		return "", fmt.Errorf("token not found")
	}
	if !t.IsActive {
		return "", fmt.Errorf("cannot rotate a revoked token")
	}

	// Invalidate old cache entry
	_ = s.validator.Invalidate(ctx, t.TokenHash)

	newRaw, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("generating new token: %w", err)
	}

	newHash := HashToken(newRaw)
	newPrefix := newRaw[:len(tokenPrefix)+8]

	if err := s.repo.UpdateHash(ctx, tokenID, newHash, newPrefix); err != nil {
		return "", fmt.Errorf("updating token hash: %w", err)
	}

	return newRaw, nil
}

// generateToken produces a cryptographically random token: taas_<42 base62 chars>
func generateToken() (string, error) {
	b := make([]byte, tokenRandomLength)
	max := big.NewInt(int64(len(base62Chars)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = base62Chars[n.Int64()]
	}
	return tokenPrefix + string(b), nil
}
