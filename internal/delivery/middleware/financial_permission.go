package middleware

import (
	"net/http"

	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
)

// RequireFinancialAccess bloqueia papeis sem financial:view (ENGINEER/VIEWER)
// em endpoints financeiros.
func RequireFinancialAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !CanViewFinancialData(domain.UserRole(GetUserRole(r.Context()))) {
			request.WriteJSON(w, http.StatusForbidden,
				request.Fail("FORBIDDEN", "Acesso a dados financeiros restrito", nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}
