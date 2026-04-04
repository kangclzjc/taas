package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter implements a sliding window rate limiter using Redis sorted sets.
type RateLimiter struct {
	redis *redis.Client
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{redis: rdb}
}

// CheckRPM checks and increments the requests-per-minute counter for a token.
// Returns (allowed bool, remaining int, err error).
func (r *RateLimiter) CheckRPM(ctx context.Context, tokenID string, limitRPM int) (bool, int, error) {
	return r.checkSlidingWindow(ctx, fmt.Sprintf("ratelimit:rpm:%s", tokenID), limitRPM, time.Minute)
}

// CheckTPM checks and increments the tokens-per-minute counter for a token.
func (r *RateLimiter) CheckTPM(ctx context.Context, tokenID string, tokensUsed, limitTPM int) (bool, int, error) {
	key := fmt.Sprintf("ratelimit:tpm:%s", tokenID)
	// For TPM we use a simple counter rather than sorted set due to variable increment
	return r.checkTokenWindow(ctx, key, tokensUsed, limitTPM, time.Minute)
}

// checkSlidingWindow implements a sliding window counter using a Redis sorted set.
// Each request is stored as a member with score = unix timestamp in nanoseconds.
func (r *RateLimiter) checkSlidingWindow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	now := time.Now()
	windowStart := now.Add(-window)

	pipe := r.redis.Pipeline()

	// Remove expired entries
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// Count current entries
	countCmd := pipe.ZCard(ctx, key)

	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Set expiry on the key
	pipe.Expire(ctx, key, window*2)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, fmt.Errorf("rate limit check: %w", err)
	}

	current := int(countCmd.Val())
	if current >= limit {
		// Remove the entry we just added (rejected request)
		r.redis.ZRemRangeByScore(ctx, key, fmt.Sprintf("%d", now.UnixNano()), fmt.Sprintf("%d", now.UnixNano())) //nolint:errcheck
		return false, 0, nil
	}

	remaining := limit - current - 1
	return true, remaining, nil
}

// checkTokenWindow uses a simple INCRBY + EXPIRE for token-based limits.
func (r *RateLimiter) checkTokenWindow(ctx context.Context, key string, increment, limit int, window time.Duration) (bool, int, error) {
	pipe := r.redis.Pipeline()
	incrCmd := pipe.IncrBy(ctx, key, int64(increment))
	pipe.Expire(ctx, key, window)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, fmt.Errorf("token rate limit check: %w", err)
	}

	total := int(incrCmd.Val())
	if total > limit {
		return false, 0, nil
	}
	return true, limit - total, nil
}

// CheckConcurrent checks concurrent request count for a deployment.
func (r *RateLimiter) CheckConcurrent(ctx context.Context, deploymentID string, maxConcurrent int) (bool, error) {
	key := fmt.Sprintf("active:%s", deploymentID)
	count, err := r.redis.Get(ctx, key).Int()
	if err == redis.Nil {
		count = 0
	} else if err != nil {
		return false, err
	}
	return count < maxConcurrent, nil
}

// IncrementConcurrent increments the active request counter.
func (r *RateLimiter) IncrementConcurrent(ctx context.Context, deploymentID string, ttl time.Duration) error {
	key := fmt.Sprintf("active:%s", deploymentID)
	pipe := r.redis.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// DecrementConcurrent decrements the active request counter.
func (r *RateLimiter) DecrementConcurrent(ctx context.Context, deploymentID string) error {
	key := fmt.Sprintf("active:%s", deploymentID)
	return r.redis.Decr(ctx, key).Err()
}
