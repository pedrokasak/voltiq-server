package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrAlertNotFound = errors.New("alert not found")
)

// AlertUseCase handles alert business logic
type AlertUseCase struct {
	alertRepo *repository.AlertRepository
}

// CreateAlertInput contains data to create an alert configuration
type CreateAlertInput struct {
	TenantID      domain.UUID
	TransformerID domain.UUID
	Type          domain.AlertType
	Channel       domain.AlertChannel
	Recipient     string
}

// UpdateAlertInput contains data to update an alert configuration
type UpdateAlertInput struct {
	ID        domain.UUID
	Type      domain.AlertType
	Channel   domain.AlertChannel
	Recipient string
}

// NewAlertUseCase creates a new AlertUseCase
func NewAlertUseCase(alertRepo *repository.AlertRepository) *AlertUseCase {
	return &AlertUseCase{
		alertRepo: alertRepo,
	}
}

// CreateAlert creates a new alert configuration for a transformer
func (uc *AlertUseCase) CreateAlert(ctx context.Context, input CreateAlertInput) (*domain.Alert, error) {
	// Validate alert type
	if input.Type != domain.AlertTypeWarning && input.Type != domain.AlertTypeCritical {
		return nil, errors.New("invalid alert type: must be WARNING or CRITICAL")
	}

	// Validate channel
	if input.Channel != domain.AlertChannelEmail && input.Channel != domain.AlertChannelWhatsapp {
		return nil, errors.New("invalid channel: must be EMAIL or WHATSAPP")
	}

	// Validate recipient
	if input.Recipient == "" {
		return nil, errors.New("recipient is required")
	}

	now := time.Now()
	alert := &domain.Alert{
		ID:             domain.UUID(uuid.New().String()),
		TenantID:       input.TenantID,
		TransformerID:  input.TransformerID,
		BalanceID:      domain.UUID(""), // Not linked to a specific balance yet
		Type:           input.Type,
		Channel:        input.Channel,
		Recipient:      input.Recipient,
		DeliveryStatus: domain.AlertDeliveryPending,
		CreatedAt:      now,
	}

	if err := uc.alertRepo.Create(ctx, alert); err != nil {
		return nil, errors.New("failed to create alert configuration")
	}

	return alert, nil
}

// GetAlertsByTransformer retrieves all alert configurations for a transformer
func (uc *AlertUseCase) GetAlertsByTransformer(ctx context.Context, transformerID domain.UUID) ([]*domain.Alert, error) {
	alerts, err := uc.alertRepo.GetByTransformer(ctx, transformerID)
	if err != nil {
		return nil, errors.New("failed to get alert configurations")
	}
	return alerts, nil
}

// ListAlertsByTenant retrieves all alert configurations for a tenant
func (uc *AlertUseCase) ListAlertsByTenant(ctx context.Context, tenantID domain.UUID) ([]*domain.Alert, error) {
	alerts, err := uc.alertRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, errors.New("failed to get alert configurations")
	}
	return alerts, nil
}

// GetAlertByID retrieves an alert configuration by ID
func (uc *AlertUseCase) GetAlertByID(ctx context.Context, id domain.UUID) (*domain.Alert, error) {
	alert, err := uc.alertRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if alert == nil {
		return nil, ErrAlertNotFound
	}
	return alert, nil
}

// UpdateAlert updates an alert configuration
func (uc *AlertUseCase) UpdateAlert(ctx context.Context, input UpdateAlertInput) (*domain.Alert, error) {
	alert, err := uc.alertRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if alert == nil {
		return nil, ErrAlertNotFound
	}

	if input.Type != "" {
		if input.Type != domain.AlertTypeWarning && input.Type != domain.AlertTypeCritical {
			return nil, errors.New("invalid alert type: must be WARNING or CRITICAL")
		}
		alert.Type = input.Type
	}

	if input.Channel != "" {
		if input.Channel != domain.AlertChannelEmail && input.Channel != domain.AlertChannelWhatsapp {
			return nil, errors.New("invalid channel: must be EMAIL or WHATSAPP")
		}
		alert.Channel = input.Channel
	}

	if input.Recipient != "" {
		alert.Recipient = input.Recipient
	}

	if err := uc.alertRepo.Update(ctx, alert); err != nil {
		return nil, errors.New("failed to update alert configuration")
	}

	return alert, nil
}

// DeleteAlert deletes an alert configuration
func (uc *AlertUseCase) DeleteAlert(ctx context.Context, id domain.UUID) error {
	alert, err := uc.alertRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if alert == nil {
		return ErrAlertNotFound
	}

	return uc.alertRepo.Delete(ctx, id)
}