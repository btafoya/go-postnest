package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-postnest/postnest/internal/db"
)

// DeliveryProcessor handles Postmark delivery webhooks.
type DeliveryProcessor struct {
	pool   *db.Pool
	logger *slog.Logger
}

// NewDeliveryProcessor creates a delivery event processor.
func NewDeliveryProcessor(pool *db.Pool, logger *slog.Logger) *DeliveryProcessor {
	return &DeliveryProcessor{pool: pool, logger: logger}
}

// Process handles a single delivery job.
func (p *DeliveryProcessor) Process(ctx context.Context, job *Job) error {
	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	postmarkMessageID, _ := payload["MessageID"].(string)
	if postmarkMessageID == "" {
		return fmt.Errorf("missing MessageID in delivery payload")
	}

	recipient, _ := payload["Recipient"].(string)

	// Update delivery log
	_, err := p.pool.Exec(ctx, `
		UPDATE delivery_logs
		SET status = 'delivered', updated_at = now(), details = details || $2::jsonb
		WHERE postmark_message_id = $1
	`, postmarkMessageID, fmt.Sprintf(`{"delivered_at":"%s"}`, time.Now().UTC().Format(time.RFC3339)))
	if err != nil {
		p.logger.Warn("failed to update delivery log", "postmark_id", postmarkMessageID, "error", err)
	}

	p.logger.Info("delivery processed", "postmark_id", postmarkMessageID, "recipient", recipient)
	return nil
}
