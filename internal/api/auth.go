package api

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/go-postnest/postnest/internal/auth"
)

// AuthHandler provides public authentication endpoints.
type AuthHandler struct {
	Auth       *auth.Service
	SessionKey string
}

// NewAuthHandler creates an auth handler.
func NewAuthHandler(authSvc *auth.Service, sessionKey string) *AuthHandler {
	return &AuthHandler{Auth: authSvc, SessionKey: sessionKey}
}

// RegisterRoutes mounts public auth routes.
func (h *AuthHandler) RegisterRoutes(r interface{ Post(string, http.HandlerFunc); Get(string, http.HandlerFunc) }) {
	r.Post("/api/v1/auth/login", h.Login)
	r.Post("/api/v1/auth/logout", h.Logout)
	r.Get("/api/v1/auth/me", h.Me)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Login authenticates a user and creates a session.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, ErrValidation)
		return
	}
	if req.Email == "" || req.Password == "" {
		WriteError(w, ErrValidation)
		return
	}

	user, err := h.Auth.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		WriteError(w, ErrUnauthorized)
		return
	}

	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteIP = host
	}
	session, token, err := h.Auth.CreateSession(r.Context(), user.ID, remoteIP, r.UserAgent(), 7*24*time.Hour)
	if err != nil {
		WriteError(w, ErrInternal)
		return
	}

	secure := r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":           user.ID,
			"email":        user.Email,
			"display_name": user.DisplayName,
			"is_super_admin": user.IsSuperAdmin,
		},
		"session": map[string]any{
			"id":         session.ID,
			"expires_at": session.ExpiresAt,
		},
	})
}

// Logout invalidates the current session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token != "" {
		_ = h.Auth.Logout(r.Context(), token)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Me returns the currently authenticated user.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token == "" {
		WriteError(w, ErrUnauthorized)
		return
	}

	_, user, err := h.Auth.ValidateSession(r.Context(), token)
	if err != nil {
		WriteError(w, ErrUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":             user.ID,
			"email":          user.Email,
			"display_name":   user.DisplayName,
			"is_super_admin": user.IsSuperAdmin,
		},
	})
}
