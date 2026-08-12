package middleware

import (
	"net/http"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/delivery/request"
)

// RequireFinancialAccess middleware bloqueia ENGINEER/VIEWER em endpoints financeiros
func RequireFinancialAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := GetUserRole(r.Context())

		// Roles com acesso financeiro
		allowedRoles := map[domain.UserRole]bool{
			domain.UserRoleSuperAdmin: true,
			domain.UserRoleOwner:      true,
			domain.UserRoleAdmin:      true,
			domain.UserRoleManager:    true,
		}

		if !allowedRoles[domain.UserRole(role)] {
			request.WriteJSON(w, http.StatusForbidden,
				request.Fail("FORBIDDEN", "Acesso a dados financeiros restrito", nil))
			return
		}

		next.ServeHTTP(w, r)
	})
}