package workers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/models"
	"github.com/go-postnest/postnest/internal/postmark"
	"github.com/microcosm-cc/bluemonday"
	"github.com/go-postnest/postnest/internal/reputation"
	"github.com/google/uuid"
)

// InboundProcessor handles Postmark inbound mail webhooks.
type InboundProcessor struct {
	store    mailstore.Store
	auth     *auth.Service
	rep      *reputation.Engine
	logger   *slog.Logger
}

// NewInboundProcessor creates an inbound mail processor.
func NewInboundProcessor(store mailstore.Store, auth *auth.Service, rep *reputation.Engine, logger *slog.Logger) *InboundProcessor {
	return &InboundProcessor{store: store, auth: auth, rep: rep, logger: logger}
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

	recipient := in.OriginalRecipient
	if recipient == "" {
		recipient = in.To
	}
	if recipient == "" {
		return fmt.Errorf("no recipient in inbound payload")
	}

	// Resolve recipient: exact user, alias fan-out, or domain catch-all.
	targets, domain, err := p.auth.ResolveRecipients(ctx, recipient)
	if err != nil {
		return fmt.Errorf("resolve recipient %s: %w", recipient, err)
	}
	if len(targets) == 0 {
		return fmt.Errorf("no route for recipient: %s", recipient)
	}

	// Parse date
	msgDate := time.Now().UTC()
	if in.Date != "" {
		if d, err := time.Parse(time.RFC1123Z, in.Date); err == nil {
			msgDate = d.UTC()
		}
	}

	// Evaluate reputation/greylist
	if p.rep != nil {
		decision, err := p.rep.EvaluateInbound(ctx, domain.ID, in.From, recipient, net.ParseIP(in.From))
		if err != nil {
			p.logger.Warn("reputation evaluation failed", "error", err)
		} else {
			switch decision {
			case reputation.DecisionBlock:
				p.logger.Info("inbound blocked by blacklist", "from", in.From, "to", recipient)
				return fmt.Errorf("sender blacklisted: %s", in.From)
			case reputation.DecisionGreylist:
				p.logger.Info("inbound greylisted", "from", in.From, "to", recipient)
				if err := p.rep.RecordGreylist(ctx, domain.ID, in.From, recipient, net.ParseIP(in.From)); err != nil {
					p.logger.Warn("failed to record greylist", "error", err)
				}
				return fmt.Errorf("greylisted: retry later")
			}
		}
	}

	sanitizedHTML := bluemonday.UGCPolicy().Sanitize(in.HTMLBody)

	type decodedAttachment struct {
		name, contentType string
		data              []byte
	}
	var decoded []decodedAttachment
	for _, a := range in.Attachments {
		data, err := base64.StdEncoding.DecodeString(a.Content)
		if err != nil {
			p.logger.Warn("failed to decode attachment", "name", a.Name, "error", err)
			continue
		}
		decoded = append(decoded, decodedAttachment{name: a.Name, contentType: a.ContentType, data: data})
	}

	// Fan-out: deliver an independent copy to each resolved target user.
	for _, user := range targets {
		msg := &models.Message{
			ID:              uuid.Must(uuid.NewV7()),
			DomainID:        domain.ID,
			UserID:          user.ID,
			Mailbox:         "INBOX",
			MessageIDHeader: in.MessageID,
			Subject:         in.Subject,
			FromAddress:     in.From,
			FromName:        in.FromName,
			ToAddresses:     []string{recipient},
			Date:            msgDate,
			PlainText:       in.TextBody,
			HTMLBody:        sanitizedHTML,
			SizeBytes:       len(in.TextBody) + len(in.HTMLBody),
			IsOutbound:      false,
			IsRead:          false,
		}

		thread, err := p.store.FindOrCreateThread(ctx, domain.ID, user.ID, in.Subject, in.MessageID, "", nil)
		if err != nil {
			p.logger.Warn("thread find/create failed", "error", err, "user", user.ID)
		} else {
			msg.ThreadID = &thread.ID
		}

		var attachments []*models.Attachment
		for i, d := range decoded {
			att := &models.Attachment{
				ID:          uuid.Must(uuid.NewV7()),
				MessageID:   msg.ID,
				Filename:    d.name,
				ContentType: d.contentType,
				SizeBytes:   len(d.data),
				Data:        d.data,
			}
			if i < len(in.Attachments) {
				att.ContentID = in.Attachments[i].ContentID
			}
			attachments = append(attachments, att)
		}

		inboxLabel, err := p.store.GetLabelByName(ctx, domain.ID, user.ID, "INBOX")
		if err != nil {
			if err == mailstore.ErrNotFound {
				if seedErr := p.seedSystemLabels(ctx, domain.ID, user.ID); seedErr != nil {
					return fmt.Errorf("seed system labels for %s: %w", user.ID, seedErr)
				}
				inboxLabel, err = p.store.GetLabelByName(ctx, domain.ID, user.ID, "INBOX")
				if err != nil {
					return fmt.Errorf("inbox label lookup after seed for %s: %w", user.ID, err)
				}
			} else {
				return fmt.Errorf("inbox label lookup for %s: %w", user.ID, err)
			}
		}

		if err := p.store.CreateMessage(ctx, msg, []uuid.UUID{inboxLabel.ID}, attachments); err != nil {
			return fmt.Errorf("create message for %s: %w", user.ID, err)
		}

		if err := p.store.UpdateSearchVector(ctx, msg.ID); err != nil {
			p.logger.Warn("search vector update failed", "error", err, "message_id", msg.ID)
		}

		p.logger.Info("inbound message processed",
			"message_id", msg.ID,
			"from", in.From,
			"subject", in.Subject,
			"recipient", recipient,
			"target_user", user.ID,
		)
	}
	return nil
}

// seedSystemLabels creates default system labels for a user if they don't exist.
func (p *InboundProcessor) seedSystemLabels(ctx context.Context, domainID, userID uuid.UUID) error {
	names := []string{"INBOX", "SENT", "DRAFTS", "TRASH", "JUNK", "IMPORTANT", "STARRED", "ALL_MAIL"}
	for _, name := range names {
		label := &models.Label{
			ID:        uuid.Must(uuid.NewV7()),
			DomainID:  domainID,
			UserID:    userID,
			Name:      name,
			Color:     "#4285f4",
			IsSystem:  true,
			CreatedAt: time.Now().UTC(),
		}
		if err := p.store.CreateLabel(ctx, label); err != nil {
			// Ignore unique violations (label already exists from concurrent seeding).
			if !isUniqueViolation(err) {
				return err
			}
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// pgx unique-violation SQLSTATE is 23505
	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "unique violation")
}
