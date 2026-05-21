package webmail

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/models"
)

func TestToMessageDTO_NestedAndSnakeCase(t *testing.T) {
	m := &models.Message{
		ID:           uuid.Must(uuid.NewV7()),
		Subject:      "Hi",
		FromAddress:  "alice@example.com",
		ToAddresses:  []string{"Bob <bob@example.com>", "carol@example.com"},
		CcAddresses:  []string{"cc@example.com"},
		PlainText:    "body text",
		HTMLBody:     "<p>body</p>",
		IsRead:       true,
		IsFlagged:    false,
		IsDraft:      true,
		Mailbox:      "DRAFTS",
		Date:         time.Now(),
	}
	dto := toMessageDTO(m, []string{"INBOX"}, nil)

	if dto.From.Email != "alice@example.com" {
		t.Errorf("from.email = %q", dto.From.Email)
	}
	if len(dto.To) != 2 || dto.To[0].Name != "Bob" || dto.To[0].Email != "bob@example.com" {
		t.Errorf("to = %+v", dto.To)
	}
	if dto.To[1].Email != "carol@example.com" {
		t.Errorf("to[1] = %+v", dto.To[1])
	}
	if len(dto.Cc) != 1 || dto.Cc[0].Email != "cc@example.com" {
		t.Errorf("cc = %+v", dto.Cc)
	}
	if !dto.IsRead || dto.IsFlagged || !dto.IsDraft {
		t.Errorf("flags wrong: %+v", dto)
	}
	if dto.Snippet != "body text" {
		t.Errorf("snippet = %q", dto.Snippet)
	}
	if len(dto.Labels) != 1 || dto.Labels[0] != "INBOX" {
		t.Errorf("labels = %v", dto.Labels)
	}
}

func TestToMessageDTO_NilLabelsIsEmptySlice(t *testing.T) {
	dto := toMessageDTO(&models.Message{FromAddress: "x@y.com"}, nil, nil)
	if dto.Labels == nil {
		t.Fatal("labels must serialize as [] not null")
	}
	if len(dto.To) != 0 {
		t.Fatalf("empty to should be empty slice, got %v", dto.To)
	}
}

func TestSnippetTruncates(t *testing.T) {
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	m := &models.Message{PlainText: string(long)}
	if len(snippet(m)) != 160 {
		t.Fatalf("snippet len = %d, want 160", len(snippet(m)))
	}
}
