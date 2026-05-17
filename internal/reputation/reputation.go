package reputation

import (
	"context"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Decision is the spam evaluation result.
type Decision string

const (
	DecisionPass     Decision = "pass"
	DecisionGreylist Decision = "greylist"
	DecisionBlock    Decision = "block"
	DecisionJunk     Decision = "junk"
)

// Engine evaluates spam rules and reputation.
type Engine struct {
	pool *pgxpool.Pool
}

// NewEngine creates a reputation engine.
func NewEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{pool: pool}
}

// EvaluateInbound checks whitelist/blacklist/greylist rules.
func (e *Engine) EvaluateInbound(ctx context.Context, domainID uuid.UUID, from, to string, senderIP net.IP) (Decision, error) {
	// Check whitelist
	var wl int
	_ = e.pool.QueryRow(ctx, `SELECT 1 FROM whitelist WHERE domain_id=$1 AND (value=$2 OR value=$3)`, domainID, from, domainFromEmail(from)).Scan(&wl)
	if wl == 1 {
		return DecisionPass, nil
	}
	// Check blacklist
	var bl int
	_ = e.pool.QueryRow(ctx, `SELECT 1 FROM blacklist WHERE domain_id=$1 AND (value=$2 OR value=$3)`, domainID, from, domainFromEmail(from)).Scan(&bl)
	if bl == 1 {
		return DecisionBlock, nil
	}
	// Check greylist
	var gl int
	_ = e.pool.QueryRow(ctx, `SELECT 1 FROM greylist WHERE domain_id=$1 AND sender_email=$2 AND recipient_email=$3 AND passed_at IS NULL`, domainID, from, to).Scan(&gl)
	if gl == 1 {
		return DecisionGreylist, nil
	}
	return DecisionPass, nil
}

// RecordGreylist inserts a greylist triplet record.
func (e *Engine) RecordGreylist(ctx context.Context, domainID uuid.UUID, senderEmail, recipientEmail string, senderIP net.IP) error {
	_, err := e.pool.Exec(ctx, `
		INSERT INTO greylist (id, domain_id, sender_email, sender_ip, recipient_email, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, now())
		ON CONFLICT (domain_id, sender_email, sender_ip, recipient_email) DO NOTHING
	`, domainID, senderEmail, senderIP.String(), recipientEmail)
	return err
}

// UpdateReputation records an event for a contact.
func (e *Engine) UpdateReputation(ctx context.Context, domainID uuid.UUID, email string, eventType string) error {
	// Ensure contact exists
	var contactID uuid.UUID
	err := e.pool.QueryRow(ctx, `
		SELECT id FROM contacts WHERE domain_id=$1 AND email=$2 LIMIT 1
	`, domainID, email).Scan(&contactID)
	if err == pgx.ErrNoRows {
		return nil // no contact to update
	}
	if err != nil {
		return err
	}

	col := "received_count"
	switch eventType {
	case "sent":
		col = "sent_count"
	case "bounced":
		col = "bounce_count"
	case "complained":
		col = "complaint_count"
	}

	_, err = e.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO contact_reputation (contact_id, domain_id, %s, last_interaction_at, updated_at)
		VALUES ($1, $2, 1, now(), now())
		ON CONFLICT (contact_id) DO UPDATE SET
			%s = contact_reputation.%s + 1,
			last_interaction_at = now(),
			updated_at = now()
	`, col, col, col), contactID, domainID)
	return err
}

func domainFromEmail(email string) string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return email[i+1:]
		}
	}
	return ""
}
