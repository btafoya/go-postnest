package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/go-postnest/postnest/internal/postmark"
	"github.com/google/uuid"
)

// SendProcessor handles sending draft messages via Postmark.
type SendProcessor struct {
	store    mailstore.Store
	auth     *auth.Service
	postmark *postmark.Client
	logger   *slog.Logger
}

// NewSendProcessor creates a send processor.
func NewSendProcessor(store mailstore.Store, authSvc *auth.Service, pm *postmark.Client, logger *slog.Logger) *SendProcessor {
	return &SendProcessor{store: store, auth: authSvc, postmark: pm, logger: logger}
}

// Process sends a draft message.
func (p *SendProcessor) Process(ctx context.Context, job *Job) error {
	var payload struct {
		DraftID     string `json:"draft_id"`
		UserID      string `json:"user_id"`
		DomainID    string `json:"domain_id"`
		FromAddress string `json:"from_address"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	draftID, err := uuid.Parse(payload.DraftID)
	if err != nil {
		return fmt.Errorf("invalid draft_id: %w", err)
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}
	domainID, err := uuid.Parse(payload.DomainID)
	if err != nil {
		return fmt.Errorf("invalid domain_id: %w", err)
	}

	msg, err := p.store.GetMessage(ctx, domainID, userID, draftID)
	if err != nil {
		return fmt.Errorf("get draft: %w", err)
	}
	if !msg.IsDraft {
		return fmt.Errorf("message is not a draft")
	}

	domain, err := p.auth.GetDomainByID(ctx, domainID)
	if err != nil {
		return fmt.Errorf("get domain: %w", err)
	}

	pmMsg := &postmark.OutboundMessage{
		From:          payload.FromAddress,
		To:            msg.ToAddresses,
		Subject:       msg.Subject,
		TextBody:      msg.PlainText,
		HTMLBody:      msg.HTMLBody,
		MessageStream: domain.PostmarkStream,
	}

	atts, err := p.store.ListMessageAttachments(ctx, draftID)
	if err != nil {
		p.logger.Error("failed to list attachments", "error", err)
	} else {
		for _, a := range atts {
			pmMsg.Attachments = append(pmMsg.Attachments, postmark.Attachment{
				Name:        a.Filename,
				ContentType: a.ContentType,
				Content:     a.Data,
			})
		}
	}

	res, err := p.postmark.SendEmail(ctx, domain.PostmarkToken, pmMsg)
	if err != nil {
		return fmt.Errorf("postmark send: %w", err)
	}
	if res.ErrorCode != 0 {
		return fmt.Errorf("postmark error: %s", res.Message)
	}

	f := false
	sent := "SENT"
	tr := true
	patch := mailstore.MessagePatch{
		IsDraft:    &f,
		IsOutbound: &tr,
		Mailbox:    &sent,
	}
	if err := p.store.UpdateMessage(ctx, domainID, userID, draftID, patch); err != nil {
		p.logger.Error("failed to update sent draft", "error", err)
		return fmt.Errorf("update message: %w", err)
	}
	if _, _, err := p.store.GetOrCreateIMAPUID(ctx, draftID, userID, "SENT"); err != nil {
		p.logger.Warn("failed to assign IMAP UID for sent", "error", err, "message_id", draftID)
	}

	// Record delivery log for bounce/delivery webhook correlation.
	dl := &models.DeliveryLog{
		ID:                uuid.Must(uuid.NewV7()),
		MessageID:         draftID,
		DomainID:          domainID,
		Recipient:         msg.ToAddresses[0],
		Status:            "sent",
		PostmarkMessageID: res.MessageID,
		Details:           map[string]any{"postmark_response": res},
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	if len(msg.ToAddresses) > 1 {
		dl.Details["all_recipients"] = msg.ToAddresses
	}
	if err := p.store.CreateDeliveryLog(ctx, dl); err != nil {
		p.logger.Error("failed to create delivery log", "error", err)
	}

	p.logger.Info("draft sent", "draft_id", draftID, "postmark_id", res.MessageID)
	return nil
}
