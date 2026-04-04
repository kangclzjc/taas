package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestBlocklist(t *testing.T) (*Blocklist, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewBlocklist(rdb), mr
}

func TestBlocklist_AddAndCheck(t *testing.T) {
	bl, mr := setupTestBlocklist(t)
	defer mr.Close()

	ctx := context.Background()
	jti := "test-jti-123"

	// Initially not blocked
	blocked, err := bl.IsBlocked(ctx, jti)
	if err != nil {
		t.Fatalf("IsBlocked: %v", err)
	}
	if blocked {
		t.Error("expected jti to not be blocked initially")
	}

	// Add to blocklist
	err = bl.Add(ctx, jti, 5*time.Minute)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Now should be blocked
	blocked, err = bl.IsBlocked(ctx, jti)
	if err != nil {
		t.Fatalf("IsBlocked after add: %v", err)
	}
	if !blocked {
		t.Error("expected jti to be blocked after adding")
	}
}

func TestBlocklist_TTLExpiry(t *testing.T) {
	bl, mr := setupTestBlocklist(t)
	defer mr.Close()

	ctx := context.Background()
	jti := "test-jti-expire"

	err := bl.Add(ctx, jti, 1*time.Second)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	blocked, err := bl.IsBlocked(ctx, jti)
	if err != nil {
		t.Fatalf("IsBlocked: %v", err)
	}
	if !blocked {
		t.Error("expected blocked right after add")
	}

	// Fast forward time in miniredis
	mr.FastForward(2 * time.Second)

	blocked, err = bl.IsBlocked(ctx, jti)
	if err != nil {
		t.Fatalf("IsBlocked after expiry: %v", err)
	}
	if blocked {
		t.Error("expected jti to be unblocked after TTL expiry")
	}
}

func TestBlocklist_ZeroTTL(t *testing.T) {
	bl, mr := setupTestBlocklist(t)
	defer mr.Close()

	ctx := context.Background()
	jti := "test-jti-zero-ttl"

	// Add with zero TTL should be a no-op
	err := bl.Add(ctx, jti, 0)
	if err != nil {
		t.Fatalf("Add with zero TTL: %v", err)
	}

	blocked, err := bl.IsBlocked(ctx, jti)
	if err != nil {
		t.Fatalf("IsBlocked: %v", err)
	}
	if blocked {
		t.Error("expected not blocked when added with zero TTL")
	}
}

func TestBlocklist_NegativeTTL(t *testing.T) {
	bl, mr := setupTestBlocklist(t)
	defer mr.Close()

	ctx := context.Background()
	jti := "test-jti-negative-ttl"

	// Add with negative TTL should be a no-op (token already expired)
	err := bl.Add(ctx, jti, -1*time.Minute)
	if err != nil {
		t.Fatalf("Add with negative TTL: %v", err)
	}

	blocked, err := bl.IsBlocked(ctx, jti)
	if err != nil {
		t.Fatalf("IsBlocked: %v", err)
	}
	if blocked {
		t.Error("expected not blocked when added with negative TTL")
	}
}
