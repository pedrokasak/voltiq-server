package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/jwt"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrTokenInvalid       = errors.New("invalid or expired token")
	ErrUserInactive       = errors.New("user is inactive")
)

var (
	dummyOnce sync.Once
	dummy     []byte
)

// dummyHash is compared on unknown e-mails so the response time does not
// reveal whether an account exists.
func dummyHash() []byte {
	dummyOnce.Do(func() { dummy, _ = bcrypt.GenerateFromPassword([]byte("voltiq-timing-guard"), 12) })
	return dummy
}

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
	Token         string
	ExpiresAt     time.Time
	RefreshToken  string
	RefreshExpiry time.Time
	User          *domain.User
}

// RefreshTokenInput contains refresh token request data
type RefreshTokenInput struct {
	RefreshToken string
}

// RefreshTokenOutput contains new tokens and the user they belong to
type RefreshTokenOutput struct {
	Token         string
	ExpiresAt     time.Time
	RefreshToken  string
	RefreshExpiry time.Time
	User          *domain.User
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

// issue generates the access and refresh tokens of a user.
func (uc *AuthUseCase) issue(user *domain.User) (*LoginOutput, error) {
	token, err := uc.jwtService.GenerateToken(string(user.ID), string(user.TenantID), user.Email, string(user.Role))
	if err != nil {
		return nil, errors.New("failed to generate token")
	}
	refresh, err := uc.jwtService.GenerateRefreshToken(string(user.ID), string(user.TenantID))
	if err != nil {
		return nil, errors.New("failed to generate refresh token")
	}
	now := time.Now()
	return &LoginOutput{
		Token:         token,
		ExpiresAt:     now.Add(uc.jwtService.Expiration()),
		RefreshToken:  refresh,
		RefreshExpiry: now.Add(uc.jwtService.RefreshExpiration()),
		User:          user,
	}, nil
}

// Login authenticates a user and returns access and refresh tokens
func (uc *AuthUseCase) Login(ctx context.Context, input LoginInput) (*LoginOutput, error) {
	user, err := uc.userRepo.GetByEmail(ctx, strings.TrimSpace(input.Email))
	if err != nil || user == nil {
		// Compara com um hash fixo para nao revelar, pelo tempo de resposta,
		// se o e-mail existe.
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(input.Password))
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.Active {
		return nil, ErrUserInactive
	}

	now := time.Now()
	user.LastLogin = &now
	if err := uc.userRepo.Update(ctx, user); err != nil {
		return nil, errors.New("failed to update user login")
	}

	return uc.issue(user)
}

// IssueFor returns tokens for a user just created (signup, invite accept).
func (uc *AuthUseCase) IssueFor(user *domain.User) (*LoginOutput, error) {
	return uc.issue(user)
}

// RefreshToken generates a new access token using a refresh token and
// rotates the refresh token.
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
		return nil, ErrUserInactive
	}

	out, err := uc.issue(user)
	if err != nil {
		return nil, err
	}
	return &RefreshTokenOutput{
		Token:         out.Token,
		ExpiresAt:     out.ExpiresAt,
		RefreshToken:  out.RefreshToken,
		RefreshExpiry: out.RefreshExpiry,
		User:          user,
	}, nil
}

// Logout ends the session. Tokens are stateless: the handler clears the
// refresh cookie and the access token expires on its own.
func (uc *AuthUseCase) Logout(ctx context.Context, input LogoutInput) error {
	return nil
}

// ChangePassword changes the password of the authenticated user.
func (uc *AuthUseCase) ChangePassword(ctx context.Context, userID domain.UUID, current, next string) error {
	user, err := uc.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return ErrUserNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)); err != nil {
		return &domain.ValidationError{Fields: []domain.FieldError{{Field: "current_password", Message: "senha atual incorreta"}}}
	}
	if err := ValidatePassword(next); err != nil {
		return &domain.ValidationError{Fields: []domain.FieldError{{Field: "new_password", Message: err.Error()}}}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), 12)
	if err != nil {
		return errors.New("failed to hash password")
	}
	user.PasswordHash = string(hash)
	return uc.userRepo.Update(ctx, user)
}

// GetJWTService returns the JWT service for token operations
func (uc *AuthUseCase) GetJWTService() *jwt.Service {
	return uc.jwtService
}
