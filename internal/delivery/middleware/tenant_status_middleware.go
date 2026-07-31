package middleware

import (
	"net/http"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/delivery/request"
)

// TenantStatusMiddleware blocks access for tenants with suspended/pending payment status
type TenantStatusMiddleware struct {
	tenantRepo *repository.TenantRepository
}

// NewTenantStatusMiddleware creates a new TenantStatusMiddleware
func NewTenantStatusMiddleware(tenantRepo *repository.TenantRepository) *TenantStatusMiddleware {
	return &TenantStatusMiddleware{
		tenantRepo: tenantRepo,
	}
}

// Handler checks tenant status and blocks access if needed
func (m *TenantStatusMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip for public paths
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		tenantID := GetTenantID(r.Context())
		if tenantID == "" {
			// No tenant in context - allow (public routes or SUPER_ADMIN)
			next.ServeHTTP(w, r)
			return
		}

		// Get tenant from context (set by AuthMiddleware)
		tenant := GetTenant(r.Context())
		if tenant == nil {
			// Try to fetch from repo if not in context
			ctx := r.Context()
			tenant, err := m.tenantRepo.GetByID(ctx, domain.UUID(tenantID))
			if err != nil || tenant == nil {
				request.WriteJSON(w, http.StatusForbidden,
					request.Fail("TENANT_NOT_FOUND", "tenant not found", nil))
				return
			}
		}

		// Check tenant status
		switch tenant.Status {
		case domain.TenantStatusSuspended:
			request.WriteJSON(w, http.StatusPaymentRequired,
				request.Fail("TENANT_SUSPENDED",
					"Tenant suspenso por inadimplência. Contate o administrador para reativar.",
					map[string]any{"status": tenant.Status}))
			return

		case domain.TenantStatusPendingPayment:
			// Permitir apenas endpoints de billing/ativação
			if !isBillingAllowedPath(r.URL.Path) {
				request.WriteJSON(w, http.StatusPaymentRequired,
					request.Fail("PENDING_PAYMENT",
						"Pagamento pendente. Complete o pagamento para acessar.",
						map[string]any{"status": tenant.Status}))
				return
			}

		case domain.TenantStatusCancelled:
			request.WriteJSON(w, http.StatusForbidden,
				request.Fail("TENANT_CANCELLED",
					"Conta cancelada. Contate o suporte.",
					map[string]any{"status": tenant.Status}))
			return
		}

		// Verificar trial expirado
		if tenant.Status == domain.TenantStatusTrial && tenant.TrialExpiresAt != nil {
			if time.Now().After(*tenant.TrialExpiresAt) {
				request.WriteJSON(w, http.StatusPaymentRequired,
					request.Fail("TRIAL_EXPIRED",
						"Período de trial expirado. Complete o pagamento para continuar.",
						map[string]any{"trial_expires_at": tenant.TrialExpiresAt}))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// isPublicPath verifica se o path é público (não precisa de tenant status check)
func isPublicPath(path string) bool {
	publicPaths := []string{
		"/health",
		"/ready",
		"/metrics",
		"/api/v1/auth/login",
		"/api/v1/auth/signup",
		"/api/v1/auth/refresh",
		"/api/v1/auth/logout",
		"/api/v1/invites/",
	}

	for _, p := range publicPaths {
		if path == p || (len(p) > 1 && len(path) >= len(p) && path[:len(p)] == p) {
			return true
		}
	}
	return false
}

// isBillingAllowedPath verifica se o path é permitido para PENDING_PAYMENT
func isBillingAllowedPath(path string) bool {
	billingPaths := []string{
		"/api/v1/billing",
		"/api/v1/tenants/current",
		"/api/v1/admin/tenants",
	}

	for _, p := range billingPaths {
		if path == p || (len(p) > 1 && len(path) >= len(p) && path[:len(p)] == p) {
			return true
		}
	}
	return false
}