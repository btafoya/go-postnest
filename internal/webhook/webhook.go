package webhook

import (
	"encoding/json"
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/redis"
	"github.com/go-postnest/postnest/internal/workers"
	"github.com/google/uuid"
)

// Handler receives Postmark webhooks.
type Handler struct {
	redis  *redis.Client
	secret string
}

// NewHandler creates a webhook handler.
func NewHandler(r *redis.Client, secret string) *Handler {
	return &Handler{redis: r, secret: secret}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/webhooks/postmark/inbound", h.handleInbound)
	r.Post("/webhooks/postmark/bounce", h.handleBounce)
	r.Post("/webhooks/postmark/delivery", h.handleDelivery)
	r.Post("/webhooks/postmark/spam", h.handleSpam)
}

func (h *Handler) handleInbound(w http.ResponseWriter, r *http.Request) {
	if !h.verify(r) {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.WriteError(w, api.ErrValidation)
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
	if !h.verify(r) {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.WriteError(w, api.ErrValidation)
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
	if !h.verify(r) {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.WriteError(w, api.ErrValidation)
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
	if !h.verify(r) {
		api.WriteError(w, api.ErrUnauthorized)
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.WriteError(w, api.ErrValidation)
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

func (h *Handler) verify(r *http.Request) bool {
	// In production, verify Postmark signature or compare a shared secret.
	// For now accept all valid JSON POSTs with a basic secret check.
	token := r.Header.Get("X-Postmark-Server-Token")
	return token != "" && token == h.secret
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
