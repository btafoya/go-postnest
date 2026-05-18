package webmail

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-postnest/postnest/internal/api"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/google/uuid"
)

// mockStore is a minimal in-memory store for testing.
type mockStore struct {
	messages []*models.Message
	labels   []*models.Label
}

func (m *mockStore) CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error {
	m.messages = append(m.messages, msg)
	return nil
}
func (m *mockStore) GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*models.Message, error) {
	for _, msg := range m.messages {
		if msg.ID == messageID {
			return msg, nil
		}
	}
	return nil, mailstore.ErrNotFound
}
func (m *mockStore) ListMessages(ctx context.Context, domainID, userID uuid.UUID, labelID *uuid.UUID, opts mailstore.ListOptions) ([]*models.Message, int64, error) {
	return m.messages, int64(len(m.messages)), nil
}
func (m *mockStore) UpdateMessage(ctx context.Context, domainID, userID, messageID uuid.UUID, patch mailstore.MessagePatch) error {
	for _, msg := range m.messages {
		if msg.ID == messageID {
			if patch.Subject != nil {
				msg.Subject = *patch.Subject
			}
			if patch.HTMLBody != nil {
				msg.HTMLBody = *patch.HTMLBody
			}
			if patch.PlainText != nil {
				msg.PlainText = *patch.PlainText
			}
			if patch.ToAddresses != nil {
				msg.ToAddresses = patch.ToAddresses
			}
			if patch.IsDraft != nil {
				msg.IsDraft = *patch.IsDraft
			}
			if patch.IsOutbound != nil {
				msg.IsOutbound = *patch.IsOutbound
			}
			if patch.Mailbox != nil {
				msg.Mailbox = *patch.Mailbox
			}
			return nil
		}
	}
	return mailstore.ErrNotFound
}
func (m *mockStore) DeleteMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) error { return nil }
func (m *mockStore) MoveToMailbox(ctx context.Context, domainID, userID, messageID uuid.UUID, mailbox string) error { return nil }
func (m *mockStore) CreateLabel(ctx context.Context, label *models.Label) error {
	m.labels = append(m.labels, label)
	return nil
}
func (m *mockStore) GetLabels(ctx context.Context, domainID, userID uuid.UUID) ([]*models.Label, error) {
	return m.labels, nil
}
func (m *mockStore) GetLabelByName(ctx context.Context, domainID, userID uuid.UUID, name string) (*models.Label, error) {
	return nil, mailstore.ErrNotFound
}
func (m *mockStore) DeleteLabel(ctx context.Context, domainID, userID, labelID uuid.UUID) error { return nil }
func (m *mockStore) UpdateLabel(ctx context.Context, domainID, userID, labelID uuid.UUID, name, color string) error {
	for _, l := range m.labels {
		if l.ID == labelID {
			l.Name = name
			l.Color = color
			return nil
		}
	}
	return mailstore.ErrNotFound
}
func (m *mockStore) ApplyLabels(ctx context.Context, messageID uuid.UUID, addLabelIDs, removeLabelIDs []uuid.UUID) error { return nil }
func (m *mockStore) GetMessageLabels(ctx context.Context, messageID uuid.UUID) ([]*models.Label, error) { return nil, nil }
func (m *mockStore) GetThread(ctx context.Context, domainID, userID, threadID uuid.UUID) (*models.Thread, []*models.Message, error) {
	return nil, nil, nil
}
func (m *mockStore) FindOrCreateThread(ctx context.Context, domainID, userID uuid.UUID, subject, messageID, inReplyTo string, references []string) (*models.Thread, error) {
	return nil, nil
}
func (m *mockStore) CreateAttachments(ctx context.Context, attachments []*models.Attachment) error { return nil }
func (m *mockStore) GetAttachment(ctx context.Context, attachmentID uuid.UUID) (*models.Attachment, error) { return nil, nil }
func (m *mockStore) SetFlag(ctx context.Context, messageID uuid.UUID, flag string) error { return nil }
func (m *mockStore) ClearFlag(ctx context.Context, messageID uuid.UUID, flag string) error { return nil }
func (m *mockStore) GetFlags(ctx context.Context, messageID uuid.UUID) ([]string, error) { return nil, nil }
func (m *mockStore) GetFlagsBatch(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID][]string, error) { return nil, nil }
func (m *mockStore) Search(ctx context.Context, domainID, userID uuid.UUID, query string, opts mailstore.SearchOptions) ([]*models.Message, int64, error) {
	return nil, 0, nil
}
func (m *mockStore) UpdateSearchVector(ctx context.Context, messageID uuid.UUID) error { return nil }
func (m *mockStore) CountUnreadByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockStore) CountTotalByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error) {
	return int64(len(m.messages)), nil
}
func (m *mockStore) CountsByLabel(ctx context.Context, domainID, userID uuid.UUID) (map[uuid.UUID]mailstore.LabelCounts, error) {
	return map[uuid.UUID]mailstore.LabelCounts{}, nil
}
func (m *mockStore) GetMessageSource(ctx context.Context, domainID, userID, messageID uuid.UUID) ([]byte, error) {
	return nil, nil
}
func (m *mockStore) ListMessageAttachments(ctx context.Context, messageID uuid.UUID) ([]*models.Attachment, error) {
	return nil, nil
}
func (m *mockStore) DeleteAttachment(ctx context.Context, attachmentID uuid.UUID) error { return nil }
func (m *mockStore) CountMessagesToday(ctx context.Context) (int64, error) { return 0, nil }
func (m *mockStore) CreateDeliveryLog(ctx context.Context, log *models.DeliveryLog) error { return nil }

