package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// TenantStatusMiddleware blocks access for tenants with suspended/pending
// payment/cancelled status or expired trial. Billing, session and profile
// routes stay open so the customer can regularise the subscription.
type TenantStatusMiddleware struct {
	tenantRepo *repository.TenantRepository
}

// NewTenantStatusMiddleware creates a new TenantStatusMiddleware
func NewTenantStatusMiddleware(tenantRepo *repository.TenantRepository) *TenantStatusMiddleware {
	return &TenantStatusMiddleware{tenantRepo: tenantRepo}
}

// Handler checks tenant status and blocks access if needed
func (m *TenantStatusMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) || GetUserRole(r.Context()) == string(domain.UserRoleSuperAdmin) {
			next.ServeHTTP(w, r)
			return
		}
		tenantID := GetTenantID(r.Context())
		if tenantID == "" {
			next.ServeHTTP(w, r)
			return
		}

		tenant := GetTenant(r.Context())
		if tenant == nil {
			t, err := m.tenantRepo.GetByID(r.Context(), tenantID)
			if err != nil || t == nil {
				request.WriteJSON(w, http.StatusForbidden,
					request.Fail("TENANT_NOT_FOUND", "tenant não encontrado", nil))
				return
			}
			tenant = t
		}
		ctx := context.WithValue(r.Context(), TenantKey, tenant)
		r = r.WithContext(ctx)

		// Rotas de cobranca e sessao continuam acessiveis para regularizar
		if isBillingAllowedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		switch tenant.Status {
		case domain.TenantStatusSuspended:
			request.WriteJSON(w, http.StatusPaymentRequired,
				request.Fail("TENANT_SUSPENDED",
					"Conta suspensa por inadimplência. Regularize o pagamento em Assinatura.",
					map[string]any{"status": tenant.Status}))
			return
		case domain.TenantStatusPendingPayment:
			request.WriteJSON(w, http.StatusPaymentRequired,
				request.Fail("PENDING_PAYMENT",
					"Pagamento pendente. Complete o pagamento para acessar.",
					map[string]any{"status": tenant.Status}))
			return
		case domain.TenantStatusCancelled:
			request.WriteJSON(w, http.StatusForbidden,
				request.Fail("TENANT_CANCELLED",
					"Conta cancelada. Contate o suporte.",
					map[string]any{"status": tenant.Status}))
			return
		case domain.TenantStatusTrial:
			expires := tenant.TrialExpiresAt
			if expires == nil && !tenant.TrialUntil.IsZero() {
				expires = &tenant.TrialUntil
			}
			if expires != nil && time.Now().After(*expires) {
				request.WriteJSON(w, http.StatusPaymentRequired,
					request.Fail("TRIAL_EXPIRED",
						"Período de avaliação expirado. Escolha um plano para continuar.",
						map[string]any{"trial_expires_at": expires}))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func hasPathPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")+"/")
}

// isPublicPath verifica se o path é público (não precisa de tenant status check)
func isPublicPath(path string) bool {
	for _, p := range []string{
		"/health", "/ready", "/metrics",
		"/api/v1/auth/login", "/api/v1/auth/signup", "/api/v1/auth/refresh", "/api/v1/auth/logout",
		"/api/v1/invites/validate", "/api/v1/webhooks",
	} {
		if hasPathPrefix(path, p) {
			return true
		}
	}
	return false
}

// isBillingAllowedPath lista as rotas liberadas para tenants bloqueados:
// cobranca, perfil/sessao e configuracoes basicas.
func isBillingAllowedPath(path string) bool {
	for _, p := range []string{
		"/api/v1/billing",
		"/api/v1/auth/me",
		"/api/v1/auth/change-password",
		"/api/v1/tenants/current",
		"/api/v1/tenant/settings",
	} {
		if hasPathPrefix(path, p) {
			return true
		}
	}
	return false
}
