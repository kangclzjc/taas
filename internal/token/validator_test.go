package token

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestValidator(t *testing.T) (*Validator, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	dbLookup := func(ctx context.Context, hash string) (*CachedTokenInfo, error) {
		return nil, nil // default: not found
	}

	return NewValidator(rdb, dbLookup), mr
}

func TestValidate_CacheHit(t *testing.T) {
	v, mr := setupTestValidator(t)
	defer mr.Close()

	rawToken := "taas_testtoken123"
	hash := HashToken(rawToken)
	info := &CachedTokenInfo{
		TokenID:  "tok-1",
		UserID:   "user-1",
		OrgID:    "org-1",
		IsActive: true,
		SLATier:  "standard",
	}
	data, _ := json.Marshal(info)
	mr.Set(tokenCachePrefix+hash, string(data))

	result, err := v.Validate(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.TokenID != "tok-1" {
		t.Errorf("expected tok-1, got %s", result.TokenID)
	}
}

func TestValidate_CacheMiss_DBHit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	dbInfo := &CachedTokenInfo{
		TokenID:  "tok-db",
		UserID:   "user-db",
		OrgID:    "org-db",
		IsActive: true,
	}
	dbLookup := func(ctx context.Context, hash string) (*CachedTokenInfo, error) {
		return dbInfo, nil
	}
	v := NewValidator(rdb, dbLookup)

	result, err := v.Validate(context.Background(), "taas_sometoken")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.TokenID != "tok-db" {
		t.Errorf("expected tok-db, got %s", result.TokenID)
	}

	// Verify cache was populated
	hash := HashToken("taas_sometoken")
	if !mr.Exists(tokenCachePrefix + hash) {
		t.Error("expected cache to be populated")
	}
}

func TestValidate_TokenNotFound(t *testing.T) {
	v, mr := setupTestValidator(t)
	defer mr.Close()

	_, err := v.Validate(context.Background(), "taas_nonexistent")
	if err != ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound, got: %v", err)
	}
}

func TestValidate_TokenRevoked(t *testing.T) {
	v, mr := setupTestValidator(t)
	defer mr.Close()

	rawToken := "taas_revokedtoken"
	hash := HashToken(rawToken)
	info := &CachedTokenInfo{TokenID: "tok-r", IsActive: false}
	data, _ := json.Marshal(info)
	mr.Set(tokenCachePrefix+hash, string(data))

	_, err := v.Validate(context.Background(), rawToken)
	if err != ErrTokenRevoked {
		t.Errorf("expected ErrTokenRevoked, got: %v", err)
	}
}

func TestValidate_TokenExpired(t *testing.T) {
	v, mr := setupTestValidator(t)
	defer mr.Close()

	rawToken := "taas_expiredtoken"
	hash := HashToken(rawToken)
	past := time.Now().Add(-1 * time.Hour)
	info := &CachedTokenInfo{TokenID: "tok-e", IsActive: true, ExpiresAt: past}
	data, _ := json.Marshal(info)
	mr.Set(tokenCachePrefix+hash, string(data))

	_, err := v.Validate(context.Background(), rawToken)
	if err != ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestInvalidate(t *testing.T) {
	v, mr := setupTestValidator(t)
	defer mr.Close()

	tokenHash := "abc123hash"
	mr.Set(tokenCachePrefix+tokenHash, "some-data")

	if !mr.Exists(tokenCachePrefix + tokenHash) {
		t.Fatal("expected key to exist before invalidate")
	}

	err := v.Invalidate(context.Background(), tokenHash)
	if err != nil {
		t.Fatalf("invalidate error: %v", err)
	}

	if mr.Exists(tokenCachePrefix + tokenHash) {
		t.Error("expected key to be deleted after invalidate")
	}
}
