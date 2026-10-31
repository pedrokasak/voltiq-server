package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/voltiq/server/internal/app"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureDatabaseURL(); err != nil {
		fatal("invalid database configuration", err)
	}
	secret, err := jwtSecret()
	if err != nil {
		fatal("invalid jwt configuration", err)
	}
	a, err := app.Build(ctx, app.Options{JWTSecret: secret, CORSOrigins: corsOrigins()})
	if err != nil {
		fatal("startup failed", err)
	}
	defer a.Close()
	if err := seedSuperAdminIfMissing(ctx, a.Users, a.Tenants); err != nil {
		slog.Warn("super admin seed failed", "error", err)
	}

	// "api cron" roda todos os jobs uma vez e sai (agendadores externos).
	if len(os.Args) > 1 && os.Args[1] == "cron" {
		a.Scheduler.RunAll(ctx)
		return
	}

	a.Alerts.Start(ctx, getEnvAsInt("ALERT_WORKERS", 2))
	if getEnv("SCHEDULER_ENABLED", "true") == "true" {
		a.Scheduler.Start(ctx)
	}

	port := getEnv("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           a.Router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      3 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("server failed", err)
		}
	}()
	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
	a.Scheduler.Wait()
}

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func isProduction() bool {
	return getEnv("APP_ENV", "") == "production"
}

func ensureDatabaseURL() error {
	if os.Getenv("DATABASE_URL") != "" {
		return nil
	}
	if isProduction() {
		return errors.New("DATABASE_URL is required in production")
	}
	return os.Setenv("DATABASE_URL", fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		getEnv("DATABASE_USER", "postgres"), getEnv("DATABASE_PASSWORD", "postgres"),
		getEnv("DATABASE_HOST", "localhost"), getEnv("DATABASE_PORT", "5432"),
		getEnv("DATABASE_NAME", "voltiq-sw"), getEnv("DATABASE_SSL_MODE", "disable")))
}

func jwtSecret() (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) >= 32 {
		return secret, nil
	}
	if isProduction() {
		return "", errors.New("JWT_SECRET must have at least 32 characters in production")
	}
	slog.Warn("JWT_SECRET missing or short; using development secret")
	return "dev-only-secret-change-me-0123456789abcdef", nil
}

// corsOrigins reads CORS_ALLOWED_ORIGINS (comma separated). Credentials are
// enabled, so "*" is never used.
func corsOrigins() []string {
	raw := getEnv("CORS_ALLOWED_ORIGINS", "")
	if raw == "" {
		if isProduction() {
			return []string{getEnv("DASHBOARD_URL", "https://app.voltiq.com.br")}
		}
		return []string{"http://localhost:5173", "http://localhost:8080", "http://localhost:3000"}
	}
	var out []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			out = append(out, strings.TrimRight(o, "/"))
		}
	}
	return out
}

// seedSuperAdminIfMissing creates the development SUPER_ADMIN. Never runs in
// production; the password comes from SEED_SUPER_ADMIN_PASSWORD and is
// never logged.
func seedSuperAdminIfMissing(ctx context.Context, userRepo *repository.UserRepository, tenantRepo *repository.TenantRepository) error {
	if isProduction() {
		return nil
	}
	password := os.Getenv("SEED_SUPER_ADMIN_PASSWORD")
	if password == "" {
		return nil
	}
	seedEmail := getEnv("SEED_SUPER_ADMIN_EMAIL", "super@admin.local")
	existing, err := userRepo.GetByEmail(ctx, seedEmail)
	if err != nil || existing != nil {
		return err
	}
	tenants, err := tenantRepo.ListAll(ctx)
	if err != nil {
		return err
	}
	if len(tenants) == 0 {
		slog.Info("super admin seed skipped: no tenants yet")
		return nil
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	now := time.Now()
	if err := userRepo.Create(ctx, &domain.User{
		ID: domain.UUID(uuid.New().String()), TenantID: tenants[0].ID, Email: seedEmail,
		Name: "Voltiq Super Admin (dev)", PasswordHash: string(hashed), Role: domain.UserRoleSuperAdmin,
		Active: true, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return err
	}
	slog.Info("super admin seeded (dev only)", "email", seedEmail)
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return defaultValue
}