// mockAuth implements just enough for domain context.
type mockAuth struct{}

func (a *mockAuth) GetUserDomains(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error) {
	return []*models.DomainMember{{DomainID: userID, UserID: userID, Role: "admin"}}, nil
}

func newTestHandler() (*Handler, *mockStore) {
	store := &mockStore{}
	return NewHandler(store, &mockAuth{}, nil, 25<<20), store
}



func TestCreateDraft(t *testing.T) {
	h, store := newTestHandler()

	reqBody, _ := json.Marshal(map[string]any{
		"subject":    "Hello",
		"to":         []map[string]string{{"address": "bob@example.com"}},
		"html_body":  "<p>Hi</p>",
		"plain_text": "Hi",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", bytes.NewReader(reqBody))
	req = req.WithContext(api.WithUser(req.Context(), &models.User{
		ID:    uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Email: "alice@example.com",
	}))
	rr := httptest.NewRecorder()

	h.createDraft(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if len(store.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(store.messages))
	}
	msg := store.messages[0]
	if msg.Subject != "Hello" {
		t.Errorf("subject = %q, want Hello", msg.Subject)
	}
	if len(msg.ToAddresses) != 1 || msg.ToAddresses[0] != "bob@example.com" {
		t.Errorf("to = %v, want [bob@example.com]", msg.ToAddresses)
	}
	if !msg.IsDraft {
		t.Error("expected IsDraft=true")
	}
}

func TestBatchMessages_ReportsResults(t *testing.T) {
	h, store := newTestHandler()
	id1 := uuid.Must(uuid.NewV7())
	store.messages = append(store.messages, &models.Message{ID: id1, IsRead: false})

	body, _ := json.Marshal(map[string]any{
		"action":      "mark_read",
		"message_ids": []string{id1.String()},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/batch", bytes.NewReader(body))
	req = req.WithContext(api.WithUser(req.Context(), &models.User{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001")}))
	rr := httptest.NewRecorder()
	h.batchMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		Succeeded []string `json:"succeeded"`
		Failed    []struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		} `json:"failed"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Succeeded) != 1 || len(resp.Failed) != 0 {
		t.Fatalf("succeeded=%v failed=%v", resp.Succeeded, resp.Failed)
	}
}

func TestBatchMessages_UnknownActionRejected(t *testing.T) {
	h, _ := newTestHandler()
	body, _ := json.Marshal(map[string]any{
		"action":      "explode",
		"message_ids": []string{uuid.Must(uuid.NewV7()).String()},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/batch", bytes.NewReader(body))
	req = req.WithContext(api.WithUser(req.Context(), &models.User{ID: uuid.New()}))
	rr := httptest.NewRecorder()
	h.batchMessages(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestUpdateDraft(t *testing.T) {
	h, store := newTestHandler()
	draftID := uuid.Must(uuid.NewV7())
	store.messages = append(store.messages, &models.Message{
		ID:        draftID,
		Subject:   "Old",
		IsDraft:   true,
		Mailbox:   "DRAFTS",
		CreatedAt: time.Now().UTC(),
	})

	reqBody, _ := json.Marshal(map[string]any{
		"subject":    "New",
		"to":         []map[string]string{{"address": "charlie@example.com"}},
		"html_body":  "<b>New</b>",
		"plain_text": "New",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/drafts/"+draftID.String(), bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	// chi routing requires URL params to be set
	req = req.WithContext(api.WithUser(req.Context(), &models.User{
		ID:    uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Email: "alice@example.com",
	}))
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", draftID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	h.updateDraft(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if store.messages[0].Subject != "New" {
		t.Errorf("subject = %q, want New", store.messages[0].Subject)
	}
}
