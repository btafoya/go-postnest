package webmail

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
)

// Handler implements the webmail REST API.
type Handler struct {
	store mailstore.Store
}

// NewHandler creates a webmail handler.
func NewHandler(store mailstore.Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes mounts routes on a chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/labels", h.listLabels)
	r.Post("/api/v1/labels", h.createLabel)
	r.Patch("/api/v1/labels/{id}", h.updateLabel)
	r.Delete("/api/v1/labels/{id}", h.deleteLabel)

	r.Get("/api/v1/messages", h.listMessages)
	r.Get("/api/v1/messages/{id}", h.getMessage)
	r.Patch("/api/v1/messages/{id}", h.patchMessage)
	r.Delete("/api/v1/messages/{id}", h.deleteMessage)
	r.Post("/api/v1/messages/batch", h.batchMessages)
	r.Post("/api/v1/messages/{id}/labels", h.applyLabels)

	r.Get("/api/v1/threads/{id}", h.getThread)

	r.Post("/api/v1/drafts", h.createDraft)
	r.Put("/api/v1/drafts/{id}", h.updateDraft)
	r.Post("/api/v1/drafts/{id}/send", h.sendDraft)

	r.Get("/api/v1/search", h.search)
}

func (h *Handler) currentUser(r *http.Request) *models.User {
	return api.UserFromContext(r.Context())
}

func (h *Handler) listLabels(w http.ResponseWriter, r *http.Request) {
	u := h.currentUser(r)
	labels, err := h.store.GetLabels(r.Context(), u.ID, u.ID) // domainID should come from context
	if err != nil {
		api.WriteError(w, err)
		return
	}
	// TODO: compute unread/total counts per label
	writeJSON(w, http.StatusOK, map[string]any{"labels": labels})
}

func (h *Handler) createLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	label := &models.Label{
		DomainID: u.ID, // TODO: use domain context
		UserID:   u.ID,
		Name:     req.Name,
		Color:    req.Color,
	}
	if err := h.store.CreateLabel(r.Context(), label); err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, label)
}

func (h *Handler) updateLabel(w http.ResponseWriter, r *http.Request) {
	api.WriteError(w, api.ErrNotFound)
}

func (h *Handler) deleteLabel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	if err := h.store.DeleteLabel(r.Context(), u.ID, u.ID, id); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	u := h.currentUser(r)
	q := r.URL.Query()
	var labelID *uuid.UUID
	if v := q.Get("label_id"); v != "" {
		id, err := uuid.Parse(v)
		if err == nil {
			labelID = &id
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	msgs, total, err := h.store.ListMessages(r.Context(), u.ID, u.ID, labelID, mailstore.ListOptions{
		Limit:     limit,
		Offset:    offset,
		SortField: q.Get("sort"),
		SortDesc:  true,
	})
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs, "total": total})
}

func (h *Handler) getMessage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	msg, err := h.store.GetMessage(r.Context(), u.ID, u.ID, id)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

func (h *Handler) patchMessage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var req struct {
		IsRead    *bool `json:"is_read"`
		IsFlagged *bool `json:"is_flagged"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	patch := mailstore.MessagePatch{IsRead: req.IsRead, IsFlagged: req.IsFlagged}
	if err := h.store.UpdateMessage(r.Context(), u.ID, u.ID, id, patch); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteMessage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	if err := h.store.DeleteMessage(r.Context(), u.ID, u.ID, id); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) batchMessages(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action     string      `json:"action"`
		MessageIDs []uuid.UUID `json:"message_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	for _, id := range req.MessageIDs {
		switch req.Action {
		case "mark_read":
			tr := true
			_ = h.store.UpdateMessage(r.Context(), u.ID, u.ID, id, mailstore.MessagePatch{IsRead: &tr})
		case "mark_unread":
			f := false
			_ = h.store.UpdateMessage(r.Context(), u.ID, u.ID, id, mailstore.MessagePatch{IsRead: &f})
		case "trash":
			_ = h.store.MoveToMailbox(r.Context(), u.ID, u.ID, id, "TRASH")
		case "delete":
			_ = h.store.DeleteMessage(r.Context(), u.ID, u.ID, id)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) applyLabels(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var req struct {
		Add    []uuid.UUID `json:"label_ids"`
		Remove []uuid.UUID `json:"remove_label_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if err := h.store.ApplyLabels(r.Context(), id, req.Add, req.Remove); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getThread(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	thread, msgs, err := h.store.GetThread(r.Context(), u.ID, u.ID, id)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": thread, "messages": msgs})
}

func (h *Handler) createDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject  string            `json:"subject"`
		To       []map[string]string `json:"to"`
		HTMLBody string            `json:"html_body"`
		PlainText string           `json:"plain_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	msg := &models.Message{
		DomainID:  u.ID,
		UserID:    u.ID,
		Subject:   req.Subject,
		HTMLBody:  req.HTMLBody,
		PlainText: req.PlainText,
		IsDraft:   true,
		Mailbox:   "DRAFTS",
		CreatedAt: time.Now().UTC(),
	}
	// TODO: parse To into to_addresses
	_ = msg
	writeJSON(w, http.StatusCreated, map[string]any{"id": uuid.Must(uuid.NewV7())})
}

func (h *Handler) updateDraft(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) sendDraft(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued"})
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	u := h.currentUser(r)
	msgs, total, err := h.store.Search(r.Context(), u.ID, u.ID, q, mailstore.SearchOptions{})
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": msgs, "total": total})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
