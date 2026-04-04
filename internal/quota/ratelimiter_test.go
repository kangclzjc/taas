package quota

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestLimiter(t *testing.T) (*RateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRateLimiter(rdb), mr
}

func TestCheckRPM_UnderLimit(t *testing.T) {
	limiter, mr := setupTestLimiter(t)
	defer mr.Close()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		allowed, remaining, err := limiter.CheckRPM(ctx, "token-1", 10)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if !allowed {
			t.Errorf("iteration %d: expected allowed", i)
		}
		if remaining < 0 {
			t.Errorf("iteration %d: negative remaining %d", i, remaining)
		}
	}
}

func TestCheckRPM_AtLimit(t *testing.T) {
	limiter, mr := setupTestLimiter(t)
	defer mr.Close()

	ctx := context.Background()
	limit := 5

	for i := 0; i < limit; i++ {
		allowed, _, err := limiter.CheckRPM(ctx, "token-2", limit)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if !allowed {
			t.Errorf("iteration %d: expected allowed within limit", i)
		}
	}

	// This should be rejected
	allowed, _, err := limiter.CheckRPM(ctx, "token-2", limit)
	if err != nil {
		t.Fatalf("over-limit: %v", err)
	}
	if allowed {
		t.Error("expected request over limit to be rejected")
	}
}

func TestCheckTPM_UnderLimit(t *testing.T) {
	limiter, mr := setupTestLimiter(t)
	defer mr.Close()

	ctx := context.Background()
	allowed, remaining, err := limiter.CheckTPM(ctx, "token-3", 500, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected allowed under limit")
	}
	if remaining != 9500 {
		t.Errorf("expected 9500 remaining, got %d", remaining)
	}
}

func TestCheckTPM_OverLimit(t *testing.T) {
	limiter, mr := setupTestLimiter(t)
	defer mr.Close()

	ctx := context.Background()

	// First: use up the budget
	limiter.CheckTPM(ctx, "token-4", 9000, 10000)

	// Second: go over
	allowed, _, err := limiter.CheckTPM(ctx, "token-4", 2000, 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected rejected over TPM limit")
	}
}
