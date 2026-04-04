package token

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	raw, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}

	if !strings.HasPrefix(raw, "taas_") {
		t.Errorf("expected token to start with 'taas_', got '%s'", raw[:10])
	}

	expectedLen := len(tokenPrefix) + tokenRandomLength
	if len(raw) != expectedLen {
		t.Errorf("expected token length %d, got %d", expectedLen, len(raw))
	}

	// Verify the random part only contains base62 chars
	randomPart := raw[len(tokenPrefix):]
	for i, ch := range randomPart {
		if !strings.ContainsRune(base62Chars, ch) {
			t.Errorf("unexpected character '%c' at position %d in random part", ch, i)
		}
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	tokens := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		raw, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken failed on iteration %d: %v", i, err)
		}
		if tokens[raw] {
			t.Fatalf("duplicate token generated on iteration %d", i)
		}
		tokens[raw] = true
	}
}

func TestHashToken(t *testing.T) {
	raw := "taas_testtoken1234567890abcdef"
	hash := HashToken(raw)

	// Verify it's a valid hex string
	if len(hash) != 64 { // SHA-256 produces 32 bytes = 64 hex chars
		t.Errorf("expected hash length 64, got %d", len(hash))
	}

	// Verify it matches our expected SHA-256
	expected := sha256.Sum256([]byte(raw))
	expectedHex := hex.EncodeToString(expected[:])
	if hash != expectedHex {
		t.Errorf("hash mismatch: expected %s, got %s", expectedHex, hash)
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	raw := "taas_same_token_value"
	hash1 := HashToken(raw)
	hash2 := HashToken(raw)
	if hash1 != hash2 {
		t.Error("HashToken should produce the same hash for the same input")
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	hash1 := HashToken("taas_token_one")
	hash2 := HashToken("taas_token_two")
	if hash1 == hash2 {
		t.Error("different inputs should produce different hashes")
	}
}
