package search

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Indexer manages asynchronous full-text search indexing.
type Indexer struct {
	pool *pgxpool.Pool
}

// NewIndexer creates a search indexer.
func NewIndexer(pool *pgxpool.Pool) *Indexer {
	return &Indexer{pool: pool}
}

// Queue adds a message to the search index queue.
func (i *Indexer) Queue(ctx context.Context, messageID uuid.UUID) error {
	// In production, push to a Redis list; here simplified to direct update.
	_, err := i.pool.Exec(ctx, `
		UPDATE messages SET search_vector = (
			setweight(to_tsvector('english', coalesce(subject, '')), 'A') ||
			setweight(to_tsvector('english', coalesce(from_address, '')), 'B') ||
			setweight(to_tsvector('english', coalesce(from_name, '')), 'B') ||
			setweight(to_tsvector('english', coalesce(plain_text, '')), 'C') ||
			setweight(to_tsvector('simple', coalesce(to_addresses::text, '')), 'D')
		) WHERE id=$1
	`, messageID)
	return err
}

// ProcessQueue processes queued messages.
func (i *Indexer) ProcessQueue(ctx context.Context, batchSize int) (int, error) {
	// Simplified: no-op because Queue updates directly.
	return 0, nil
}
