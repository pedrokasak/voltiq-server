package handler

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
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

// RegisterRequest represents a signup request (SDD 05-api-endpoints). The
// web client field names are accepted as aliases.
type RegisterRequest struct {
	TenantName     string `json:"tenant_name"`
	TenantDocument string `json:"tenant_document"`
	Plan           string `json:"plan"`
	AdminName      string `json:"admin_name"`
	AdminEmail     string `json:"admin_email"`
	AdminPassword  string `json:"admin_password"`

	CompanyName string `json:"company_name"`
	CNPJ        string `json:"cnpj"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

func (r *RegisterRequest) normalize() {
	pick := func(a, b string) string {
		if strings.TrimSpace(a) != "" {
			return a
		}
		return b
	}
	r.TenantName = pick(r.TenantName, r.CompanyName)
	r.TenantDocument = pick(r.TenantDocument, r.CNPJ)
	r.AdminName = pick(r.AdminName, r.Name)
	r.AdminEmail = pick(r.AdminEmail, r.Email)
	r.AdminPassword = pick(r.AdminPassword, r.Password)
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

// DefaultRefreshCookieConfig returns the refresh cookie configuration.
// COOKIE_SAMESITE (strict|lax|none, padrao strict), COOKIE_SECURE (padrao
// true) e COOKIE_DOMAIN permitem frontend e API em dominios diferentes: nesse
// caso use COOKIE_SAMESITE=none (exige Secure).
func DefaultRefreshCookieConfig() CookieConfig {
	cfg := CookieConfig{
		Name:     "refresh_token",
		Path:     "/api/v1/auth/refresh",
		Domain:   os.Getenv("COOKIE_DOMAIN"),
		Secure:   os.Getenv("COOKIE_SECURE") != "false",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	}
	switch strings.ToLower(os.Getenv("COOKIE_SAMESITE")) {
	case "lax":
		cfg.SameSite = http.SameSiteLaxMode
	case "none":
		cfg.SameSite = http.SameSiteNoneMode
		cfg.Secure = true
	}
	return cfg
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(authUseCase *usecase.AuthUseCase, signupUseCase *usecase.SignupUseCase) *AuthHandler {
	return &AuthHandler{authUseCase: authUseCase, signupUseCase: signupUseCase}
}

func setRefreshCookie(w http.ResponseWriter, token string, expires time.Time) {
	cfg := DefaultRefreshCookieConfig()
	maxAge := int(time.Until(expires).Seconds())
	if maxAge <= 0 {
		maxAge = cfg.MaxAge
	}
	http.SetCookie(w, &http.Cookie{
		Name: cfg.Name, Value: token, Path: cfg.Path, Domain: cfg.Domain, MaxAge: maxAge,
		Secure: cfg.Secure, HttpOnly: cfg.HttpOnly, SameSite: cfg.SameSite,
	})
}

func clearRefreshCookie(w http.ResponseWriter) {
	cfg := DefaultRefreshCookieConfig()
	http.SetCookie(w, &http.Cookie{
		Name: cfg.Name, Value: "", Path: cfg.Path, Domain: cfg.Domain, MaxAge: -1,
		Secure: cfg.Secure, HttpOnly: cfg.HttpOnly, SameSite: cfg.SameSite,
	})
}

func sessionResponse(out *usecase.LoginOutput) map[string]any {
	return map[string]any{
		"token":              out.Token,
		"access_token":       out.Token,
		"expires_at":         out.ExpiresAt,
		"user":               out.User,
		"refresh_expires_at": out.RefreshExpiry,
	}
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "e-mail e senha são obrigatórios", nil))
		return
	}

	output, err := h.authUseCase.Login(r.Context(), usecase.LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		if errors.Is(err, usecase.ErrUserInactive) {
			request.WriteJSON(w, http.StatusForbidden, request.Fail("USER_INACTIVE", "usuário desativado; fale com o administrador", nil))
			return
		}
		if errors.Is(err, usecase.ErrInvalidCredentials) {
			request.WriteJSON(w, http.StatusUnauthorized, request.Fail("INVALID_CREDENTIALS", "e-mail ou senha inválidos", nil))
			return
		}
		writeUseCaseError(w, err, "LOGIN_ERROR")
		return
	}

	setRefreshCookie(w, output.RefreshToken, output.RefreshExpiry)
	request.WriteJSON(w, http.StatusOK, request.Success(sessionResponse(output), "login successful"))
}

// Signup creates the tenant in trial and its owner, and starts a session
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.normalize()

	output, err := h.signupUseCase.Signup(r.Context(), usecase.SignupInput{
		TenantName:     req.TenantName,
		TenantDocument: req.TenantDocument,
		Plan:           usecase.ParseTenantPlan(req.Plan),
		AdminName:      req.AdminName,
		AdminEmail:     req.AdminEmail,
		AdminPassword:  req.AdminPassword,
	})
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrTenantExists):
			request.WriteJSON(w, http.StatusConflict, request.Fail("TENANT_EXISTS", "já existe uma conta com este CNPJ", nil))
		case errors.Is(err, usecase.ErrEmailAlreadyInUse):
			request.WriteJSON(w, http.StatusConflict, request.Fail("EMAIL_IN_USE", "este e-mail já está em uso", nil))
		default:
			writeUseCaseError(w, err, "SIGNUP_ERROR")
		}
		return
	}

	response := map[string]any{"tenant": output.Tenant, "user": output.User}
	if session, err := h.authUseCase.IssueFor(output.User); err == nil {
		setRefreshCookie(w, session.RefreshToken, session.RefreshExpiry)
		for k, v := range sessionResponse(session) {
			response[k] = v
		}
	}
	request.WriteJSON(w, http.StatusCreated, request.Success(response, "account created successfully"))
}

// RefreshToken rotates the refresh cookie and returns a new access token
// with the user, so the client can restore the session after a reload.
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var refreshToken string
	if cookie, err := r.Cookie("refresh_token"); err == nil {
		refreshToken = cookie.Value
	}
	if refreshToken == "" && r.ContentLength > 0 {
		var req RefreshTokenRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		refreshToken = req.RefreshToken
	}
	if refreshToken == "" {
		request.WriteJSON(w, http.StatusUnauthorized, request.Fail("UNAUTHORIZED", "sessão expirada", nil))
		return
	}

	output, err := h.authUseCase.RefreshToken(r.Context(), usecase.RefreshTokenInput{RefreshToken: refreshToken})
	if err != nil {
		clearRefreshCookie(w)
		request.WriteJSON(w, http.StatusUnauthorized, request.Fail("UNAUTHORIZED", "sessão expirada", nil))
		return
	}

	setRefreshCookie(w, output.RefreshToken, output.RefreshExpiry)
	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{
		"token":              output.Token,
		"access_token":       output.Token,
		"expires_at":         output.ExpiresAt,
		"refresh_expires_at": output.RefreshExpiry,
		"user":               output.User,
	}, "token refreshed successfully"))
}

// Logout clears the refresh cookie
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	clearRefreshCookie(w)
	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{
		"message":         "logout successful",
		"cookies_cleared": true,
	}, "logout successful"))
}

// ChangePassword changes the password of the authenticated user
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.authUseCase.ChangePassword(r.Context(), middleware.GetUserID(r.Context()), req.CurrentPassword, req.NewPassword); err != nil {
		writeUseCaseError(w, err, "PASSWORD_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(nil, "senha alterada"))
}
