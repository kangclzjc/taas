package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowLuaScript atomically checks and increments a sliding window rate limiter.
// Returns {1, remaining} on allow, {0, 0} on reject.
const slidingWindowLuaScript = `
local key = KEYS[1]
local window_start = ARGV[1]
local now = ARGV[2]
local limit = tonumber(ARGV[3])

-- Remove expired entries
redis.call('ZREMRANGEBYSCORE', key, '0', window_start)

-- Count current entries
local count = redis.call('ZCARD', key)

if count >= limit then
    return {0, 0}
end

-- Add current request
redis.call('ZADD', key, now, now)
redis.call('EXPIRE', key, math.ceil(tonumber(ARGV[4])))

return {1, limit - count - 1}
`

var slidingWindowScript = redis.NewScript(slidingWindowLuaScript)

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

// checkSlidingWindow implements a sliding window counter using a Redis sorted set
// with an atomic Lua script. Each request is stored as a member with score = unix
// timestamp in nanoseconds.
func (r *RateLimiter) checkSlidingWindow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	now := time.Now()
	windowStart := now.Add(-window)
	expireSeconds := window.Seconds() * 2

	result, err := slidingWindowScript.Run(ctx, r.redis,
		[]string{key},
		fmt.Sprintf("%d", windowStart.UnixNano()),
		fmt.Sprintf("%d", now.UnixNano()),
		limit,
		fmt.Sprintf("%f", expireSeconds),
	).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit check: %w", err)
	}

	allowed := result[0] == 1
	remaining := int(result[1])
	return allowed, remaining, nil
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
