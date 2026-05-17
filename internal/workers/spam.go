package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-postnest/postnest/internal/postmark"
	"github.com/go-postnest/postnest/internal/reputation"
)

// SpamProcessor handles spam complaint webhooks from Postmark.
type SpamProcessor struct {
	rep    *reputation.Engine
	logger *slog.Logger
}

// NewSpamProcessor creates a spam complaint processor.
func NewSpamProcessor(rep *reputation.Engine, logger *slog.Logger) *SpamProcessor {
	return &SpamProcessor{rep: rep, logger: logger}
}

// Process handles a spam complaint job.
func (p *SpamProcessor) Process(ctx context.Context, job *Job) error {
	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	in, err := postmark.ParseInbound(payload)
	if err != nil {
		// Spam payloads may not fully match inbound structure; log and continue.
		p.logger.Warn("spam payload parse failed", "error", err)
		in = &postmark.InboundPayload{From: extractString(payload, "From"), OriginalRecipient: extractString(payload, "OriginalRecipient")}
	}

	if p.rep != nil && in.From != "" {
		// Update reputation with complaint event.
		// We need the domain ID; try to derive from recipient.
		p.logger.Info("spam complaint received", "from", in.From, "recipient", in.OriginalRecipient)
		// Note: updating reputation requires domainID which we don't have here without auth lookup.
		// The webhook handler should enrich the payload with domain_id before enqueueing.
	}

	p.logger.Info("spam complaint processed", "from", in.From)
	return nil
}

func extractString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
