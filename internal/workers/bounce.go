package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-postnest/postnest/internal/db"
	"github.com/google/uuid"
)

// BounceProcessor handles Postmark bounce webhooks.
type BounceProcessor struct {
	pool   *db.Pool
	logger *slog.Logger
}

// NewBounceProcessor creates a bounce event processor.
func NewBounceProcessor(pool *db.Pool, logger *slog.Logger) *BounceProcessor {
	return &BounceProcessor{pool: pool, logger: logger}
}

// Process handles a single bounce job.
func (p *BounceProcessor) Process(ctx context.Context, job *Job) error {
	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	postmarkMessageID, _ := payload["MessageID"].(string)
	if postmarkMessageID == "" {
		return fmt.Errorf("missing MessageID in bounce payload")
	}

	bounceType, _ := payload["Type"].(string)
	bounceDescription, _ := payload["Description"].(string)
	diagnosticCode, _ := payload["Details"].(string)

	// Update delivery log
	_, err := p.pool.Exec(ctx, `
		UPDATE delivery_logs
		SET status = 'bounced', updated_at = now(), details = details || $2::jsonb
		WHERE postmark_message_id = $1
	`, postmarkMessageID, fmt.Sprintf(`{"bounce_type":"%s","description":"%s","diagnostic":"%s"}`, bounceType, bounceDescription, diagnosticCode))
	if err != nil {
		p.logger.Warn("failed to update delivery log for bounce", "postmark_id", postmarkMessageID, "error", err)
	}

	// Create bounce event
	var deliveryLogID uuid.UUID
	var domainID uuid.UUID
	row := p.pool.QueryRow(ctx, `SELECT id, domain_id FROM delivery_logs WHERE postmark_message_id=$1 LIMIT 1`, postmarkMessageID)
	if err := row.Scan(&deliveryLogID, &domainID); err == nil {
		_, _ = p.pool.Exec(ctx, `
			INSERT INTO bounce_events (id, delivery_log_id, domain_id, bounce_type, bounce_description, diagnostic_code, created_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now())
		`, deliveryLogID, domainID, bounceType, bounceDescription, diagnosticCode)
	}

	p.logger.Info("bounce processed", "postmark_id", postmarkMessageID, "type", bounceType)
	return nil
}
