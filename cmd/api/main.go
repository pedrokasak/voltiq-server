package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/voltiq/server/internal/delivery/handler"
	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/router"
	"github.com/voltiq/server/internal/ingestion"
	"github.com/voltiq/server/internal/jwt"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
	"github.com/voltiq/server/pkg/metrics"
)

func main() {
	ctx := context.Background()

	// Load environment variables
	databaseURL := getEnv("DATABASE_URL", "")
	if databaseURL == "" {
		// Use individual env vars if DATABASE_URL is not set
		dbHost := getEnv("DATABASE_HOST", "localhost")
		dbPort := getEnv("DATABASE_PORT", "5432")
		dbUser := getEnv("DATABASE_USER", "postgres")
		dbPassword := getEnv("DATABASE_PASSWORD", "postgres")
		dbName := getEnv("DATABASE_NAME", "voltiq")
		dbSSLMode := getEnv("DATABASE_SSL_MODE", "disable")

		databaseURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			dbUser, dbPassword, dbHost, dbPort, dbName, dbSSLMode)
	}

	// Connect to database
	os.Setenv("DATABASE_URL", databaseURL)
	db, err := repository.NewDatabase(ctx)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize JWT service
	jwtSecret := getEnv("JWT_SECRET", "default-secret-key-change-in-production")
	jwtExpirationHours := getEnvAsInt("JWT_EXPIRATION_HOURS", 24)
	refreshTokenExpirationDays := getEnvAsInt("REFRESH_TOKEN_EXPIRATION_DAYS", 7)

	jwtService := jwt.NewService(jwtSecret)
	jwtService.SetExpiration(time.Duration(jwtExpirationHours) * time.Hour)
	jwtService.SetRefreshExpiration(time.Duration(refreshTokenExpirationDays) * 24 * time.Hour)

	// Initialize repositories
	userRepo := repository.NewUserRepository(db)
	tenantRepo := repository.NewTenantRepository(db)
	transformerRepo := repository.NewTransformerRepository(db)
	ucRepo := repository.NewConsumingUnitRepository(db)
	transformerReadingRepo := repository.NewTransformerReadingRepository(db)
	ucReadingRepo := repository.NewUCReadingRepository(db)
	balanceRepo := repository.NewBalanceRepository(db)
	importRepo := repository.NewImportRepository(db)
	inviteRepo := repository.NewInviteRepository(db)

	// Initialize use cases
	authUseCase := usecase.NewAuthUseCase(userRepo, tenantRepo, jwtService)
	signupUseCase := usecase.NewSignupUseCase(tenantRepo, userRepo)
	inviteUseCase := usecase.NewInviteUseCase(inviteRepo, userRepo, tenantRepo)
	transformerUseCase := usecase.NewTransformerUseCase(transformerRepo)
	ucUseCase := usecase.NewConsumingUnitUseCase(ucRepo)
	balanceUseCase := usecase.NewBalanceUseCase(balanceRepo, transformerRepo, transformerReadingRepo, ucReadingRepo, ucRepo)
	importUseCase := usecase.NewImportUseCase(ingestion.NewCSVParser(), importRepo, transformerReadingRepo, ucReadingRepo)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authUseCase, signupUseCase)
	inviteHandler := handler.NewInviteHandler(inviteUseCase)
	transformerHandler := handler.NewTransformerHandler(transformerUseCase)
	ucHandler := handler.NewConsumingUnitHandler(ucUseCase)
	balanceHandler := handler.NewBalanceHandler(balanceUseCase)
	importHandler := handler.NewImportHandler(importUseCase)
	healthHandler := handler.NewHealthHandler("0.1.0")

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(jwtService)

	// Rate limiting
	rateLimitRequests := getEnvAsInt("RATE_LIMIT_REQUESTS_PER_MINUTE", 60)
	rateLimitBurst := getEnvAsInt("RATE_LIMIT_BURST", 30)
	rateLimiter := middleware.NewRateLimiter(rateLimitRequests, rateLimitBurst)

	// Security headers
	securityMiddleware := middleware.NewSecurityMiddleware()

	// CORS
	corsMiddleware := middleware.NewCORSMiddleware(middleware.CORSConfig{
		AllowedOrigins:   strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ","),
		AllowCredentials: true,
	})

	metricsCollector := metrics.NewMetricsCollector()

	// Setup router
	cfg := router.Config{
		AuthHandler:          authHandler,
		InviteHandler:        inviteHandler,
		TransformerHandler:   transformerHandler,
		ConsumingUnitHandler: ucHandler,
		BalanceHandler:       balanceHandler,
		ImportHandler:        importHandler,
		HealthHandler:        healthHandler,
		MetricsCollector:     metricsCollector,
		AuthMiddleware:       authMiddleware,
		RateLimiter:          rateLimiter,
		SecurityMiddleware:   securityMiddleware,
		CORSOrigins:          strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ","),
	}

	r := router.Setup(cfg)

	// Apply CORS (must be after router setup)
	r.Use(corsMiddleware.Handler)

	// Start server
	port := getEnv("PORT", "8080")
	addr := ":" + port

	log.Printf("Server starting on %s", addr)
	log.Printf("Health check: http://localhost:%s/health", port)
	log.Printf("Metrics: http://localhost:%s/metrics", port)
	log.Printf("API docs: http://localhost:%s/api/v1", port)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// Helper functions
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
