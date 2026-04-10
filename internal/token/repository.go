package token

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenRepository defines the interface for token persistence operations.
type TokenRepository interface {
	Create(ctx context.Context, t *Token) error
	GetByHash(ctx context.Context, hash string) (*Token, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Token, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*Token, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	UpdateHash(ctx context.Context, id uuid.UUID, newHash, newPrefix string) error
	LookupForValidation(ctx context.Context, hash string) (*CachedTokenInfo, error)
	SetLiteLLMKeyToken(ctx context.Context, id uuid.UUID, litellmKeyToken string) error
}

// Token represents an API token stored in the database.
// The raw token value is never stored — only the SHA-256 hash.
type Token struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	OrgID          uuid.UUID  `json:"org_id"`
	Name           string     `json:"name"`
	TokenHash      string     `json:"-"`
	Prefix         string     `json:"prefix"` // first 8 chars for identification
	AllowedModels  []string   `json:"allowed_models,omitempty"`
	Scopes         []string   `json:"scopes"`
	RateLimitRPM   int        `json:"rate_limit_rpm"`
	RateLimitTPM   int        `json:"rate_limit_tpm"`
	BudgetLimitUSD float64    `json:"budget_limit_usd"`
	SLATier        string     `json:"sla_tier"`
	IsActive        bool       `json:"is_active"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	LiteLLMKeyToken string     `json:"litellm_key_token,omitempty"` // LiteLLM virtual key hash (for sync)
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Repository handles token persistence in PostgreSQL.
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create inserts a new token record.
func (r *Repository) Create(ctx context.Context, t *Token) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO api_tokens (id, user_id, org_id, name, token_hash, prefix, allowed_models, scopes,
		 rate_limit_rpm, rate_limit_tpm, budget_limit_usd, sla_tier, is_active, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		t.ID, t.UserID, t.OrgID, t.Name, t.TokenHash, t.Prefix, t.AllowedModels, t.Scopes,
		t.RateLimitRPM, t.RateLimitTPM, t.BudgetLimitUSD, t.SLATier, t.IsActive, t.ExpiresAt, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// GetByHash retrieves a token by its SHA-256 hash.
func (r *Repository) GetByHash(ctx context.Context, hash string) (*Token, error) {
	t := &Token{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, org_id, name, token_hash, prefix, allowed_models, scopes,
		 rate_limit_rpm, rate_limit_tpm, budget_limit_usd, sla_tier, is_active, expires_at,
		 last_used_at, created_at, updated_at
		 FROM api_tokens WHERE token_hash = $1`, hash,
	).Scan(&t.ID, &t.UserID, &t.OrgID, &t.Name, &t.TokenHash, &t.Prefix, &t.AllowedModels, &t.Scopes,
		&t.RateLimitRPM, &t.RateLimitTPM, &t.BudgetLimitUSD, &t.SLATier, &t.IsActive, &t.ExpiresAt,
		&t.LastUsedAt, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// GetByID retrieves a token by its UUID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Token, error) {
	t := &Token{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, org_id, name, token_hash, prefix, allowed_models, scopes,
		 rate_limit_rpm, rate_limit_tpm, budget_limit_usd, sla_tier, is_active, expires_at,
		 last_used_at, created_at, updated_at
		 FROM api_tokens WHERE id = $1`, id,
	).Scan(&t.ID, &t.UserID, &t.OrgID, &t.Name, &t.TokenHash, &t.Prefix, &t.AllowedModels, &t.Scopes,
		&t.RateLimitRPM, &t.RateLimitTPM, &t.BudgetLimitUSD, &t.SLATier, &t.IsActive, &t.ExpiresAt,
		&t.LastUsedAt, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// ListByOrg returns all tokens for an organization.
func (r *Repository) ListByOrg(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*Token, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, org_id, name, prefix, allowed_models, scopes,
		 rate_limit_rpm, rate_limit_tpm, budget_limit_usd, sla_tier, is_active, expires_at,
		 last_used_at, created_at, updated_at
		 FROM api_tokens WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*Token
	for rows.Next() {
		t := &Token{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.OrgID, &t.Name, &t.Prefix, &t.AllowedModels, &t.Scopes,
			&t.RateLimitRPM, &t.RateLimitTPM, &t.BudgetLimitUSD, &t.SLATier, &t.IsActive, &t.ExpiresAt,
			&t.LastUsedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// Revoke marks a token as inactive.
func (r *Repository) Revoke(ctx context.Context, id uuid.UUID) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE api_tokens SET is_active = false, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("token not found")
	}
	return nil
}

// UpdateHash updates the token hash and prefix (used during rotation).
func (r *Repository) UpdateHash(ctx context.Context, id uuid.UUID, newHash, newPrefix string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE api_tokens SET token_hash = $1, prefix = $2, updated_at = NOW() WHERE id = $3`,
		newHash, newPrefix, id)
	return err
}

// SetLiteLLMKeyToken stores the LiteLLM virtual key token hash for a TaaS token.
func (r *Repository) SetLiteLLMKeyToken(ctx context.Context, id uuid.UUID, litellmKeyToken string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE api_tokens SET litellm_key_token = $1, updated_at = NOW() WHERE id = $2`,
		litellmKeyToken, id)
	return err
}

// LookupForValidation is the DB lookup function compatible with the Validator.
func (r *Repository) LookupForValidation(ctx context.Context, hash string) (*CachedTokenInfo, error) {
	t, err := r.GetByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}

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
