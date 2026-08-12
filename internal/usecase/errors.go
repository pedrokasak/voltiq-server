package usecase

import "errors"

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