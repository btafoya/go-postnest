package webmail

import (
	"encoding/json"
	"fmt"
	"context"
	"io"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/go-postnest/postnest/internal/redis"
	"github.com/go-postnest/postnest/internal/workers"
	"github.com/microcosm-cc/bluemonday"
)

// DomainLister returns domain memberships for a user.
type DomainLister interface {
	GetUserDomains(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error)
}

// Handler implements the webmail REST API.
type Handler struct {
	store             mailstore.Store
	auth              DomainLister
	redis             *redis.Client
	maxAttachmentSize int64
}

// NewHandler creates a webmail handler.
func NewHandler(store mailstore.Store, authSvc DomainLister, redis *redis.Client, maxAttachmentSize int64) *Handler {
	if maxAttachmentSize <= 0 {
		maxAttachmentSize = 25 << 20
	}
	return &Handler{store: store, auth: authSvc, redis: redis, maxAttachmentSize: maxAttachmentSize}
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
	r.Get("/api/v1/drafts/{id}/attachments", h.listAttachments)
	r.Post("/api/v1/drafts/{id}/attachments", h.uploadAttachment)
	r.Delete("/api/v1/drafts/{id}/attachments/{attID}", h.deleteAttachment)

	r.Get("/api/v1/search", h.search)
}

func (h *Handler) currentUser(r *http.Request) *models.User {
	return api.UserFromContext(r.Context())
}

