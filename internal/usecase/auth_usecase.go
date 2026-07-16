package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/jwt"
	"github.com/voltiq/server/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrTokenInvalid       = errors.New("invalid or expired token")
)

// AuthUseCase handles authentication business logic
type AuthUseCase struct {
	userRepo   *repository.UserRepository
	tenantRepo *repository.TenantRepository
	jwtService *jwt.Service
}

// LoginInput contains login request data
type LoginInput struct {
	Email    string
	Password string
}

// LoginOutput contains login response data
type LoginOutput struct {
	Token     string
	ExpiresAt time.Time
	User      *domain.User
}

// RefreshTokenInput contains refresh token request data
type RefreshTokenInput struct {
	RefreshToken string
}

// RefreshTokenOutput contains new tokens
type RefreshTokenOutput struct {
	Token         string
	ExpiresAt     time.Time
	RefreshToken  string
	RefreshExpiry time.Time
}

// LogoutInput contains logout request data
type LogoutInput struct {
	UserID domain.UUID
}

// NewAuthUseCase creates a new AuthUseCase
func NewAuthUseCase(
	userRepo *repository.UserRepository,
	tenantRepo *repository.TenantRepository,
	jwtService *jwt.Service,
) *AuthUseCase {
	return &AuthUseCase{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		jwtService: jwtService,
	}
}

// Login authenticates a user and returns a JWT token
func (uc *AuthUseCase) Login(ctx context.Context, input LoginInput) (*LoginOutput, error) {
	user, err := uc.userRepo.GetByEmail(ctx, input.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if !user.Active {
		return nil, errors.New("user is inactive")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	now := time.Now()
	user.LastLogin = &now
	if err := uc.userRepo.Update(ctx, user); err != nil {
		return nil, errors.New("failed to update user login")
	}

	token, err := uc.jwtService.GenerateToken(
		string(user.ID),
		string(user.TenantID),
		user.Email,
		string(user.Role),
	)
	if err != nil {
		return nil, errors.New("failed to generate token")
	}

	return &LoginOutput{
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User:      user,
	}, nil
}

// RefreshToken generates a new access token using a refresh token
func (uc *AuthUseCase) RefreshToken(ctx context.Context, input RefreshTokenInput) (*RefreshTokenOutput, error) {
	claims, err := uc.jwtService.ValidateRefreshToken(input.RefreshToken)
	if err != nil {
		return nil, ErrTokenInvalid
	}

	user, err := uc.userRepo.GetByID(ctx, domain.UUID(claims.UserID))
	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}

	if !user.Active {
		return nil, errors.New("user is inactive")
	}

	token, err := uc.jwtService.GenerateToken(
		string(user.ID),
		string(user.TenantID),
		user.Email,
		string(user.Role),
	)
	if err != nil {
		return nil, errors.New("failed to generate token")
	}
	expiresAt := time.Now().Add(24 * time.Hour)

	newRefreshToken, err := uc.jwtService.GenerateRefreshToken(
		string(user.ID),
		string(user.TenantID),
	)
	if err != nil {
		return nil, errors.New("failed to generate refresh token")
	}
	refreshExpiry := time.Now().Add(7 * 24 * time.Hour)

	return &RefreshTokenOutput{
		Token:         token,
		ExpiresAt:     expiresAt,
		RefreshToken:  newRefreshToken,
		RefreshExpiry: refreshExpiry,
	}, nil
}

// Logout invalidates tokens (in a real system, would add to blacklist)
func (uc *AuthUseCase) Logout(ctx context.Context, input LogoutInput) error {
	// Em produção, adicionar o token a uma blacklist no Redis
	// Por enquanto, apenas logamos o logout
	return nil
}

// GetJWTService returns the JWT service for token operations
func (uc *AuthUseCase) GetJWTService() *jwt.Service {
	return uc.jwtService
}
