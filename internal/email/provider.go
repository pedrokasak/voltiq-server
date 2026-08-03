package email

import (
	"context"
)

// EmailProvider defines the interface for sending emails
type EmailProvider interface {
	SendEmail(ctx context.Context, input SendEmailInput) error
	SendTemplateEmail(ctx context.Context, input SendTemplateEmailInput) error
}

// SendEmailInput input for sending a simple email
type SendEmailInput struct {
	To       []string
	Subject  string
	HTMLBody string
	TextBody string
	From     string
	ReplyTo  string
}

// SendTemplateEmailInput input for sending a templated email
type SendTemplateEmailInput struct {
	To           []string
	Subject      string
	TemplateID   string
	TemplateData map[string]any
	From         string
	ReplyTo      string
}

// EmailConfig holds email configuration
type EmailConfig struct {
	Provider    string // "resend", "sendgrid", "ses"
	APIKey      string
	FromEmail   string
	FromName    string
	ReplyTo     string
	BaseURL     string // For links in templates
}