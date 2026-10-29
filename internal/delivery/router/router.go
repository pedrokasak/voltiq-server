package router

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/voltiq/server/internal/delivery/handler"
	mw "github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/pkg/metrics"
)

// Config holds router configuration
type Config struct {
	AuthHandler            *handler.AuthHandler
	AccountHandler         *handler.AccountHandler
	InviteHandler          *handler.InviteHandler
	TransformerHandler     *handler.TransformerHandler
	ConsumingUnitHandler   *handler.ConsumingUnitHandler
	NetworkHandler         *handler.NetworkHandler
	BalanceHandler         *handler.BalanceHandler
	ImportHandler          *handler.ImportHandler
	AlertHandler           *handler.AlertHandler
	RiskHandler            *handler.RiskHandler
	DashboardHandler       *handler.DashboardHandler
	ExportHandler          *handler.ExportHandler
	SuperAdminHandler      *handler.SuperAdminHandler
	BillingHandler         *handler.BillingHandler
	WebhookHandler         *handler.WebhookHandler
	HealthHandler          *handler.HealthHandler
	MetricsCollector       *metrics.MetricsCollector
	AuthMiddleware         *mw.AuthMiddleware
	RateLimits             *mw.RateLimits
	SecurityMiddleware     *mw.SecurityMiddleware
	TenantStatusMiddleware *mw.TenantStatusMiddleware
	CORSOrigins            []string
}

func perm(p domain.Permission) func(http.Handler) http.Handler {
	return mw.RequirePermission(string(p))
}

// limit applies a rate limiter when configured.
func limit(rl *mw.RateLimiter) func(http.Handler) http.Handler {
	if rl == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return rl.Middleware
}

