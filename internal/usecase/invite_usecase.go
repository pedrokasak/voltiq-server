package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInviteNotFound    = errors.New("invite not found")
	ErrInviteExpired     = errors.New("invite has expired")
	ErrInviteAlreadyUsed = errors.New("invite already accepted")
	ErrInviteCancelled   = errors.New("invite was cancelled")
	ErrEmailAlreadyInUse = errors.New("email already in use")
)

// InviteUseCase handles user invitation business logic
type InviteUseCase struct {
	inviteRepo *repository.InviteRepository
	userRepo   *repository.UserRepository
	tenantRepo *repository.TenantRepository
}

// CreateInviteInput contains data to create an invite
type CreateInviteInput struct {
	TenantID  domain.UUID
	Email     string
	Role      domain.UserRole
	InvitedBy domain.UUID
}

// CreateInviteOutput contains the created invite
type CreateInviteOutput struct {
	Invite    *domain.Invite
	Token     string
	ExpiresAt time.Time
}

// AcceptInviteInput contains data to accept an invite
type AcceptInviteInput struct {
	Token    string
	Password string
	Name     string
}

// NewInviteUseCase creates a new InviteUseCase
func NewInviteUseCase(
	inviteRepo *repository.InviteRepository,
	userRepo *repository.UserRepository,
	tenantRepo *repository.TenantRepository,
) *InviteUseCase {
	return &InviteUseCase{
		inviteRepo: inviteRepo,
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
	}
}

// CreateInvite creates a new user invitation
func (uc *InviteUseCase) CreateInvite(ctx context.Context, input CreateInviteInput) (*CreateInviteOutput, error) {
	existingUser, _ := uc.userRepo.GetByEmail(ctx, input.Email)
	if existingUser != nil {
		return nil, ErrEmailAlreadyInUse
	}

	existingInvite, _ := uc.inviteRepo.GetByEmail(ctx, input.Email)
	if existingInvite != nil && existingInvite.Status == domain.InviteStatusPending {
		return nil, ErrEmailAlreadyInUse
	}

	// Validate seat limit before creating invite
	tenant, err := uc.tenantRepo.GetByID(ctx, input.TenantID)
	if err != nil {
		return nil, errors.New("failed to get tenant")
	}
	if tenant == nil {
		return nil, errors.New("tenant not found")
	}

	// Check seat limit if max_users is set
	if tenant.MaxUsers > 0 && tenant.SeatCount >= tenant.MaxUsers {
		return nil, ErrSeatLimitExceeded
	}

	token := uuid.New().String()
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	invite := &domain.Invite{
		ID:        domain.UUID(uuid.New().String()),
		TenantID:  input.TenantID,
		Email:     input.Email,
		Role:      input.Role,
		Token:     token,
		Status:    domain.InviteStatusPending,
		InvitedBy: input.InvitedBy,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	if err := uc.inviteRepo.Create(ctx, invite); err != nil {
		return nil, errors.New("failed to create invite")
	}

	return &CreateInviteOutput{
		Invite:    invite,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateInvite validates an invite token
func (uc *InviteUseCase) ValidateInvite(ctx context.Context, token string) (*domain.Invite, error) {
	invite, err := uc.inviteRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, ErrInviteNotFound
	}

	if invite == nil {
		return nil, ErrInviteNotFound
	}

	if invite.Status != domain.InviteStatusPending {
		if invite.Status == domain.InviteStatusAccepted {
			return nil, ErrInviteAlreadyUsed
		}
		return nil, ErrInviteCancelled
	}

	if time.Now().After(invite.ExpiresAt) {
		return nil, ErrInviteExpired
	}

	return invite, nil
}

// AcceptInviteInput accepts an invitation and creates a user
func (uc *InviteUseCase) AcceptInvite(ctx context.Context, input AcceptInviteInput) (*domain.User, error) {
	invite, err := uc.ValidateInvite(ctx, input.Token)
	if err != nil {
		return nil, err
	}

	existingUser, _ := uc.userRepo.GetByEmail(ctx, invite.Email)
	if existingUser != nil {
		return nil, ErrEmailAlreadyInUse
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &domain.User{
		ID:           domain.UUID(uuid.New().String()),
		TenantID:     invite.TenantID,
		Email:        invite.Email,
		Name:         input.Name,
		PasswordHash: string(hashedPassword),
		Role:         invite.Role,
		Active:       true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, errors.New("failed to create user")
	}

	now := time.Now()
	invite.Status = domain.InviteStatusAccepted
	invite.AcceptedAt = &now
	if err := uc.inviteRepo.Update(ctx, invite); err != nil {
		return nil, errors.New("failed to update invite")
	}

	return user, nil
}

// CancelInvite cancels an invitation
func (uc *InviteUseCase) CancelInvite(ctx context.Context, inviteID domain.UUID) error {
	invite, err := uc.inviteRepo.GetByToken(ctx, string(inviteID))
	if err != nil {
		return ErrInviteNotFound
	}

	if invite.Status != domain.InviteStatusPending {
		return ErrInviteAlreadyUsed
	}

	invite.Status = domain.InviteStatusCancelled
	return uc.inviteRepo.Update(ctx, invite)
}