func (h *Handler) domainID(r *http.Request) uuid.UUID {
	if id := api.DomainIDFromContext(r.Context()); id != uuid.Nil {
		return id
	}
	u := h.currentUser(r)
	if u == nil {
		return uuid.Nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	doms, err := h.auth.GetUserDomains(ctx, u.ID)
	if err != nil || len(doms) == 0 {
		return uuid.Nil
	}
	return doms[0].DomainID
}

// resolveDomain returns the request's domain ID, or writes a 4xx and
// returns ok=false when the authenticated user has no domain membership.
// Mutating handlers MUST guard with this; a Nil domain_id violates the
// messages_domain_id_fkey constraint.
func (h *Handler) resolveDomain(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	did := h.domainID(r)
	if did == uuid.Nil {
		api.WriteError(w, &api.AppError{
			Code:       "no_domain",
			Message:    "User is not a member of any domain",
			StatusCode: http.StatusForbidden,
		})
		return uuid.Nil, false
	}
	return did, true
}

func (h *Handler) listLabels(w http.ResponseWriter, r *http.Request) {
	u := h.currentUser(r)
	did := h.domainID(r)
	labels, err := h.store.GetLabels(r.Context(), did, u.ID)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	type labelOut struct {
		ID       uuid.UUID `json:"id"`
		Name     string    `json:"name"`
		Color    string    `json:"color"`
		IsSystem bool       `json:"is_system"`
		Total    int64     `json:"total"`
		Unread   int64     `json:"unread"`
	}
	counts, err := h.store.CountsByLabel(r.Context(), did, u.ID)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	out := make([]labelOut, 0, len(labels))
	for _, l := range labels {
		lo := labelOut{ID: l.ID, Name: l.Name, Color: l.Color, IsSystem: l.IsSystem}
		if c, ok := counts[l.ID]; ok {
			lo.Total = c.Total
			lo.Unread = c.Unread
		}
		out = append(out, lo)
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": out})
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
		DomainID: h.domainID(r),
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
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	if err := h.store.UpdateLabel(r.Context(), h.domainID(r), u.ID, id, req.Name, req.Color); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteLabel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	if err := h.store.DeleteLabel(r.Context(), h.domainID(r), u.ID, id); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	u := h.currentUser(r)
	q := r.URL.Query()
	var labelID *uuid.UUID
	var mailboxFilter string
	if v := q.Get("label_id"); v != "" {
		switch v {
		case "inbox":
			mailboxFilter = "INBOX"
		case "sent", "drafts", "trash", "junk":
			mailboxFilter = strings.ToUpper(v)
		default:
			id, err := uuid.Parse(v)
			if err == nil {
				labelID = &id
			}
		}
	} else {
		mailboxFilter = "INBOX"
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	msgs, total, err := h.store.ListMessages(r.Context(), h.domainID(r), u.ID, labelID, mailstore.ListOptions{
		Limit:     limit,
		Offset:    offset,
		SortField: q.Get("sort"),
		SortDesc:  true,
		Mailbox:   mailboxFilter,
	})
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": toMessageDTOs(msgs), "total": total})
}

func (h *Handler) getMessage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	msg, err := h.store.GetMessage(r.Context(), h.domainID(r), u.ID, id)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	var labelNames []string
	if lbls, lerr := h.store.GetMessageLabels(r.Context(), msg.ID); lerr == nil {
		for _, l := range lbls {
			labelNames = append(labelNames, l.Name)
		}
	}
	writeJSON(w, http.StatusOK, toMessageDTO(msg, labelNames))
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
	if err := h.store.UpdateMessage(r.Context(), h.domainID(r), u.ID, id, patch); err != nil {
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
	if err := h.store.DeleteMessage(r.Context(), h.domainID(r), u.ID, id); err != nil {
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
	switch req.Action {
	case "mark_read", "mark_unread", "trash", "archive", "spam", "delete":
	default:
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	did := h.domainID(r)
	succeeded := make([]string, 0, len(req.MessageIDs))
	type failure struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	failed := make([]failure, 0)
	for _, id := range req.MessageIDs {
		var err error
		switch req.Action {
		case "mark_read":
			tr := true
			err = h.store.UpdateMessage(r.Context(), did, u.ID, id, mailstore.MessagePatch{IsRead: &tr})
		case "mark_unread":
			f := false
			err = h.store.UpdateMessage(r.Context(), did, u.ID, id, mailstore.MessagePatch{IsRead: &f})
		case "trash":
			err = h.store.MoveToMailbox(r.Context(), did, u.ID, id, "TRASH")
		case "archive":
			err = h.store.MoveToMailbox(r.Context(), did, u.ID, id, "ARCHIVE")
		case "spam":
			err = h.store.MoveToMailbox(r.Context(), did, u.ID, id, "SPAM")
		case "delete":
			err = h.store.DeleteMessage(r.Context(), did, u.ID, id)
		}
		if err != nil {
			failed = append(failed, failure{ID: id.String(), Error: err.Error()})
		} else {
			succeeded = append(succeeded, id.String())
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"succeeded": succeeded, "failed": failed})
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
	thread, msgs, err := h.store.GetThread(r.Context(), h.domainID(r), u.ID, id)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": thread, "messages": toMessageDTOs(msgs)})
}

func (h *Handler) createDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject   string              `json:"subject"`
		To        []map[string]string `json:"to"`
		Cc        []map[string]string `json:"cc"`
		Bcc       []map[string]string `json:"bcc"`
		HTMLBody  string              `json:"html_body"`
		PlainText string              `json:"plain_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	did, ok := h.resolveDomain(w, r)
	if !ok {
		return
	}
	toAddrs := extractAddresses(req.To)
	ccAddrs := extractAddresses(req.Cc)
	bccAddrs := extractAddresses(req.Bcc)
	if err := validateEmailAddresses(append(append(append([]string{}, toAddrs...), ccAddrs...), bccAddrs...)); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	msg := &models.Message{
		DomainID:     did,
		UserID:       u.ID,
		FromAddress:  u.Email,
		Subject:      req.Subject,
		ToAddresses:  toAddrs,
		CcAddresses:  ccAddrs,
		BccAddresses: bccAddrs,
		HTMLBody:     bluemonday.UGCPolicy().Sanitize(req.HTMLBody),
		PlainText:    req.PlainText,
		IsDraft:      true,
		Mailbox:      "DRAFTS",
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.store.CreateMessage(r.Context(), msg, nil, nil); err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": msg.ID})
}

func (h *Handler) updateDraft(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	var req struct {
		Subject   string              `json:"subject"`
		To        []map[string]string `json:"to"`
		Cc        []map[string]string `json:"cc"`
		Bcc       []map[string]string `json:"bcc"`
		HTMLBody  string              `json:"html_body"`
		PlainText string              `json:"plain_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	did, ok := h.resolveDomain(w, r)
	if !ok {
		return
	}
	toAddrs := extractAddresses(req.To)
	ccAddrs := extractAddresses(req.Cc)
	bccAddrs := extractAddresses(req.Bcc)
	if err := validateEmailAddresses(append(append(append([]string{}, toAddrs...), ccAddrs...), bccAddrs...)); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	sanitizedHTML := bluemonday.UGCPolicy().Sanitize(req.HTMLBody)
	patch := mailstore.MessagePatch{
		Subject:      &req.Subject,
		HTMLBody:     &sanitizedHTML,
		PlainText:    &req.PlainText,
		ToAddresses:  toAddrs,
		CcAddresses:  ccAddrs,
		BccAddresses: bccAddrs,
	}
	if err := h.store.UpdateMessage(r.Context(), did, u.ID, id, patch); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// extractAddresses pulls non-empty "address" values from a recipient list.
func extractAddresses(list []map[string]string) []string {
	var out []string
	for _, t := range list {
		if addr, ok := t["address"]; ok && addr != "" {
			out = append(out, addr)
		}
	}
	return out
}

// validateEmailAddresses checks that all strings are valid RFC 5322 addresses.
func validateEmailAddresses(addrs []string) error {
	for _, a := range addrs {
		if _, err := mail.ParseAddress(a); err != nil {
			return fmt.Errorf("invalid email address: %s", a)
		}
	}
	return nil
}

func (h *Handler) sendDraft(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	did, ok := h.resolveDomain(w, r)
	if !ok {
		return
	}
	msg, err := h.store.GetMessage(r.Context(), did, u.ID, id)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	if !msg.IsDraft {
		api.WriteError(w, api.ErrValidation)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"draft_id":     id.String(),
		"user_id":      u.ID.String(),
		"domain_id":    did.String(),
		"from_address": u.Email,
	})
	job := workers.Job{
		ID:          uuid.Must(uuid.NewV7()).String(),
		Type:        "send_draft",
		Payload:     payload,
		MaxAttempts: 3,
		CreatedAt:   time.Now().Unix(),
		ScheduledAt: time.Now().Unix(),
	}
	jb, _ := json.Marshal(job)
	if err := h.redis.Enqueue(r.Context(), "queue:jobs", jb); err != nil {
		api.WriteError(w, api.ErrInternal)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued"})
}

func (h *Handler) listAttachments(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	if _, err := h.store.GetMessage(r.Context(), h.domainID(r), u.ID, id); err != nil {
		api.WriteError(w, err)
		return
	}
	atts, err := h.store.ListMessageAttachments(r.Context(), id)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	type attOut struct {
		ID       uuid.UUID `json:"id"`
		Filename string    `json:"filename"`
		Size     int       `json:"size"`
		Type     string    `json:"content_type"`
	}
	out := make([]attOut, 0, len(atts))
	for _, a := range atts {
		out = append(out, attOut{ID: a.ID, Filename: a.Filename, Size: a.SizeBytes, Type: a.ContentType})
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachments": out})
}

func (h *Handler) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	msg, err := h.store.GetMessage(r.Context(), h.domainID(r), u.ID, id)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	if !msg.IsDraft {
		api.WriteError(w, api.ErrValidation)
		return
	}
	if err := r.ParseMultipartForm(h.maxAttachmentSize + (1 << 20)); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	defer file.Close()
	if header.Size > h.maxAttachmentSize {
		api.WriteError(w, api.NewValidationError([]api.FieldError{{Field: "file", Issue: "too_large"}}))
		return
	}
	data := make([]byte, header.Size)
	if _, err := io.ReadFull(file, data); err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	ct := header.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	att := &models.Attachment{
		MessageID:   id,
		Filename:    header.Filename,
		ContentType: ct,
		SizeBytes:   int(header.Size),
		Data:        data,
	}
	if err := h.store.CreateAttachments(r.Context(), []*models.Attachment{att}); err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": att.ID, "filename": att.Filename, "size": att.SizeBytes})
}

func (h *Handler) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	attID, err := uuid.Parse(chi.URLParam(r, "attID"))
	if err != nil {
		api.WriteError(w, api.ErrValidation)
		return
	}
	u := h.currentUser(r)
	if _, err := h.store.GetMessage(r.Context(), h.domainID(r), u.ID, id); err != nil {
		api.WriteError(w, err)
		return
	}
	att, err := h.store.GetAttachment(r.Context(), attID)
	if err != nil || att.MessageID != id {
		api.WriteError(w, api.ErrNotFound)
		return
	}
	if err := h.store.DeleteAttachment(r.Context(), attID); err != nil {
		api.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	u := h.currentUser(r)
	msgs, total, err := h.store.Search(r.Context(), h.domainID(r), u.ID, q, mailstore.SearchOptions{})
	if err != nil {
		api.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": toMessageDTOs(msgs), "total": total})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
