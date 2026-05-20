package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/redis"
	"github.com/go-postnest/postnest/internal/workers"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler receives Postmark webhooks.
type Handler struct {
	redis *redis.Client
	db    *pgxpool.Pool
}

// NewHandler creates a webhook handler.
func NewHandler(r *redis.Client, db *pgxpool.Pool) *Handler {
	return &Handler{redis: r, db: db}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/webhooks/postmark/inbound", h.handleInbound)
	r.Post("/webhooks/postmark/bounce", h.handleBounce)
	r.Post("/webhooks/postmark/delivery", h.handleDelivery)
	r.Post("/webhooks/postmark/spam", h.handleSpam)
}

// readBody reads the request body and returns it as bytes.
func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	_ = r.Body.Close()
	return body, nil
}

// extractDomain pulls the domain from an email address string.
// It handles display names like "Name" <addr@domain.com>.
func extractDomain(email string) string {
	if addr, err := mail.ParseAddress(email); err == nil {
		email = addr.Address
	}
	if i := strings.LastIndex(email, "@"); i >= 0 && i < len(email)-1 {
		return strings.ToLower(email[i+1:])
	}
	return ""
}

// getDomainFromPayload extracts the recipient domain based on webhook type.
func getDomainFromPayload(payload map[string]any, eventType string) string {
	switch eventType {
	case "inbound":
		if v, ok := payload["OriginalRecipient"].(string); ok && v != "" {
			return extractDomain(v)
		}
		if toFull, ok := payload["ToFull"].([]any); ok && len(toFull) > 0 {
			if first, ok := toFull[0].(map[string]any); ok {
				if v, ok := first["Email"].(string); ok {
					return extractDomain(v)
				}
			}
		}
		if v, ok := payload["To"].(string); ok {
			return extractDomain(v)
		}
	case "bounce", "delivery", "spam":
		if v, ok := payload["Recipient"].(string); ok {
			return extractDomain(v)
		}
	}
	return ""
}

// verifySignature checks the webhook request against the domain's stored Postmark token.
// The secret token is passed as a URL query parameter (e.g. ?token=...) configured in
// Postmark's webhook URL, since Postmark does not send authentication headers.
func (h *Handler) verifySignature(ctx context.Context, domain string, r *http.Request) bool {
	if domain == "" {
		return false
	}

	// Look up the domain's stored token.
	var token string
	err := h.db.QueryRow(ctx, `SELECT COALESCE(postmark_token,'') FROM domains WHERE name=$1`, domain).Scan(&token)
	if err != nil || token == "" {
		return false
	}

	tok := r.URL.Query().Get("token")
	return tok != "" && tok == token
}

func (h *Handler) handleInbound(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}

	domain := getDomainFromPayload(payload, "inbound")
	if !h.verifySignature(r.Context(), domain, r) {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}

	if !h.dedup(r.Context(), payload) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := h.enqueue(r.Context(), "inbound", payload); err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleBounce(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}

	domain := getDomainFromPayload(payload, "bounce")
	if !h.verifySignature(r.Context(), domain, r) {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}

	if !h.dedup(r.Context(), payload) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := h.enqueue(r.Context(), "bounce", payload); err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleDelivery(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}

	domain := getDomainFromPayload(payload, "delivery")
	if !h.verifySignature(r.Context(), domain, r) {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}

	if !h.dedup(r.Context(), payload) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := h.enqueue(r.Context(), "delivery", payload); err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleSpam(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}

	domain := getDomainFromPayload(payload, "spam")
	if !h.verifySignature(r.Context(), domain, r) {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}

	if !h.dedup(r.Context(), payload) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := h.enqueue(r.Context(), "spam", payload); err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) dedup(ctx context.Context, payload map[string]any) bool {
	msgID, _ := payload["MessageID"].(string)
	if msgID == "" {
		return true
	}
	key := "webhook:" + msgID
	ok, err := h.redis.SetNX(ctx, key, "1", 5*time.Minute).Result()
	if err != nil {
		return true // fail open: process on Redis error
	}
	return ok
}

func (h *Handler) enqueue(ctx context.Context, jobType string, payload map[string]any) error {
	pb, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	job := workers.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		Type:        jobType,
		Payload:     pb,
		MaxAttempts: 3,
		CreatedAt:   time.Now().Unix(),
		ScheduledAt: time.Now().Unix(),
	}
	jb, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return h.redis.Enqueue(ctx, "queue:jobs", jb)
}
