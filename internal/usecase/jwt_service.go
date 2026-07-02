package usecase

import (
	"errors"
	"time"
)

// Claims represents JWT token claims
type Claims struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// JWTService defines the interface for JWT operations
type JWTService interface {
	GenerateToken(userID, tenantID, email, role string) (string, time.Time, error)
	ValidateToken(tokenString string) (*Claims, error)
	GenerateRefreshToken(userID, tenantID string) (string, time.Time, error)
	ValidateRefreshToken(tokenString string) (*Claims, error)
}

var ErrInvalidToken = errors.New("invalid or expired token")