// Setup creates and configures the Chi router
func Setup(cfg Config) *chi.Mux {
	r := chi.NewRouter()
	rl := cfg.RateLimits
	if rl == nil {
		rl = &mw.RateLimits{}
	}

	// Global middleware - Order matters!
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(120 * time.Second))
	if cfg.SecurityMiddleware != nil {
		r.Use(cfg.SecurityMiddleware.Handler)
	}
	r.Use(mw.ContentTypeMiddleware)
	if len(cfg.CORSOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   cfg.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Request-ID", "X-Requested-With"},
			ExposedHeaders:   []string{"X-Request-ID", "X-Total-Count", "Content-Disposition", "Retry-After"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}
	r.Use(limit(rl.Default))

	r.Get("/health", cfg.HealthHandler.Health)
	r.Get("/ready", cfg.HealthHandler.Ready)
	if cfg.MetricsCollector != nil {
		r.Handle("/metrics", http.HandlerFunc(cfg.MetricsCollector.ServeHTTP))
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Rotas publicas
		r.Group(func(r chi.Router) {
			r.With(limit(rl.Login)).Post("/auth/login", cfg.AuthHandler.Login)
			r.With(limit(rl.Login)).Post("/auth/signup", cfg.AuthHandler.Signup)
			r.With(limit(rl.Refresh)).Post("/auth/refresh", cfg.AuthHandler.RefreshToken)
			r.Post("/auth/logout", cfg.AuthHandler.Logout)
			r.Get("/invites/{token}", cfg.InviteHandler.ValidateInvite)
			r.With(limit(rl.Login)).Post("/invites/{token}/accept", cfg.InviteHandler.AcceptInvite)
			r.Post("/webhooks/asaas", cfg.WebhookHandler.HandleAsaasWebhook)
		})

		// Rotas autenticadas do tenant
		r.Group(func(r chi.Router) {
			r.Use(cfg.AuthMiddleware.Chain())
			if cfg.TenantStatusMiddleware != nil {
				r.Use(cfg.TenantStatusMiddleware.Handler)
			}

			// Perfil, sessao e configuracoes
			r.Get("/auth/me", cfg.AccountHandler.Me)
			r.Post("/auth/change-password", cfg.AuthHandler.ChangePassword)
			r.Get("/roles", cfg.AccountHandler.Roles)
			r.Get("/onboarding", cfg.AccountHandler.Onboarding)
			r.With(perm(domain.PermissionTenantManage)).Post("/onboarding/complete", cfg.AccountHandler.CompleteOnboarding)
			r.Get("/tenant/settings", cfg.AccountHandler.GetSettings)
			r.With(perm(domain.PermissionTenantManage)).Put("/tenant/settings", cfg.AccountHandler.UpdateSettings)
			r.With(perm(domain.PermissionAuditView)).Get("/audit-logs", cfg.AccountHandler.AuditLog)

			// Usuarios e convites (RBAC)
			r.Route("/users", func(r chi.Router) {
				r.Use(perm(domain.PermissionUserManage))
				r.Get("/", cfg.AccountHandler.ListUsers)
				r.Patch("/{id}", cfg.AccountHandler.UpdateUser)
				r.Delete("/{id}", cfg.AccountHandler.RemoveUser)
			})
			r.Route("/invites", func(r chi.Router) {
				r.Use(perm(domain.PermissionUserManage))
				r.Post("/", cfg.InviteHandler.CreateInvite)
				r.Get("/", cfg.InviteHandler.ListInvites)
				r.Delete("/{token}", cfg.InviteHandler.CancelInvite)
				r.Post("/{token}/resend", cfg.InviteHandler.ResendInvite)
			})

			// Subestacoes
			r.Route("/substations", func(r chi.Router) {
				r.With(perm(domain.PermissionTransformerView)).Get("/", cfg.NetworkHandler.ListSubstations)
				r.With(perm(domain.PermissionTransformerView)).Get("/{id}", cfg.NetworkHandler.GetSubstation)
				r.With(perm(domain.PermissionTransformerManage)).Post("/", cfg.NetworkHandler.CreateSubstation)
				r.With(perm(domain.PermissionTransformerManage)).Put("/{id}", cfg.NetworkHandler.UpdateSubstation)
				r.With(perm(domain.PermissionTransformerManage)).Delete("/{id}", cfg.NetworkHandler.DeleteSubstation)
			})

			// Transformadores
			r.Route("/transformers", func(r chi.Router) {
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionTransformerView))
					r.Get("/", cfg.TransformerHandler.List)
					r.Get("/count", cfg.TransformerHandler.Count)
					r.Get("/with-count", cfg.TransformerHandler.GetAllWithCount)
					r.Get("/paginated", cfg.TransformerHandler.GetWithPagination)
					r.Get("/code/{code}", cfg.TransformerHandler.GetByCode)
					r.Get("/code/{code}/exists", cfg.TransformerHandler.ExistsByCode)
					r.Get("/substation/{substation_id}", cfg.TransformerHandler.ListBySubstation)
					r.Get("/{id}", cfg.TransformerHandler.GetByID)
					r.Get("/{id}/location", cfg.TransformerHandler.GetLocation)
					r.Get("/{id}/technical-data", cfg.TransformerHandler.GetTechnicalData)
					r.Get("/{id}/loss-limit", cfg.TransformerHandler.GetLossLimit)
					r.Get("/{id}/substation", cfg.TransformerHandler.GetSubstation)
					r.Get("/{id}/active", cfg.TransformerHandler.GetActive)
					r.Get("/{id}/stats", cfg.TransformerHandler.Stats)
					r.Get("/{id}/loss-analysis", cfg.BalanceHandler.LossAnalysis)
					r.Get("/{id}/readings/series", cfg.BalanceHandler.ReadingSeries)
					r.Get("/{id}/top-consumers", cfg.BalanceHandler.TopConsumers)
					r.Get("/{id}/alert-config", cfg.AlertHandler.RecipientsByTransformer)
				})
				r.With(perm(domain.PermissionReportExport)).Get("/{id}/export", cfg.ExportHandler.ExportTransformerCSV)
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionTransformerManage))
					r.Post("/", cfg.TransformerHandler.Create)
					r.Post("/batch", cfg.TransformerHandler.BatchCreate)
					r.Put("/batch", cfg.TransformerHandler.BatchUpdate)
					r.Delete("/batch", cfg.TransformerHandler.BatchDelete)
					r.With(limit(rl.Import)).Post("/import", cfg.TransformerHandler.ImportCSV)
					r.Put("/{id}", cfg.TransformerHandler.Update)
					r.Patch("/{id}", cfg.TransformerHandler.Update)
					r.Put("/{id}/location", cfg.TransformerHandler.UpdateLocation)
					r.Put("/{id}/technical-data", cfg.TransformerHandler.UpdateTechnicalData)
					r.Put("/{id}/loss-limit", cfg.TransformerHandler.UpdateLossLimit)
					r.Put("/{id}/substation", cfg.TransformerHandler.UpdateSubstation)
					r.Put("/{id}/active", cfg.TransformerHandler.UpdateActive)
					r.Delete("/{id}", cfg.TransformerHandler.Delete)
				})
				r.With(perm(domain.PermissionReadingManage)).Post("/{id}/readings", cfg.NetworkHandler.AddTransformerReading)
				r.With(perm(domain.PermissionAlertManage)).Post("/{id}/alert-config", cfg.AlertHandler.CreateForTransformer)
			})

			// Unidades consumidoras
			r.Route("/consuming-units", func(r chi.Router) {
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionUCView))
					r.Get("/", cfg.ConsumingUnitHandler.List)
					r.Get("/transformer/{transformer_id}", cfg.ConsumingUnitHandler.ListByTransformer)
					r.Get("/{id}", cfg.ConsumingUnitHandler.GetByID)
					r.Get("/{id}/consumption", cfg.ConsumingUnitHandler.Consumption)
				})
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionUCManage))
					r.Post("/", cfg.ConsumingUnitHandler.Create)
					r.With(limit(rl.Import)).Post("/import", cfg.ConsumingUnitHandler.Import)
					r.Put("/{id}", cfg.ConsumingUnitHandler.Update)
					r.Patch("/{id}", cfg.ConsumingUnitHandler.Update)
					r.Delete("/{id}", cfg.ConsumingUnitHandler.Delete)
				})
				r.With(perm(domain.PermissionReadingManage)).Post("/{id}/readings", cfg.NetworkHandler.AddUCReading)
			})

			// Balanco energetico (PRODIST M7)
			r.Route("/balance", func(r chi.Router) {
				r.With(perm(domain.PermissionBalanceCalculate), limit(rl.Balance)).
					Post("/transformer/{transformer_id}/calculate", cfg.BalanceHandler.Calculate)
				r.With(perm(domain.PermissionBalanceCalculate), limit(rl.Balance)).
					Post("/calculate-all", cfg.BalanceHandler.CalculateAll)
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionReadingView))
					r.Get("/transformer/{transformer_id}", cfg.BalanceHandler.ListByTransformer)
					r.Get("/transformer/{transformer_id}/latest", cfg.BalanceHandler.Latest)
					r.Get("/transformer/{transformer_id}/technical-loss", cfg.BalanceHandler.TechnicalLoss)
				})
			})

			// Importacoes
			r.Route("/imports", func(r chi.Router) {
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionReadingView))
					r.Get("/", cfg.ImportHandler.ListImports)
					r.Get("/history", cfg.ImportHandler.GetImportHistory)
					r.Get("/summary", cfg.ImportHandler.GetImportSummary)
					r.Get("/template", cfg.ImportHandler.GetUploadTemplate)
					r.Get("/schemas", cfg.ImportHandler.GetSchemas)
					r.Get("/{id}", cfg.ImportHandler.GetImport)
					r.Get("/{id}/status", cfg.ImportHandler.GetImportStatus)
					r.Get("/{id}/errors", cfg.ImportHandler.DownloadErrorReport)
					r.Get("/{id}/logs", cfg.ImportHandler.GetImportLogs)
				})
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionImportManage), limit(rl.Import))
					r.Post("/transformers", cfg.ImportHandler.UploadTransformerReadings)
					r.Post("/ucs", cfg.ImportHandler.UploadUCReadings)
					r.Post("/batch", cfg.ImportHandler.BatchUpload)
					r.Post("/validate", cfg.ImportHandler.ValidateCSV)
					r.Post("/{id}/retry", cfg.ImportHandler.RetryImport)
					r.Delete("/{id}", cfg.ImportHandler.DeleteImport)
				})
			})

			// Alertas: destinatarios, grupos e historico (RF05)
			r.Route("/alert-recipients", func(r chi.Router) {
				r.With(perm(domain.PermissionDashboardView)).Get("/", cfg.AlertHandler.ListRecipients)
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionAlertManage))
					r.Post("/", cfg.AlertHandler.CreateRecipient)
					r.Put("/{id}", cfg.AlertHandler.UpdateRecipient)
					r.Delete("/{id}", cfg.AlertHandler.DeleteRecipient)
					r.Post("/{id}/test", cfg.AlertHandler.TestRecipient)
				})
			})
			r.Route("/transformer-groups", func(r chi.Router) {
				r.With(perm(domain.PermissionTransformerView)).Get("/", cfg.AlertHandler.ListGroups)
				r.Group(func(r chi.Router) {
					r.Use(perm(domain.PermissionAlertManage))
					r.Post("/", cfg.AlertHandler.CreateGroup)
					r.Put("/{id}", cfg.AlertHandler.UpdateGroup)
					r.Delete("/{id}", cfg.AlertHandler.DeleteGroup)
				})
			})
			r.Route("/alerts", func(r chi.Router) {
				r.Use(perm(domain.PermissionDashboardView))
				r.Get("/", cfg.AlertHandler.ListHistory)
				r.Get("/stats", cfg.AlertHandler.Stats)
				r.Get("/transformer/{transformer_id}", cfg.AlertHandler.RecipientsByTransformer)
				r.Get("/{id}", cfg.AlertHandler.GetByID)
			})

			// Risco
			r.Route("/risk", func(r chi.Router) {
				r.Use(perm(domain.PermissionDashboardView))
				r.Get("/transformer/{transformer_id}/score", cfg.RiskHandler.GetRiskScore)
				r.Get("/transformer/{transformer_id}/anomalies", cfg.RiskHandler.GetTransformerAnomalies)
				r.Get("/all-scores", cfg.RiskHandler.GetAllRiskScores)
				r.Get("/all-anomalies", cfg.RiskHandler.GetAllTransformersAnomalies)
			})

			// Dashboards e mapa
			r.Route("/dashboard", func(r chi.Router) {
				r.Use(perm(domain.PermissionDashboardView))
				r.Get("/kpis", cfg.DashboardHandler.GetKPIs)
				r.Get("/monthly-loss", cfg.DashboardHandler.GetMonthlyLossHistory)
				r.Get("/transformer-current-status", cfg.DashboardHandler.GetTransformerCurrentStatus)
				r.Get("/kpi-sparklines", cfg.DashboardHandler.GetKPISparklines)
				r.Get("/loss-evolution", cfg.DashboardHandler.GetLossEvolution)
				r.Get("/loss-composition", cfg.DashboardHandler.GetLossComposition)
				r.Get("/status-distribution", cfg.DashboardHandler.GetTransformerStatusDistribution)
				r.Get("/quarterly-injected-billed", cfg.DashboardHandler.GetQuarterlyInjectedBilled)
				r.Get("/top-transformers", cfg.DashboardHandler.GetTopTransformersByLoss)
				r.Get("/map", cfg.DashboardHandler.GetMap)
				r.Get("/operational", cfg.DashboardHandler.GetOperational)
				r.Group(func(r chi.Router) {
					r.Use(mw.RequireFinancialAccess)
					r.Get("/financial-table", cfg.DashboardHandler.GetFinancialTable)
					r.Get("/accumulated-loss-cost", cfg.DashboardHandler.GetAccumulatedLossCost)
					r.Get("/managerial", cfg.DashboardHandler.GetManagerial)
				})
			})

			// Relatorios e exportacoes (RF06)
			r.Group(func(r chi.Router) {
				r.Use(perm(domain.PermissionReportExport))
				r.Get("/exports/balance/{transformer_id}", cfg.ExportHandler.ExportBalance)
				r.Get("/exports/readings", cfg.ExportHandler.ExportReadings)
				r.With(mw.RequireFinancialAccess).Get("/exports/aneel", cfg.ExportHandler.ExportANEEL)
				r.Get("/reports/comparative", cfg.ExportHandler.Comparative)
			})

			// Assinatura (o TenantStatusMiddleware libera /billing mesmo
			// com o tenant bloqueado, para permitir a regularizacao)
			r.Route("/billing", func(r chi.Router) {
				r.Use(perm(domain.PermissionBillingManage))
				r.Get("/", cfg.BillingHandler.GetBillingInfo)
				r.Post("/activate", cfg.BillingHandler.ActivatePlan)
				r.Patch("/plan", cfg.BillingHandler.ChangePlan)
				r.Post("/cancel", cfg.BillingHandler.CancelSubscription)
				r.Get("/payments", cfg.BillingHandler.GetPaymentHistory)
				r.Get("/payments/{id}/invoice", cfg.BillingHandler.GetInvoice)
				r.Get("/payments/{id}/boleto", cfg.BillingHandler.GetBoleto)
				r.Get("/payments/{id}/pix-qrcode", cfg.BillingHandler.GetPixQRCode)
				r.Get("/proration", cfg.BillingHandler.GetProrationHistory)
				r.Get("/dunning", cfg.BillingHandler.GetDunningHistory)
			})
		})

		// Painel master (SUPER_ADMIN) - fora do TenantStatusMiddleware
		r.Route("/admin", func(r chi.Router) {
			r.Use(cfg.AuthMiddleware.Chain())
			r.Use(cfg.AuthMiddleware.RoleMiddleware(domain.UserRoleSuperAdmin))
			r.Get("/tenants", cfg.SuperAdminHandler.ListTenants)
			r.Get("/tenants/{id}", cfg.SuperAdminHandler.GetTenantByID)
			r.Get("/tenants/{id}/users", cfg.SuperAdminHandler.ListTenantUsers)
			r.Post("/tenants/{id}/activate", cfg.SuperAdminHandler.ActivateTenant)
			r.Patch("/tenants/{id}/plan", cfg.SuperAdminHandler.UpdateTenantPlan)
			r.Patch("/tenants/{id}/status", cfg.SuperAdminHandler.UpdateTenantStatus)
			r.Post("/tenants/{id}/extend-trial", cfg.SuperAdminHandler.ExtendTrial)
			r.Get("/billing/metrics", cfg.SuperAdminHandler.BillingMetrics)
			r.Get("/audit", cfg.SuperAdminHandler.AuditLog)
		})
	})

	return r
}
