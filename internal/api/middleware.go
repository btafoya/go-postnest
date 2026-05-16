package api



import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/models"
)

// Context keys.
type ctxKey string

const (
	ctxKeyUser      ctxKey = "user"
	ctxKeyDomainID  ctxKey = "domain_id"
	ctxKeyRequestID ctxKey = "request_id"
)

// RequestID middleware injects a unique request ID.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 12)
			_, _ = rand.Read(b)
			id = base64.RawURLEncoding.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyRequestID, id)))
	})
}

// StructuredLogger logs every request.
func StructuredLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"duration", time.Since(start).String(),
				"request_id", RequestIDFromContext(r.Context()),
			)
		})
	}
}

// Recovery recovers from panics and returns 500.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				WriteError(w, fmt.Errorf("panic: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS adds basic CORS headers.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Domain-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireSession validates session cookies or Bearer tokens.
func RequireSession(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				WriteError(w, ErrUnauthorized)
				return
			}
			session, user, err := svc.ValidateSession(r.Context(), token)
			if err != nil {
				// try api key
				session, user, err = svc.ValidateAPIKey(r.Context(), token)
				if err != nil {
					WriteError(w, ErrUnauthorized)
					return
				}
			}
			ctx := context.WithValue(r.Context(), ctxKeyUser, user)
			ctx = context.WithValue(ctx, ctxKeyRequestID, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireDomainAdmin ensures the user is an admin for the requested domain.
func RequireDomainAdmin(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				WriteError(w, ErrUnauthorized)
				return
			}
			if user.IsSuperAdmin {
				next.ServeHTTP(w, r)
				return
			}
			domainIDStr := r.URL.Query().Get("domain_id")
			if domainIDStr == "" {
				domainIDStr = r.Header.Get("X-Domain-ID")
			}
			if domainIDStr == "" {
				WriteError(w, NewValidationError([]FieldError{{Field: "domain_id", Issue: "required"}}))
				return
			}
			domainID, err := models.ParseUUID(domainIDStr)
			if err != nil {
				WriteError(w, NewValidationError([]FieldError{{Field: "domain_id", Issue: "invalid_uuid"}}))
				return
			}
			ok, err := svc.IsDomainAdmin(r.Context(), user.ID, domainID)
			if err != nil || !ok {
				WriteError(w, ErrForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyDomainID, domainID)))
		})
	}
}

// extractToken pulls the bearer token from Authorization header or Cookie.
func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if c, err := r.Cookie("session"); err == nil {
		return c.Value
	}
	return ""
}

// UserFromContext returns the authenticated user.
func UserFromContext(ctx context.Context) *models.User {
	if u, ok := ctx.Value(ctxKeyUser).(*models.User); ok {
		return u
	}
	return nil
}

// DomainIDFromContext returns the active domain ID.

func DomainIDFromContext(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(ctxKeyDomainID).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

// RequestIDFromContext returns the request ID.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return id
	}
	return ""
}


// ParseUUID parses a UUID string into uuid.UUID.
func ParseUUID(s string) (uuid.UUID, error) {
	return models.ParseUUID(s)
}
