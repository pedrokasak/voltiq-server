package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/voltiq/server/internal/delivery/handler"
	deliverymiddleware "github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/pkg/metrics"
)

// Config holds router configuration
type Config struct {
	AuthHandler          *handler.AuthHandler
	InviteHandler        *handler.InviteHandler
	TransformerHandler   *handler.TransformerHandler
	ConsumingUnitHandler *handler.ConsumingUnitHandler
	BalanceHandler       *handler.BalanceHandler
	ImportHandler        *handler.ImportHandler
	AlertHandler         *handler.AlertHandler
	RiskHandler          *handler.RiskHandler
	DashboardHandler     *handler.DashboardHandler
	ExportHandler        *handler.ExportHandler
	SuperAdminHandler    *handler.SuperAdminHandler
	HealthHandler        *handler.HealthHandler
	MetricsCollector     *metrics.MetricsCollector
	AuthMiddleware       *deliverymiddleware.AuthMiddleware
	RateLimiter          *deliverymiddleware.RateLimiter
	SecurityMiddleware   *deliverymiddleware.SecurityMiddleware
	CORSOrigins          []string
}

// Setup creates and configures the Chi router
func Setup(cfg Config) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware - Order matters!
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60000)) // 60 seconds

	// Security middleware (must be early)
	if cfg.SecurityMiddleware != nil {
		r.Use(cfg.SecurityMiddleware.Handler)
	}

	// Content-Type security
	r.Use(deliverymiddleware.ContentTypeMiddleware)

	// Rate limiting
	if cfg.RateLimiter != nil {
		r.Use(cfg.RateLimiter.Middleware)
	}

	// CORS
	if len(cfg.CORSOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   cfg.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Request-ID"},
			ExposedHeaders:   []string{"X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	// Health check endpoints (no auth required)
	r.Get("/health", cfg.HealthHandler.Health)
	r.Get("/ready", cfg.HealthHandler.Ready)

	// Metrics endpoint (no auth required, but rate limited)
	r.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.MetricsCollector.ServeHTTP(w, r)
	}))

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public routes (no auth required)
		r.Group(func(r chi.Router) {
			r.Post("/auth/login", cfg.AuthHandler.Login)
			r.Post("/auth/signup", cfg.AuthHandler.Signup)
			r.Post("/auth/refresh", cfg.AuthHandler.RefreshToken)
			r.Post("/auth/logout", cfg.AuthHandler.Logout)
			r.Get("/invites/{token}", cfg.InviteHandler.ValidateInvite)
			r.Post("/invites/{token}/accept", cfg.InviteHandler.AcceptInvite)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(cfg.AuthMiddleware.Chain())

			// Invite routes (auth required)
			r.Post("/invites", cfg.InviteHandler.CreateInvite)
			r.Delete("/invites/{id}", cfg.InviteHandler.CancelInvite)

			// Transformer routes
			r.Route("/transformers", func(r chi.Router) {
				r.Post("/", cfg.TransformerHandler.Create)
				r.Post("/batch", cfg.TransformerHandler.BatchCreate)
				r.Put("/batch", cfg.TransformerHandler.BatchUpdate)
				r.Delete("/batch", cfg.TransformerHandler.BatchDelete)
				r.Get("/", cfg.TransformerHandler.List)
				r.Get("/count", cfg.TransformerHandler.Count)
				r.Get("/with-count", cfg.TransformerHandler.GetAllWithCount)
				r.Get("/paginated", cfg.TransformerHandler.GetWithPagination)
				r.Get("/{id}", cfg.TransformerHandler.GetByID)
				r.Get("/{id}/location", cfg.TransformerHandler.GetLocation)
				r.Get("/{id}/technical-data", cfg.TransformerHandler.GetTechnicalData)
				r.Get("/{id}/loss-limit", cfg.TransformerHandler.GetLossLimit)
				r.Get("/{id}/substation", cfg.TransformerHandler.GetSubstation)
				r.Get("/{id}/active", cfg.TransformerHandler.GetActive)
				r.Get("/code/{code}", cfg.TransformerHandler.GetByCode)
				r.Get("/code/{code}/exists", cfg.TransformerHandler.ExistsByCode)
				r.Get("/substation/{substation_id}", cfg.TransformerHandler.ListBySubstation)
				r.Put("/{id}", cfg.TransformerHandler.Update)
				r.Put("/{id}/location", cfg.TransformerHandler.UpdateLocation)
				r.Put("/{id}/technical-data", cfg.TransformerHandler.UpdateTechnicalData)
				r.Put("/{id}/loss-limit", cfg.TransformerHandler.UpdateLossLimit)
				r.Put("/{id}/substation", cfg.TransformerHandler.UpdateSubstation)
				r.Put("/{id}/active", cfg.TransformerHandler.UpdateActive)
				r.Delete("/{id}", cfg.TransformerHandler.Delete)
				r.Get("/{id}/stats", cfg.TransformerHandler.Stats)
				r.Get("/{id}/loss-analysis", cfg.TransformerHandler.LossAnalysis)
				r.Get("/{id}/export", cfg.TransformerHandler.ExportCSV)
				r.Post("/import", cfg.TransformerHandler.ImportCSV)
				r.Post("/{id}/alert-config", cfg.AlertHandler.CreateForTransformer)
			})

			// Export routes (balance artifacts: PDF/Excel)
			r.Route("/exports", func(r chi.Router) {
				r.Get("/balance/{transformer_id}", cfg.ExportHandler.ExportBalance)
			})

			// Admin routes (SUPER_ADMIN only)
			r.Route("/admin", func(r chi.Router) {
				r.Use(cfg.AuthMiddleware.RoleMiddleware(domain.UserRoleSuperAdmin))
				r.Get("/tenants", cfg.SuperAdminHandler.ListTenants)
				r.Get("/tenants/{id}", cfg.SuperAdminHandler.GetTenantByID)
				r.Get("/tenants/{id}/users", cfg.SuperAdminHandler.ListTenantUsers)
			})

			// Alert routes
			r.Route("/alerts", func(r chi.Router) {
				r.Post("/", cfg.AlertHandler.Create)
				r.Get("/", cfg.AlertHandler.ListByTenant)
				r.Get("/transformer/{transformer_id}", cfg.AlertHandler.GetByTransformer)
				r.Get("/{id}", cfg.AlertHandler.GetByID)
				r.Put("/{id}", cfg.AlertHandler.Update)
				r.Delete("/{id}", cfg.AlertHandler.Delete)
			})

			// Risk routes
			r.Route("/risk", func(r chi.Router) {
				r.Get("/transformer/{transformer_id}/score", cfg.RiskHandler.GetRiskScore)
				r.Get("/transformer/{transformer_id}/anomalies", cfg.RiskHandler.GetTransformerAnomalies)
				r.Get("/all-scores", cfg.RiskHandler.GetAllRiskScores)
				r.Get("/all-anomalies", cfg.RiskHandler.GetAllTransformersAnomalies)
			})

			// Consuming unit routes
			r.Route("/consuming-units", func(r chi.Router) {
				r.Post("/", cfg.ConsumingUnitHandler.Create)
				r.Get("/", cfg.ConsumingUnitHandler.List)
				r.Get("/{id}", cfg.ConsumingUnitHandler.GetByID)
				r.Get("/transformer/{transformer_id}", cfg.ConsumingUnitHandler.ListByTransformer)
				r.Put("/{id}", cfg.ConsumingUnitHandler.Update)
				r.Delete("/{id}", cfg.ConsumingUnitHandler.Delete)
			})

			// Balance routes
			r.Route("/balance", func(r chi.Router) {
				r.Post("/transformer/{transformer_id}/calculate", cfg.BalanceHandler.Calculate)
				r.Get("/transformer/{transformer_id}", cfg.BalanceHandler.ListByTransformer)
				r.Get("/transformer/{transformer_id}/latest", cfg.BalanceHandler.Latest)
				r.Get("/transformer/{transformer_id}/technical-loss", cfg.BalanceHandler.TechnicalLoss)
			})

			// Import routes
			r.Route("/imports", func(r chi.Router) {
				r.Post("/transformers", cfg.ImportHandler.UploadTransformerReadings)
				r.Post("/ucs", cfg.ImportHandler.UploadUCReadings)
				r.Get("/", cfg.ImportHandler.ListImports)
				r.Get("/history", cfg.ImportHandler.GetImportHistory)
				r.Get("/summary", cfg.ImportHandler.GetImportSummary)
				r.Get("/{id}", cfg.ImportHandler.GetImport)
				r.Get("/{id}/status", cfg.ImportHandler.GetImportStatus)
				r.Get("/{id}/errors", cfg.ImportHandler.DownloadErrorReport)
				r.Get("/{id}/logs", cfg.ImportHandler.GetImportLogs)
				r.Post("/{id}/retry", cfg.ImportHandler.RetryImport)
				r.Delete("/{id}", cfg.ImportHandler.DeleteImport)
				r.Post("/batch", cfg.ImportHandler.BatchUpload)
				r.Get("/template", cfg.ImportHandler.GetUploadTemplate)
				r.Post("/validate", cfg.ImportHandler.ValidateCSV)
			})

			// Dashboard routes
			r.Route("/dashboard", func(r chi.Router) {
				r.Get("/kpis", cfg.DashboardHandler.GetKPIs)
				r.Get("/monthly-loss", cfg.DashboardHandler.GetMonthlyLossHistory)
				r.Get("/transformer-current-status", cfg.DashboardHandler.GetTransformerCurrentStatus)
			})
		})
	})

	return r
}
