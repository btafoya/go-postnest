package workers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/go-postnest/postnest/internal/postmark"
	"github.com/google/uuid"
)

// InboundProcessor handles Postmark inbound mail webhooks.
type InboundProcessor struct {
	store  mailstore.Store
	auth   *auth.Service
	logger *slog.Logger
}

// NewInboundProcessor creates an inbound mail processor.
func NewInboundProcessor(store mailstore.Store, auth *auth.Service, logger *slog.Logger) *InboundProcessor {
	return &InboundProcessor{store: store, auth: auth, logger: logger}
}

// Process handles a single inbound mail job.
func (p *InboundProcessor) Process(ctx context.Context, job *Job) error {
	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	in, err := postmark.ParseInbound(payload)
	if err != nil {
		return fmt.Errorf("parse inbound payload: %w", err)
	}

	// Resolve recipient
	recipient := in.OriginalRecipient
	if recipient == "" {
		recipient = in.To
	}
	if recipient == "" {
		return fmt.Errorf("no recipient in inbound payload")
	}

	parts := strings.Split(recipient, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid recipient email: %s", recipient)
	}
	domainName := parts[1]

	// Lookup domain and user
	domain, err := p.auth.GetDomainByName(ctx, domainName)
	if err != nil {
		return fmt.Errorf("domain lookup: %w", err)
	}

	user, err := p.auth.GetUserByEmail(ctx, recipient)
	if err != nil {
		return fmt.Errorf("user lookup for %s: %w", recipient, err)
	}

	// Parse date
	msgDate := time.Now().UTC()
	if in.Date != "" {
		if d, err := time.Parse(time.RFC1123Z, in.Date); err == nil {
			msgDate = d.UTC()
		}
	}

	// Build message
	msg := &models.Message{
		ID:              uuid.Must(uuid.NewV7()),
		DomainID:        domain.ID,
		UserID:          user.ID,
		MessageIDHeader: in.MessageID,
		Subject:         in.Subject,
		FromAddress:     in.From,
		FromName:        in.FromName,
		ToAddresses:     []string{recipient},
		Date:            msgDate,
		PlainText:       in.TextBody,
		HTMLBody:        in.HTMLBody,
		SizeBytes:       len(in.TextBody) + len(in.HTMLBody),
		IsOutbound:      false,
		IsRead:          false,
	}

	// Find or create thread
	thread, err := p.store.FindOrCreateThread(ctx, domain.ID, user.ID, in.Subject, in.MessageID, "", nil)
	if err != nil {
		p.logger.Warn("thread find/create failed", "error", err)
	} else {
		msg.ThreadID = &thread.ID
	}

	// Build attachments
	var attachments []*models.Attachment
	for _, a := range in.Attachments {
		data, err := base64.StdEncoding.DecodeString(a.Content)
		if err != nil {
			p.logger.Warn("failed to decode attachment", "name", a.Name, "error", err)
			continue
		}
		attachments = append(attachments, &models.Attachment{
			ID:          uuid.Must(uuid.NewV7()),
			MessageID:   msg.ID,
			Filename:    a.Name,
			ContentType: a.ContentType,
			SizeBytes:   len(data),
			Data:        data,
		})
	}

	// Get INBOX label
	inboxLabel, err := p.store.GetLabelByName(ctx, domain.ID, user.ID, "INBOX")
	if err != nil {
		return fmt.Errorf("inbox label lookup: %w", err)
	}

	labelIDs := []uuid.UUID{inboxLabel.ID}
	if err := p.store.CreateMessage(ctx, msg, labelIDs, attachments); err != nil {
		return fmt.Errorf("create message: %w", err)
	}

	// Update search vector
	if err := p.store.UpdateSearchVector(ctx, msg.ID); err != nil {
		p.logger.Warn("search vector update failed", "error", err)
	}

	p.logger.Info("inbound message processed",
		"message_id", msg.ID,
		"from", in.From,
		"subject", in.Subject,
		"recipient", recipient,
	)
	return nil
}
