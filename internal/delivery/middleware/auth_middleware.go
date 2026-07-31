package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/jwt"
	"github.com/voltiq/server/internal/repository"
)

type contextKey string

const (
	UserIDKey   contextKey = "user_id"
	TenantIDKey contextKey = "tenant_id"
	TenantKey   contextKey = "tenant"
	EmailKey    contextKey = "email"
	RoleKey     contextKey = "role"
)

// AuthMiddleware handles JWT authentication
type AuthMiddleware struct {
	jwtService *jwt.Service
	db         *repository.Database
}

// NewAuthMiddleware creates a new AuthMiddleware
func NewAuthMiddleware(jwtService *jwt.Service, db *repository.Database) *AuthMiddleware {
	return &AuthMiddleware{
		jwtService: jwtService,
		db:         db,
	}
}

// Handler wraps a http.Handler with authentication
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			request.WriteJSON(w, http.StatusUnauthorized, request.Fail("UNAUTHORIZED", "missing authorization header", nil))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			request.WriteJSON(w, http.StatusUnauthorized, request.Fail("UNAUTHORIZED", "invalid authorization format", nil))
			return
		}

		tokenString := parts[1]
		claims, err := m.jwtService.ValidateToken(tokenString)
		if err != nil {
			request.WriteJSON(w, http.StatusUnauthorized, request.Fail("UNAUTHORIZED", "invalid or expired token", nil))
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIDKey, domain.UUID(claims.UserID))
		ctx = context.WithValue(ctx, TenantIDKey, domain.UUID(claims.TenantID))
		ctx = context.WithValue(ctx, EmailKey, claims.Email)
		ctx = context.WithValue(ctx, RoleKey, string(claims.Role))

		// Set tenant ID for RLS policies using the db instance from middleware
		if m.db != nil {
			ctx = m.db.SetTenantID(ctx, claims.TenantID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RoleMiddleware creates a middleware that checks for required roles
func (m *AuthMiddleware) RoleMiddleware(allowedRoles ...domain.UserRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := r.Context().Value(RoleKey).(string)
			if !ok {
				request.WriteJSON(w, http.StatusForbidden, request.Fail("FORBIDDEN", "role not found in context", nil))
				return
			}

			allowed := false
			for _, allowedRole := range allowedRoles {
				if role == string(allowedRole) {
					allowed = true
					break
				}
			}

			if !allowed {
				request.WriteJSON(w, http.StatusForbidden, request.Fail("FORBIDDEN", "insufficient permissions", nil))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) domain.UUID {
	if userID, ok := ctx.Value(UserIDKey).(domain.UUID); ok {
		return userID
	}
	return ""
}

// GetTenantID extracts tenant ID from context
func GetTenantID(ctx context.Context) domain.UUID {
	if tenantID, ok := ctx.Value(TenantIDKey).(domain.UUID); ok {
		return tenantID
	}
	return ""
}

// GetRole extracts role from context
func GetRole(ctx context.Context) string {
	if role, ok := ctx.Value(RoleKey).(string); ok {
		return role
	}
	return ""
}

// Chain creates a chi middleware chain
func (m *AuthMiddleware) Chain() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return m.Handler(next)
	}
}

// GetTenant extracts tenant from context
func GetTenant(ctx context.Context) *domain.Tenant {
	if tenant, ok := ctx.Value(TenantKey).(*domain.Tenant); ok {
		return tenant
	}
	return nil
}

// GetUserRole extracts user role from context
func GetUserRole(ctx context.Context) string {
	if role, ok := ctx.Value(RoleKey).(string); ok {
		return role
	}
	return ""
}
