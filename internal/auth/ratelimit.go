package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const loginAttemptsKeyPrefix = "login_attempts:"

// LoginRateLimiter limits login attempts per email using Redis.
type LoginRateLimiter struct {
	redis       *redis.Client
	maxAttempts int
	window      time.Duration
	lockout     time.Duration
}

// NewLoginRateLimiter creates a new login rate limiter.
func NewLoginRateLimiter(rdb *redis.Client, maxAttempts int, window, lockout time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{
		redis:       rdb,
		maxAttempts: maxAttempts,
		window:      window,
		lockout:     lockout,
	}
}

// Check returns (allowed bool, remainingAttempts int, err error).
// If the account is locked out, allowed is false and remainingAttempts is 0.
func (l *LoginRateLimiter) Check(ctx context.Context, email string) (bool, int, error) {
	key := loginAttemptsKeyPrefix + email

	// Check if currently locked out
	lockKey := key + ":locked"
	locked, err := l.redis.Exists(ctx, lockKey).Result()
	if err != nil {
		return false, 0, fmt.Errorf("checking lockout: %w", err)
	}
	if locked > 0 {
		return false, 0, nil
	}

	// Increment attempt counter
	pipe := l.redis.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, l.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, fmt.Errorf("incrementing login attempts: %w", err)
	}

	count := int(incrCmd.Val())
	if count > l.maxAttempts {
		// Lock out the account
		l.redis.Set(ctx, lockKey, "1", l.lockout) //nolint:errcheck
		return false, 0, nil
	}

	remaining := l.maxAttempts - count
	return true, remaining, nil
}

// Reset clears the counter after successful login.
func (l *LoginRateLimiter) Reset(ctx context.Context, email string) error {
	key := loginAttemptsKeyPrefix + email
	lockKey := key + ":locked"
	pipe := l.redis.Pipeline()
	pipe.Del(ctx, key)
	pipe.Del(ctx, lockKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("resetting login attempts: %w", err)
	}
	return nil
}
