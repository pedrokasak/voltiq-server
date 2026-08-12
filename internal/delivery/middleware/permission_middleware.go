package middleware

import (
	"net/http"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/delivery/request"
)

// RequirePermission middleware verifica se o usuário tem uma permissão específica
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := GetUserRole(r.Context())
			if role == "" {
				request.WriteJSON(w, http.StatusUnauthorized,
					request.Fail("UNAUTHORIZED", "user not authenticated", nil))
				return
			}

			hasPermission := false
			for _, p := range domain.RolePermissions[domain.UserRole(role)] {
				if domain.Permission(permission) == p {
					hasPermission = true
					break
				}
			}

			if !hasPermission {
				request.WriteJSON(w, http.StatusForbidden,
					request.Fail("FORBIDDEN", "access denied: permission required", nil))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission middleware verifica se o usuário tem pelo menos uma das permissões
func RequireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := GetUserRole(r.Context())
			if role == "" {
				request.WriteJSON(w, http.StatusUnauthorized,
					request.Fail("UNAUTHORIZED", "user not authenticated", nil))
				return
			}

			hasPermission := false
			for _, p := range permissions {
				if domain.HasPermission(domain.UserRole(role), domain.Permission(p)) {
					hasPermission = true
					break
				}
			}

			if !hasPermission {
				request.WriteJSON(w, http.StatusForbidden,
					request.Fail("FORBIDDEN", "access denied: insufficient permissions", nil))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole middleware verifica se o usuário tem um dos roles permitidos
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := GetUserRole(r.Context())
			if role == "" {
				request.WriteJSON(w, http.StatusUnauthorized,
					request.Fail("UNAUTHORIZED", "user not authenticated", nil))
				return
			}

			hasRole := false
			for _, r := range roles {
				if role == r {
					hasRole = true
					break
				}
			}

			if !hasRole {
				request.WriteJSON(w, http.StatusForbidden,
					request.Fail("FORBIDDEN", "access denied: insufficient role", nil))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireSuperAdmin middleware permite apenas SUPER_ADMIN
func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := GetUserRole(r.Context())
		if role != "SUPER_ADMIN" {
			request.WriteJSON(w, http.StatusForbidden,
				request.Fail("FORBIDDEN", "super admin access required", nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdminOrAbove middleware permite apenas SUPER_ADMIN, OWNER, ADMIN
func RequireAdminOrAbove(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := GetUserRole(r.Context())
		allowedRoles := map[string]bool{
			"SUPER_ADMIN": true,
			"OWNER":       true,
			"ADMIN":       true,
		}
		if !allowedRoles[role] {
			request.WriteJSON(w, http.StatusForbidden,
				request.Fail("FORBIDDEN", "admin access required", nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireManagerOrAbove middleware permite apenas SUPER_ADMIN, OWNER, ADMIN, MANAGER
func RequireManagerOrAbove(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := GetUserRole(r.Context())
		allowedRoles := map[string]bool{
			"SUPER_ADMIN": true,
			"OWNER":       true,
			"ADMIN":       true,
			"MANAGER":     true,
		}
		if !allowedRoles[role] {
			request.WriteJSON(w, http.StatusForbidden,
				request.Fail("FORBIDDEN", "manager access required", nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}