package usecase

import (
	"errors"
	"fmt"
)

// Common usecase errors
var (
	ErrTenantNotFound            = errors.New("tenant not found")
	ErrTenantAlreadyActive       = errors.New("tenant is already active")
	ErrTenantExists              = errors.New("tenant with this document already exists")
	ErrInvalidPlan               = errors.New("invalid plan")
	ErrSeatLimitExceeded         = errors.New("seat limit exceeded - upgrade plan to add more users")
	ErrCustomerNotFound          = errors.New("customer not found")
	ErrSubscriptionNotFound      = errors.New("subscription not found")
	ErrSubscriptionAlreadyActive = errors.New("subscription already active")
	ErrPaymentProviderError      = errors.New("payment provider error")
)

// Generic errors mapped to HTTP status codes by the delivery layer.
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrForbidden  = errors.New("forbidden")
	ErrBadRequest = errors.New("bad request")
)

// badRequest wraps a user-facing validation message.
func badRequest(msg string) error {
	return fmt.Errorf("%w: %s", ErrBadRequest, msg)
}

// conflict wraps a user-facing conflict message.
func conflict(msg string) error {
	return fmt.Errorf("%w: %s", ErrConflict, msg)
}
