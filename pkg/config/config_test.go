package config

import (
	"testing"
)

// We test the validate() method directly on a Config struct to avoid
// needing config files or environment variables.

func validConfig() *Config {
	return &Config{
		Service:      "test",
		Port:         8080,
		Env:          "test",
		DatabaseURL:  "postgres://localhost/test",
		RedisURL:     "redis://localhost:6379",
		JWTSigningKey: "this-is-a-very-long-jwt-signing-key-at-least-32-chars",
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := validConfig()
	if err := cfg.validate(); err != nil {
		t.Errorf("expected valid config to pass validation, got: %v", err)
	}
}

func TestConfig_Validate_MissingDatabaseURL(t *testing.T) {
	cfg := validConfig()
	cfg.DatabaseURL = ""
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for missing database_url")
	}
}

func TestConfig_Validate_MissingRedisURL(t *testing.T) {
	cfg := validConfig()
	cfg.RedisURL = ""
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for missing redis_url")
	}
}

func TestConfig_Validate_MissingJWTKey(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSigningKey = ""
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for missing jwt_signing_key")
	}
}

func TestConfig_Validate_ShortJWTKey(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSigningKey = "short-key" // < 32 chars
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for short jwt_signing_key")
	}
}

func TestConfig_Validate_InvalidPort_Zero(t *testing.T) {
	cfg := validConfig()
	cfg.Port = 0
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for port 0")
	}
}

func TestConfig_Validate_InvalidPort_Negative(t *testing.T) {
	cfg := validConfig()
	cfg.Port = -1
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for negative port")
	}
}

func TestConfig_Validate_InvalidPort_TooHigh(t *testing.T) {
	cfg := validConfig()
	cfg.Port = 70000
	err := cfg.validate()
	if err == nil {
		t.Error("expected error for port > 65535")
	}
}

func TestConfig_Validate_BoundaryPort_65535(t *testing.T) {
	cfg := validConfig()
	cfg.Port = 65535
	if err := cfg.validate(); err != nil {
		t.Errorf("port 65535 should be valid, got: %v", err)
	}
}

func TestConfig_Validate_BoundaryPort_1(t *testing.T) {
	cfg := validConfig()
	cfg.Port = 1
	if err := cfg.validate(); err != nil {
		t.Errorf("port 1 should be valid, got: %v", err)
	}
}

func TestConfig_Validate_JWTKeyExactly32(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSigningKey = "12345678901234567890123456789012" // exactly 32
	if err := cfg.validate(); err != nil {
		t.Errorf("jwt_signing_key of exactly 32 chars should be valid, got: %v", err)
	}
}
