package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
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

	p.logger.Info("draft sent", "draft_id", draftID, "postmark_id", res.MessageID)
	return nil
}
