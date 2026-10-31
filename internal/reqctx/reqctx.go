// Package reqctx carries the authenticated identity of a request through
// context.Context so that lower layers (repositories, the connection pool)
// can scope queries to the caller's tenant without importing HTTP packages.
package reqctx

import (
	"context"

	"github.com/voltiq/server/internal/domain"
)

type key int

const identityKey key = iota

// Identity is the authenticated principal of a request.
type Identity struct {
	UserID   domain.UUID
	TenantID domain.UUID
	Email    string
	Role     domain.UserRole
}

// WithIdentity returns a copy of ctx carrying id.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// FromContext returns the identity stored in ctx, if any.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

// TenantID returns the tenant of the request, or "" for unauthenticated or
// system contexts (signup, login, webhooks, background jobs).
func TenantID(ctx context.Context) domain.UUID {
	id, _ := FromContext(ctx)
	return id.TenantID
}

// Email returns the e-mail of the authenticated user, or "".
func Email(ctx context.Context) string {
	id, _ := FromContext(ctx)
	return id.Email
}

// UserID returns the authenticated user ID, or "".
func UserID(ctx context.Context) domain.UUID {
	id, _ := FromContext(ctx)
	return id.UserID
}

// Role returns the authenticated user role, or "".
func Role(ctx context.Context) domain.UserRole {
	id, _ := FromContext(ctx)
	return id.Role
}

// IsSuperAdmin reports whether the request comes from a platform super admin.
func IsSuperAdmin(ctx context.Context) bool {
	return Role(ctx) == domain.UserRoleSuperAdmin
}
