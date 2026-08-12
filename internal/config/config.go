package config

import (
	"os"
	"strconv"
	"time"
)

// EmailConfig holds email configuration
type EmailConfig struct {
	Provider    string
	ResendAPIKey string
	FromEmail   string
	FromName    string
	ReplyTo     string
	BaseURL     string
}

// LoadEmailConfig loads email configuration from environment variables
func LoadEmailConfig() *EmailConfig {
	return &EmailConfig{
		Provider:    getEnv("EMAIL_PROVIDER", "resend"),
		ResendAPIKey: getEnv("RESEND_API_KEY", ""),
		FromEmail:   getEnv("EMAIL_FROM_EMAIL", "noreply@voltiq.com.br"),
		FromName:    getEnv("EMAIL_FROM_NAME", "Voltiq Software"),
		ReplyTo:     getEnv("EMAIL_REPLY_TO", "suporte@voltiq.com.br"),
		BaseURL:     getEnv("APP_BASE_URL", "https://app.voltiq.com.br"),
	}
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	URL      string
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// LoadDatabaseConfig loads database configuration from environment variables
func LoadDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		URL:      getEnv("DATABASE_URL", ""),
		Host:     getEnv("DATABASE_HOST", "localhost"),
		Port:     getEnv("DATABASE_PORT", "5432"),
		User:     getEnv("DATABASE_USER", "postgres"),
		Password: getEnv("DATABASE_PASSWORD", "postgres"),
		Name:     getEnv("DATABASE_NAME", "voltiq-sw"),
		SSLMode:  getEnv("DATABASE_SSL_MODE", "disable"),
	}
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret                string
	ExpirationHours       int
	RefreshExpirationDays int
}

// LoadJWTConfig loads JWT configuration from environment variables
func LoadJWTConfig() *JWTConfig {
	return &JWTConfig{
		Secret:                getEnv("JWT_SECRET", "default-secret-key-change-in-production"),
		ExpirationHours:       getEnvAsInt("JWT_EXPIRATION_HOURS", 24),
		RefreshExpirationDays: getEnvAsInt("REFRESH_TOKEN_EXPIRATION_DAYS", 7),
	}
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port        string
	Environment string
}

// LoadServerConfig loads server configuration from environment variables
func LoadServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:        getEnv("PORT", "8080"),
		Environment: getEnv("APP_ENV", "development"),
	}
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int
	Burst             int
}

// LoadRateLimitConfig loads rate limiting configuration from environment variables
func LoadRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RequestsPerMinute: getEnvAsInt("RATE_LIMIT_REQUESTS_PER_MINUTE", 60),
		Burst:             getEnvAsInt("RATE_LIMIT_BURST", 30),
	}
}

// AppConfig holds all application configuration
type AppConfig struct {
	Email      *EmailConfig
	Database   *DatabaseConfig
	JWT        *JWTConfig
	Server     *ServerConfig
	RateLimit  *RateLimitConfig
}

// LoadAppConfig loads all application configuration
func LoadAppConfig() *AppConfig {
	return &AppConfig{
		Email:      LoadEmailConfig(),
		Database:   LoadDatabaseConfig(),
		JWT:        LoadJWTConfig(),
		Server:     LoadServerConfig(),
		RateLimit:  LoadRateLimitConfig(),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}