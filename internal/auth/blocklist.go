package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const blocklistKeyPrefix = "jwt_blocklist:"

// Blocklist stores revoked JWT IDs (JTI) in Redis with TTL matching token expiry.
type Blocklist struct {
	redis *redis.Client
}

// NewBlocklist creates a new token blocklist backed by Redis.
func NewBlocklist(rdb *redis.Client) *Blocklist {
	return &Blocklist{redis: rdb}
}

// Add puts a JTI into the blocklist. TTL should match remaining token lifetime.
func (b *Blocklist) Add(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		// Token already expired, no need to blocklist.
		return nil
	}
	key := blocklistKeyPrefix + jti
	err := b.redis.Set(ctx, key, "1", ttl).Err()
	if err != nil {
		return fmt.Errorf("adding jti to blocklist: %w", err)
	}
	return nil
}

// IsBlocked checks if a JTI has been revoked.
func (b *Blocklist) IsBlocked(ctx context.Context, jti string) (bool, error) {
	key := blocklistKeyPrefix + jti
	val, err := b.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking blocklist: %w", err)
	}
	return val != "", nil
}
