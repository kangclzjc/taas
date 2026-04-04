package token

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	tokenCacheTTL    = 5 * time.Minute
	tokenCachePrefix = "token:"
)

// CachedTokenInfo is what we store in Redis for fast validation.
type CachedTokenInfo struct {
	TokenID        string    `json:"tid"`
	UserID         string    `json:"uid"`
	OrgID          string    `json:"oid"`
	AllowedModels  []string  `json:"models"`  // empty = all models
	Scopes         []string  `json:"scopes"`
	RateLimitRPM   int       `json:"rpm"`
	RateLimitTPM   int       `json:"tpm"`
	BudgetLimitUSD float64   `json:"budget"`
	SLATier        string    `json:"sla"`
	ExpiresAt      time.Time `json:"exp"`
	IsActive       bool      `json:"active"`
}

// Validator validates API tokens, using Redis as a fast cache with DB fallback.
type Validator struct {
	redis  *redis.Client
	// db  is intentionally not included here; the validator accepts a DB lookup func
	// to keep this package decoupled from the DB layer.
	dbLookup func(ctx context.Context, hash string) (*CachedTokenInfo, error)
}

func NewValidator(rdb *redis.Client, dbLookup func(ctx context.Context, hash string) (*CachedTokenInfo, error)) *Validator {
	return &Validator{redis: rdb, dbLookup: dbLookup}
}

// HashToken returns the SHA-256 hex hash of a raw token value.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// Validate checks a raw API token and returns its info.
// Fast path: Redis cache hit.
// Slow path: DB lookup + cache populate.
func (v *Validator) Validate(ctx context.Context, rawToken string) (*CachedTokenInfo, error) {
	hash := HashToken(rawToken)
	cacheKey := tokenCachePrefix + hash

	// Fast path
	data, err := v.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var info CachedTokenInfo
		if jsonErr := json.Unmarshal(data, &info); jsonErr == nil {
			if !info.IsActive {
				return nil, ErrTokenRevoked
			}
			if !info.ExpiresAt.IsZero() && time.Now().After(info.ExpiresAt) {
				return nil, ErrTokenExpired
			}
			return &info, nil
		}
	}

	// Slow path: DB lookup
	info, err := v.dbLookup(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("token lookup: %w", err)
	}
	if info == nil {
		return nil, ErrTokenNotFound
	}

	// Populate cache (best-effort)
	if b, marshalErr := json.Marshal(info); marshalErr == nil {
		v.redis.Set(ctx, cacheKey, b, tokenCacheTTL) //nolint:errcheck
	}

	if !info.IsActive {
		return nil, ErrTokenRevoked
	}
	if !info.ExpiresAt.IsZero() && time.Now().After(info.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	return info, nil
}

// Invalidate removes a token from the Redis cache (call on revocation/rotation).
// tokenHash should be the SHA-256 hex hash already stored in the DB.
func (v *Validator) Invalidate(ctx context.Context, tokenHash string) error {
	return v.redis.Del(ctx, tokenCachePrefix+tokenHash).Err()
}

var (
	ErrTokenNotFound = fmt.Errorf("token not found")
	ErrTokenRevoked  = fmt.Errorf("token has been revoked")
	ErrTokenExpired  = fmt.Errorf("token has expired")
)
