package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all service configuration.
type Config struct {
	Service string
	Port    int
	Env     string

	// Database
	DatabaseURL    string
	DBMaxOpenConns int
	DBMaxIdleConns int

	// Redis
	RedisURL      string
	RedisPassword string

	// NATS
	NATSUrl         string
	NATSCredentials string

	// Auth
	JWTSigningKey          string
	JWTExpirySeconds       int
	RefreshTokenExpiryDays int

	// CORS
	CORSAllowedOrigins []string

	// Observability
	OTLPEndpoint string
	LogLevel     string

	// Dynamo
	DynamoFrontendURL string
	DynamoOperatorURL string
}

// Load reads configuration from environment variables (TAAS_*) and a config file.
func Load(service string) (*Config, error) {
	v := viper.New()

	v.SetConfigName(service)
	v.SetConfigType("yaml")
	v.AddConfigPath("/etc/taas")
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	// Environment variable overrides: TAAS_PORT, TAAS_DATABASE_URL, etc.
	v.SetEnvPrefix("TAAS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// Defaults
	v.SetDefault("port", 8080)
	v.SetDefault("env", "production")
	v.SetDefault("db_max_open_conns", 25)
	v.SetDefault("db_max_idle_conns", 5)
	v.SetDefault("jwt_expiry_seconds", 3600)
	v.SetDefault("refresh_token_expiry_days", 30)
	v.SetDefault("log_level", "info")
	v.SetDefault("cors_allowed_origins", []string{})

	// Required in production
	v.SetDefault("jwt_signing_key", "")

	cfg := &Config{
		Service:                service,
		Port:                   v.GetInt("port"),
		Env:                    v.GetString("env"),
		DatabaseURL:            v.GetString("database_url"),
		DBMaxOpenConns:         v.GetInt("db_max_open_conns"),
		DBMaxIdleConns:         v.GetInt("db_max_idle_conns"),
		RedisURL:               v.GetString("redis_url"),
		RedisPassword:          v.GetString("redis_password"),
		NATSUrl:                v.GetString("nats_url"),
		NATSCredentials:        v.GetString("nats_credentials"),
		JWTSigningKey:          v.GetString("jwt_signing_key"),
		JWTExpirySeconds:       v.GetInt("jwt_expiry_seconds"),
		RefreshTokenExpiryDays: v.GetInt("refresh_token_expiry_days"),
		CORSAllowedOrigins:     v.GetStringSlice("cors_allowed_origins"),
		OTLPEndpoint:           v.GetString("otlp_endpoint"),
		LogLevel:               v.GetString("log_level"),
		DynamoFrontendURL:      v.GetString("dynamo_frontend_url"),
		DynamoOperatorURL:      v.GetString("dynamo_operator_url"),
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("database_url is required")
	}
	if c.RedisURL == "" {
		return fmt.Errorf("redis_url is required")
	}
	if c.JWTSigningKey == "" {
		return fmt.Errorf("jwt_signing_key is required")
	}
	if len(c.JWTSigningKey) < 32 {
		return fmt.Errorf("jwt_signing_key must be at least 32 characters")
	}
	// Warn about wildcard CORS in production (P1)
	if c.Env == "production" {
		for _, origin := range c.CORSAllowedOrigins {
			if origin == "*" {
				return fmt.Errorf("CORS wildcard '*' is not allowed in production; specify explicit origins")
			}
		}
	}
	return nil
}
