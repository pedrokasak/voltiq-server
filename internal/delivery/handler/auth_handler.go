package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/usecase"
)

// AuthHandler handles authentication HTTP requests
type AuthHandler struct {
	authUseCase   *usecase.AuthUseCase
	signupUseCase *usecase.SignupUseCase
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest represents a signup request
type RegisterRequest struct {
	TenantName     string `json:"tenant_name"`
	TenantDocument string `json:"tenant_document"`
	Plan           string `json:"plan"`
	AdminName      string `json:"admin_name"`
	AdminEmail     string `json:"admin_email"`
	AdminPassword  string `json:"admin_password"`
}

// RefreshTokenRequest represents a refresh token request
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// CookieConfig holds secure cookie configuration
type CookieConfig struct {
	Name     string
	Path     string
	Domain   string
	MaxAge   int
	Secure   bool
	HttpOnly bool
	SameSite http.SameSite
}

// DefaultRefreshCookieConfig returns default secure cookie configuration for refresh tokens
func DefaultRefreshCookieConfig() CookieConfig {
	return CookieConfig{
		Name:     "refresh_token",
		Path:     "/api/v1/auth/refresh",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	}
}

// LogoutRequest represents a logout request
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(authUseCase *usecase.AuthUseCase, signupUseCase *usecase.SignupUseCase) *AuthHandler {
	return &AuthHandler{
		authUseCase:   authUseCase,
		signupUseCase: signupUseCase,
	}
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Email == "" || req.Password == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "email and password are required", nil))
		return
	}

	output, err := h.authUseCase.Login(r.Context(), usecase.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusUnauthorized, request.Fail("INVALID_CREDENTIALS", "invalid email or password", nil))
		return
	}

	// Generate refresh token
	refreshToken, err := h.authUseCase.jwtService.GenerateRefreshToken(
		string(output.User.ID),
		string(output.User.TenantID),
	)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("TOKEN_ERROR", "failed to generate refresh token", nil))
		return
	}

	// Set secure refresh token cookie
	cookieConfig := DefaultRefreshCookieConfig()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieConfig.Name,
		Value:    refreshToken,
		Path:     cookieConfig.Path,
		Domain:   cookieConfig.Domain,
		MaxAge:   cookieConfig.MaxAge,
		Secure:   cookieConfig.Secure,
		HttpOnly: cookieConfig.HttpOnly,
		SameSite: cookieConfig.SameSite,
	})

	response := map[string]any{
		"token":              output.Token,
		"expires_at":         output.ExpiresAt,
		"user":               output.User,
		"refresh_expires_at": time.Now().Add(7 * 24 * time.Hour),
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, "login successful"))
}

// Signup handles tenant and admin user creation (self-service onboarding)
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.TenantName == "" || req.AdminEmail == "" || req.AdminPassword == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "required fields missing", nil))
		return
	}

	output, err := h.signupUseCase.Signup(r.Context(), usecase.SignupInput{
		TenantName:     req.TenantName,
		TenantDocument: req.TenantDocument,
		Plan:           usecase.ParseTenantPlan(req.Plan),
		AdminName:      req.AdminName,
		AdminEmail:     req.AdminEmail,
		AdminPassword:  req.AdminPassword,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("SIGNUP_ERROR", err.Error(), nil))
		return
	}

	response := map[string]any{
		"tenant": output.Tenant,
		"user":   output.User,
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(response, "account created successfully"))
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	// Try to get refresh token from cookie first
	var refreshToken string
	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie != nil {
		refreshToken = cookie.Value
	}

	// If no cookie, try JSON body
	if refreshToken == "" {
		var req RefreshTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
			return
		}
		refreshToken = req.RefreshToken
	}

	if refreshToken == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "refresh_token is required", nil))
		return
	}

	output, err := h.authUseCase.RefreshToken(r.Context(), usecase.RefreshTokenInput{
		RefreshToken: refreshToken,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusUnauthorized, request.Fail("UNAUTHORIZED", "invalid or expired refresh token", nil))
		return
	}

	// Set new secure refresh token cookie
	cookieConfig := DefaultRefreshCookieConfig()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieConfig.Name,
		Value:    output.RefreshToken,
		Path:     cookieConfig.Path,
		Domain:   cookieConfig.Domain,
		MaxAge:   cookieConfig.MaxAge,
		Secure:   cookieConfig.Secure,
		HttpOnly: cookieConfig.HttpOnly,
		SameSite: cookieConfig.SameSite,
	})

	response := map[string]any{
		"token":              output.Token,
		"expires_at":         output.ExpiresAt,
		"refresh_expires_at": output.RefreshExpiry,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, "token refreshed successfully"))
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear refresh token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/api/v1/auth/refresh",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	response := map[string]any{
		"message":         "logout successful",
		"cookies_cleared": true,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, "logout successful"))
}
