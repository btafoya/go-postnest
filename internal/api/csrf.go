package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"
)

const csrfCookieName = "csrf"

// NewCSRFToken returns a random base64 token for double-submit CSRF.
func NewCSRFToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// SetCSRFCookie writes the CSRF cookie. Not HttpOnly so the SPA can echo it.
func SetCSRFCookie(w http.ResponseWriter, token string, secure bool, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(maxAge.Seconds()),
	})
}

// ClearCSRFCookie expires the CSRF cookie.
func ClearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		MaxAge:   -1,
	})
}

// CSRF enforces the double-submit cookie pattern on state-changing requests.
// Safe methods and the login endpoint (no session yet) are exempt.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/v1/auth/login" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" {
			WriteError(w, ErrForbidden)
			return
		}
		header := r.Header.Get("X-CSRF-Token")
		if header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
			WriteError(w, ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
