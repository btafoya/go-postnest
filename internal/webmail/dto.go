package webmail

import (
	"net/mail"
	"time"

	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/models"
)

// addrDTO is the nested recipient shape the frontend expects.
type addrDTO struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// messageDTO is the JSON contract consumed by the React frontend.
type messageDTO struct {
	ID        uuid.UUID  `json:"id"`
	ThreadID  *uuid.UUID `json:"thread_id"`
	Subject   string     `json:"subject"`
	Snippet   string     `json:"snippet"`
	From      addrDTO    `json:"from"`
	To        []addrDTO  `json:"to"`
	Cc        []addrDTO  `json:"cc"`
	Bcc       []addrDTO  `json:"bcc"`
	Date      time.Time  `json:"date"`
	PlainText string     `json:"plain_text"`
	HTMLBody  string     `json:"html_body"`
	Labels    []string   `json:"labels"`
	IsDraft   bool       `json:"is_draft"`
	IsRead    bool       `json:"is_read"`
	IsFlagged bool       `json:"is_flagged"`
	Mailbox   string     `json:"mailbox"`
}

func parseAddr(s string) addrDTO {
	if a, err := mail.ParseAddress(s); err == nil {
		return addrDTO{Name: a.Name, Email: a.Address}
	}
	return addrDTO{Email: s}
}

func parseAddrs(list []string) []addrDTO {
	out := make([]addrDTO, 0, len(list))
	for _, s := range list {
		out = append(out, parseAddr(s))
	}
	return out
}

func snippet(m *models.Message) string {
	s := m.PlainText
	if s == "" {
		s = m.HTMLBody
	}
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

func toMessageDTO(m *models.Message, labels []string) messageDTO {
	from := addrDTO{Email: m.FromAddress}
	if m.FromName != "" {
		from.Name = m.FromName
	} else {
		from = parseAddr(m.FromAddress)
	}
	if labels == nil {
		labels = []string{}
	}
	return messageDTO{
		ID:        m.ID,
		ThreadID:  m.ThreadID,
		Subject:   m.Subject,
		Snippet:   snippet(m),
		From:      from,
		To:        parseAddrs(m.ToAddresses),
		Cc:        parseAddrs(m.CcAddresses),
		Bcc:       parseAddrs(m.BccAddresses),
		Date:      m.Date,
		PlainText: m.PlainText,
		HTMLBody:  m.HTMLBody,
		Labels:    labels,
		IsDraft:   m.IsDraft,
		IsRead:    m.IsRead,
		IsFlagged: m.IsFlagged,
		Mailbox:   m.Mailbox,
	}
}

func toMessageDTOs(msgs []*models.Message) []messageDTO {
	out := make([]messageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessageDTO(m, nil))
	}
	return out
}
