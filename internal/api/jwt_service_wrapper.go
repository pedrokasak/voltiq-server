package api

import (
	"time"

	"github.com/energybalance/server/internal/usecase"
)

// JWTServiceWrapper wraps the existing JWTService to implement the usecase.JWTService interface
type JWTServiceWrapper struct {
	service *JWTService
}

// NewJWTServiceWrapper creates a new JWTServiceWrapper
func NewJWTServiceWrapper(service *JWTService) *JWTServiceWrapper {
	return &JWTServiceWrapper{
		service: service,
	}
}

// GenerateToken generates an access token
func (w *JWTServiceWrapper) GenerateToken(userID, tenantID, email, role string) (string, time.Time, error) {
	token, err := w.service.GenerateToken(userID, tenantID, email, role)
	if err != nil {
		return "", time.Time{}, err
	}
	
	expiresAt := time.Now().Add(24 * time.Hour)
	return token, expiresAt, nil
}

// ValidateToken validates an access token
func (w *JWTServiceWrapper) ValidateToken(tokenString string) (*usecase.Claims, error) {
	claims, err := w.service.ValidateToken(tokenString)
	if err != nil {
		return nil, usecase.ErrInvalidToken
	}

	return &usecase.Claims{
		UserID:   claims.UserID,
		TenantID: claims.TenantID,
		Email:    claims.Email,
		Role:     claims.Role,
	}, nil
}

// GenerateRefreshToken generates a refresh token
func (w *JWTServiceWrapper) GenerateRefreshToken(userID, tenantID string) (string, time.Time, error) {
	token, err := w.service.GenerateRefreshToken(userID, tenantID)
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	return token, expiresAt, nil
}

// ValidateRefreshToken validates a refresh token
func (w *JWTServiceWrapper) ValidateRefreshToken(tokenString string) (*usecase.Claims, error) {
	claims, err := w.service.ValidateRefreshToken(tokenString)
	if err != nil {
		return nil, usecase.ErrInvalidToken
	}

	return &usecase.Claims{
		UserID:   claims.UserID,
		TenantID: claims.TenantID,
		Email:    claims.Email,
		Role:     claims.Role,
	}, nil
}
