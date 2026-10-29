package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/voltiq/server/internal/aneel"
	"github.com/voltiq/server/internal/config"
	"github.com/voltiq/server/internal/delivery/handler"
	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/router"
	"github.com/voltiq/server/internal/email"
	"github.com/voltiq/server/internal/ingestion"
	"github.com/voltiq/server/internal/jobs"
	"github.com/voltiq/server/internal/jwt"
	"github.com/voltiq/server/internal/notify"
	"github.com/voltiq/server/internal/payment"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
	"github.com/voltiq/server/pkg/metrics"
)

// Options are the settings that are not read from the environment.
type Options struct {
	JWTSecret        string
	CORSOrigins      []string
	DisableRateLimit bool // testes de integracao
}

// App holds every wired dependency of the API.
type App struct {
	DB        *repository.Database
	Router    http.Handler
	Scheduler *jobs.Scheduler
	Alerts    *usecase.AlertUseCase
	Limits    *middleware.RateLimits
	Users     *repository.UserRepository
	Tenants   *repository.TenantRepository
	Auth      *usecase.AuthUseCase
}

// Close stops background helpers and closes the database.
func (a *App) Close() {
	if a.Limits != nil {
		a.Limits.Stop()
	}
	a.DB.Close()
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

// Build wires every dependency from the environment.
func Build(ctx context.Context, opt Options) (*App, error) {
	db, err := repository.NewDatabase(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	jwtService := jwt.NewService(opt.JWTSecret)
	jwtService.SetExpiration(time.Duration(getEnvAsInt("JWT_EXPIRATION_HOURS", 24)) * time.Hour)
	jwtService.SetRefreshExpiration(time.Duration(getEnvAsInt("REFRESH_TOKEN_EXPIRATION_DAYS", 7)) * 24 * time.Hour)
	middleware.SetTrustProxy(getEnv("TRUST_PROXY", "false") == "true")

	// Repositorios
	userRepo := repository.NewUserRepository(db)
	tenantRepo := repository.NewTenantRepository(db)
	transformerRepo := repository.NewTransformerRepository(db)
	ucRepo := repository.NewConsumingUnitRepository(db)
	substationRepo := repository.NewSubstationRepository(db)
	trafoReadingRepo := repository.NewTransformerReadingRepository(db)
	ucReadingRepo := repository.NewUCReadingRepository(db)
	balanceRepo := repository.NewBalanceRepository(db)
	importRepo := repository.NewImportRepository(db)
	inviteRepo := repository.NewInviteRepository(db)
	alertRepo := repository.NewAlertRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	platformRepo := repository.NewPlatformRepository(db)
	webhookRepo := repository.NewWebhookEventRepository(db)
	dunningRepo := repository.NewDunningRepository(db)
	metricsRepo := repository.NewMetricsRepository(db)
	prorationRepo := repository.NewProrationRepository(db)

	// Gateway de pagamento
	var paymentProvider payment.PaymentProvider
	if key := getEnv("ASAAS_API_KEY", ""); key != "" {
		sandbox := getEnv("ASAAS_SANDBOX", "true") == "true"
		paymentProvider = payment.NewAsaasProvider(key, getEnv("ASAAS_WEBHOOK_KEY", ""), sandbox)
		slog.Info("payment provider configured", "provider", "asaas", "sandbox", sandbox)
	} else {
		slog.Warn("ASAAS_API_KEY not set, payment features disabled")
	}

	// E-mail e WhatsApp
	emailConfig := config.LoadEmailConfig()
	var emailProvider email.EmailProvider
	emailTemplates, err := email.NewTemplateLoader()
	if err != nil {
		return nil, fmt.Errorf("load email templates: %w", err)
	}
	if emailConfig.ResendAPIKey != "" {
		emailProvider = email.NewResendProvider(emailConfig)
		slog.Info("email provider configured", "provider", "resend")
	} else {
		slog.Warn("RESEND_API_KEY not set, email features disabled")
	}
	var whatsapp notify.WhatsAppSender
	if zcfg := notify.ZAPIConfigFromEnv(); zcfg.Configured() {
		whatsapp = notify.NewZAPISender(zcfg)
		slog.Info("whatsapp provider configured", "provider", "z-api")
	}
	appURL := emailConfig.BaseURL

	// Casos de uso
	authUC := usecase.NewAuthUseCase(userRepo, tenantRepo, jwtService)
	signupUC := usecase.NewSignupUseCase(db, tenantRepo, userRepo)
	inviteUC := usecase.NewInviteUseCase(inviteRepo, userRepo, tenantRepo)
	inviteUC.SetEmail(emailProvider, appURL)
	accountUC := usecase.NewAccountUseCase(userRepo, tenantRepo, inviteRepo, accountRepo)
	transformerUC := usecase.NewTransformerUseCase(transformerRepo, tenantRepo, balanceRepo)
	ucUC := usecase.NewConsumingUnitUseCase(ucRepo, transformerRepo, ucReadingRepo)
	balanceUC := usecase.NewBalanceUseCase(balanceRepo, transformerRepo, trafoReadingRepo, ucReadingRepo, ucRepo)
	alertUC := usecase.NewAlertUseCase(alertRepo, usecase.AlertDeps{
		UserRepo: userRepo, TenantRepo: tenantRepo, EmailProvider: emailProvider, Templates: emailTemplates,
		WhatsApp: whatsapp, AppURL: appURL,
	})
	balanceUC.SetNotifier(alertUC)
	importUC := usecase.NewImportUseCase(db, ingestion.NewCSVParser(), importRepo, transformerRepo,
		trafoReadingRepo, ucRepo, ucReadingRepo, balanceUC)
	registryUC := usecase.NewRegistryImportUseCase(db, transformerRepo, ucRepo, substationRepo, transformerUC)
	reportUC := usecase.NewReportUseCase(balanceRepo, transformerRepo, trafoReadingRepo, ucReadingRepo, transformerUC)
	exportUC := usecase.NewExportUseCase(balanceRepo, transformerRepo, tenantRepo, trafoReadingRepo, ucReadingRepo, reportUC)
	riskUC := usecase.NewRiskUseCase(balanceRepo, transformerRepo, ucRepo)
	networkUC := usecase.NewNetworkUseCase(substationRepo, transformerRepo, ucRepo, trafoReadingRepo, ucReadingRepo, balanceUC)
	dashboardUC := usecase.NewDashboardUseCase(transformerRepo, balanceRepo, substationRepo, accountRepo, alertRepo, importRepo)
	billingUC := usecase.NewBillingUseCase(tenantRepo, userRepo, paymentProvider, webhookRepo, dunningRepo,
		metricsRepo, prorationRepo, emailProvider, emailTemplates)
	superAdminUC := usecase.NewSuperAdminUseCase(tenantRepo, userRepo, platformRepo, accountRepo, metricsRepo, billingUC)

	var limits *middleware.RateLimits
	if !opt.DisableRateLimit {
		limits = middleware.NewRateLimits()
	}
	r := router.Setup(router.Config{
		AuthHandler:          handler.NewAuthHandler(authUC, signupUC),
		AccountHandler:       handler.NewAccountHandler(accountUC),
		InviteHandler:        handler.NewInviteHandler(inviteUC, authUC),
		TransformerHandler:   handler.NewTransformerHandler(transformerUC, registryUC),
		ConsumingUnitHandler: handler.NewConsumingUnitHandler(ucUC, registryUC),
		NetworkHandler:       handler.NewNetworkHandler(networkUC),
		BalanceHandler:       handler.NewBalanceHandler(balanceUC, reportUC),
		ImportHandler:        handler.NewImportHandler(importUC),
		AlertHandler:         handler.NewAlertHandler(alertUC),
		AneelHandler: handler.NewAneelHandler(aneel.NewClient(getEnv("ANEEL_OPEN_DATA_URL", ""),
			getEnv("ANEEL_TARIFF_RESOURCE_ID", ""), nil)),
		RiskHandler:            handler.NewRiskHandler(riskUC),
		DashboardHandler:       handler.NewDashboardHandler(dashboardUC, balanceRepo),
		ExportHandler:          handler.NewExportHandler(exportUC, reportUC),
		SuperAdminHandler:      handler.NewSuperAdminHandler(superAdminUC),
		BillingHandler:         handler.NewBillingHandler(billingUC),
		WebhookHandler:         handler.NewWebhookHandler(billingUC, paymentProvider),
		HealthHandler:          handler.NewHealthHandler("1.0.0"),
		MetricsCollector:       metrics.NewMetricsCollector(),
		AuthMiddleware:         middleware.NewAuthMiddleware(jwtService, db),
		RateLimits:             limits,
		SecurityMiddleware:     middleware.NewSecurityMiddleware(),
		TenantStatusMiddleware: middleware.NewTenantStatusMiddleware(tenantRepo),
		CORSOrigins:            opt.CORSOrigins,
	})

	var billingForJobs *usecase.BillingUseCase
	if paymentProvider != nil || emailProvider != nil {
		billingForJobs = billingUC
	}
	scheduler := jobs.NewScheduler(db, jobs.StandardJobs(jobs.Deps{
		TenantRepo: tenantRepo, BalanceRepo: balanceRepo, Billing: billingForJobs, Balance: balanceUC, Alerts: alertUC,
	})...)

	return &App{DB: db, Router: r, Scheduler: scheduler, Alerts: alertUC, Limits: limits,
		Users: userRepo, Tenants: tenantRepo, Auth: authUC}, nil
}
