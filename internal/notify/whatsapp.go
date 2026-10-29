// Package notify sends alert messages through external channels.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
)

// WhatsAppSender sends WhatsApp text messages.
type WhatsAppSender interface {
	SendText(ctx context.Context, phone, message string) error
}

// ErrNotConfigured is returned when a channel has no credentials.
var ErrNotConfigured = errors.New("channel not configured")

// ZAPIConfig holds Z-API credentials (SDD 03-tecnologias: WhatsApp via Z-API).
type ZAPIConfig struct {
	InstanceID  string
	Token       string
	ClientToken string
	BaseURL     string
}

// ZAPIConfigFromEnv reads ZAPI_INSTANCE_ID, ZAPI_TOKEN, ZAPI_CLIENT_TOKEN.
func ZAPIConfigFromEnv() ZAPIConfig {
	return ZAPIConfig{
		InstanceID:  os.Getenv("ZAPI_INSTANCE_ID"),
		Token:       os.Getenv("ZAPI_TOKEN"),
		ClientToken: os.Getenv("ZAPI_CLIENT_TOKEN"),
		BaseURL:     os.Getenv("ZAPI_BASE_URL"),
	}
}

// Configured reports whether the credentials are present.
func (c ZAPIConfig) Configured() bool {
	return c.InstanceID != "" && c.Token != ""
}

// ZAPISender sends messages with the Z-API REST API.
type ZAPISender struct {
	cfg    ZAPIConfig
	client *http.Client
}

// NewZAPISender creates a Z-API sender.
func NewZAPISender(cfg ZAPIConfig) *ZAPISender {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.z-api.io"
	}
	return &ZAPISender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

// NormalizePhone keeps digits and prefixes Brazil's country code (55) when
// the number has only DDD + number (10 or 11 digits).
func NormalizePhone(phone string) (string, error) {
	var b strings.Builder
	for _, c := range phone {
		if unicode.IsDigit(c) {
			b.WriteRune(c)
		}
	}
	digits := b.String()
	switch {
	case len(digits) == 10 || len(digits) == 11:
		digits = "55" + digits
	case len(digits) == 12 || len(digits) == 13:
	default:
		return "", fmt.Errorf("telefone inválido: use DDD + número, ex. (11) 98765-4321")
	}
	return digits, nil
}

// SendText sends a text message to phone.
func (s *ZAPISender) SendText(ctx context.Context, phone, message string) error {
	if !s.cfg.Configured() {
		return ErrNotConfigured
	}
	number, err := NormalizePhone(phone)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"phone": number, "message": message})
	url := fmt.Sprintf("%s/instances/%s/token/%s/send-text", strings.TrimRight(s.cfg.BaseURL, "/"), s.cfg.InstanceID, s.cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.ClientToken != "" {
		req.Header.Set("Client-Token", s.cfg.ClientToken)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("z-api request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("z-api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}
