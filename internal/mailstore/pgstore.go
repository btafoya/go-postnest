package mailstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/go-postnest/postnest/internal/models"
)

// PGStore implements Store using PostgreSQL.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore creates a PostgreSQL-backed mail store.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if msg.ID == uuid.Nil {
		msg.ID = uuid.Must(uuid.NewV7())
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	msg.UpdatedAt = msg.CreatedAt

	_, err = tx.Exec(ctx, `
		INSERT INTO messages (
			id, domain_id, user_id, thread_id, postmark_message_id, mailbox,
			message_id_header, in_reply_to, references, subject,
			from_address, from_name, to_addresses, cc_addresses, bcc_addresses, reply_to,
			date, plain_text, html_body, source, size_bytes,
			is_draft, is_outbound, is_read, is_flagged, is_answered, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)
	`,
		msg.ID, msg.DomainID, msg.UserID, msg.ThreadID, msg.PostmarkMessageID, msg.Mailbox,
		msg.MessageIDHeader, msg.InReplyTo, msg.References, msg.Subject,
		msg.FromAddress, msg.FromName, msg.ToAddresses, msg.CcAddresses, msg.BccAddresses, msg.ReplyTo,
		msg.Date, msg.PlainText, msg.HTMLBody, msg.Source, msg.SizeBytes,
		msg.IsDraft, msg.IsOutbound, msg.IsRead, msg.IsFlagged, msg.IsAnswered, msg.CreatedAt, msg.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	for _, lid := range labelIDs {
		_, err = tx.Exec(ctx, `INSERT INTO message_labels (message_id, label_id) VALUES ($1,$2)`, msg.ID, lid)
		if err != nil {
			return fmt.Errorf("insert message_label: %w", err)
		}
	}

	for _, att := range attachments {
		att.MessageID = msg.ID
		if att.ID == uuid.Nil {
			att.ID = uuid.Must(uuid.NewV7())
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO attachments (id, message_id, filename, content_type, size_bytes, data, content_id, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, att.ID, att.MessageID, att.Filename, att.ContentType, att.SizeBytes, att.Data, att.ContentID, msg.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert attachment: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (s *PGStore) GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*models.Message, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, domain_id, user_id, thread_id, postmark_message_id, mailbox,
			message_id_header, in_reply_to, references, subject,
			from_address, from_name, to_addresses, cc_addresses, bcc_addresses, reply_to,
			date, plain_text, html_body, source, size_bytes,
			is_draft, is_outbound, is_read, is_flagged, is_answered, created_at, updated_at
		FROM messages WHERE id=$1 AND domain_id=$2 AND user_id=$3
	`, messageID, domainID, userID)

	var m models.Message
	var threadID *uuid.UUID
	err := row.Scan(
		&m.ID, &m.DomainID, &m.UserID, &threadID, &m.PostmarkMessageID, &m.Mailbox,
		&m.MessageIDHeader, &m.InReplyTo, &m.References, &m.Subject,
		&m.FromAddress, &m.FromName, &m.ToAddresses, &m.CcAddresses, &m.BccAddresses, &m.ReplyTo,
		&m.Date, &m.PlainText, &m.HTMLBody, &m.Source, &m.SizeBytes,
		&m.IsDraft, &m.IsOutbound, &m.IsRead, &m.IsFlagged, &m.IsAnswered, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.ThreadID = threadID
	return &m, nil
}

func (s *PGStore) ListMessages(ctx context.Context, domainID, userID uuid.UUID, labelID *uuid.UUID, opts ListOptions) ([]*models.Message, int64, error) {
	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id
		WHERE m.domain_id=$1 AND m.user_id=$2 AND ($3::uuid IS NULL OR ml.label_id=$3)
	`, domainID, userID, labelID).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	// Build ORDER BY clause safely via whitelist to avoid fmt.Sprintf injection.
	var orderBy string
	switch opts.SortField {
	case "date":
		orderBy = "m.date"
	case "subject":
		orderBy = "m.subject"
	case "from":
		orderBy = "m.from_address"
	case "size":
		orderBy = "m.size_bytes"
	default:
		orderBy = "m.created_at"
	}
	if opts.SortDesc {
		orderBy += " DESC"
	} else {
		orderBy += " ASC"
	}

	query := `
		SELECT m.id, m.domain_id, m.user_id, m.thread_id, m.postmark_message_id, m.mailbox,
			m.message_id_header, m.in_reply_to, m.references, m.subject,
			m.from_address, m.from_name, m.to_addresses, m.cc_addresses, m.bcc_addresses, m.reply_to,
			m.date, m.plain_text, m.html_body, m.source, m.size_bytes,
			m.is_draft, m.is_outbound, m.is_read, m.is_flagged, m.is_answered, m.created_at, m.updated_at
		FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id
		WHERE m.domain_id=$1 AND m.user_id=$2 AND ($3::uuid IS NULL OR ml.label_id=$3)
		ORDER BY ` + orderBy + `
		LIMIT $4 OFFSET $5
	`

	rows, err := s.pool.Query(ctx, query, domainID, userID, labelID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*models.Message
	for rows.Next() {
		var m models.Message
		var threadID *uuid.UUID
		if err := rows.Scan(
			&m.ID, &m.DomainID, &m.UserID, &threadID, &m.PostmarkMessageID, &m.Mailbox,
			&m.MessageIDHeader, &m.InReplyTo, &m.References, &m.Subject,
			&m.FromAddress, &m.FromName, &m.ToAddresses, &m.CcAddresses, &m.BccAddresses, &m.ReplyTo,
			&m.Date, &m.PlainText, &m.HTMLBody, &m.Source, &m.SizeBytes,
			&m.IsDraft, &m.IsOutbound, &m.IsRead, &m.IsFlagged, &m.IsAnswered, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		m.ThreadID = threadID
		out = append(out, &m)
	}
	return out, total, rows.Err()
}

func (s *PGStore) UpdateMessage(ctx context.Context, domainID, userID, messageID uuid.UUID, patch MessagePatch) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE messages SET
			is_read = coalesce($4, is_read),
			is_flagged = coalesce($5, is_flagged),
			is_answered = coalesce($6, is_answered),
			is_draft = coalesce($7, is_draft),
			mailbox = coalesce($8, mailbox),
			is_outbound = coalesce($9, is_outbound),
			subject = coalesce($10, subject),
			html_body = coalesce($11, html_body),
			plain_text = coalesce($12, plain_text),
			to_addresses = coalesce($13, to_addresses),
			updated_at = now()
		WHERE id=$1 AND domain_id=$2 AND user_id=$3
	`, messageID, domainID, userID, patch.IsRead, patch.IsFlagged, patch.IsAnswered, patch.IsDraft, patch.Mailbox, patch.IsOutbound, patch.Subject, patch.HTMLBody, patch.PlainText, patch.ToAddresses)
	return err
}

func (s *PGStore) DeleteMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM messages WHERE id=$1 AND domain_id=$2 AND user_id=$3`, messageID, domainID, userID)
	return err
}

func (s *PGStore) MoveToMailbox(ctx context.Context, domainID, userID, messageID uuid.UUID, mailbox string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE messages SET mailbox=$4, updated_at=now()
		WHERE id=$1 AND domain_id=$2 AND user_id=$3
	`, messageID, domainID, userID, mailbox)
	return err
}

func (s *PGStore) CreateLabel(ctx context.Context, label *models.Label) error {
	if label.ID == uuid.Nil {
		label.ID = uuid.Must(uuid.NewV7())
	}
	label.CreatedAt = time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO labels (id, domain_id, user_id, name, color, is_system, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, label.ID, label.DomainID, label.UserID, label.Name, label.Color, label.IsSystem, label.CreatedAt)
	return err
}

func (s *PGStore) GetLabels(ctx context.Context, domainID, userID uuid.UUID) ([]*models.Label, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, domain_id, user_id, name, color, is_system, created_at FROM labels
		WHERE domain_id=$1 AND user_id=$2 ORDER BY is_system DESC, name ASC
	`, domainID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.Label
	for rows.Next() {
		var l models.Label
		if err := rows.Scan(&l.ID, &l.DomainID, &l.UserID, &l.Name, &l.Color, &l.IsSystem, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *PGStore) GetLabelByName(ctx context.Context, domainID, userID uuid.UUID, name string) (*models.Label, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, domain_id, user_id, name, color, is_system, created_at FROM labels
		WHERE domain_id=$1 AND user_id=$2 AND name=$3
	`, domainID, userID, name)
	var l models.Label
	if err := row.Scan(&l.ID, &l.DomainID, &l.UserID, &l.Name, &l.Color, &l.IsSystem, &l.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}

func (s *PGStore) DeleteLabel(ctx context.Context, domainID, userID, labelID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM labels WHERE id=$1 AND domain_id=$2 AND user_id=$3 AND is_system=false
	`, labelID, domainID, userID)
	return err
}

func (s *PGStore) UpdateLabel(ctx context.Context, domainID, userID, labelID uuid.UUID, name, color string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE labels SET name=$4, color=$5 WHERE id=$1 AND domain_id=$2 AND user_id=$3 AND is_system=false
	`, labelID, domainID, userID, name, color)
	return err
}

func (s *PGStore) ApplyLabels(ctx context.Context, messageID uuid.UUID, addLabelIDs, removeLabelIDs []uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, lid := range removeLabelIDs {
		_, err = tx.Exec(ctx, `DELETE FROM message_labels WHERE message_id=$1 AND label_id=$2`, messageID, lid)
		if err != nil {
			return err
		}
	}
	for _, lid := range addLabelIDs {
		_, err = tx.Exec(ctx, `INSERT INTO message_labels (message_id, label_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, messageID, lid)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PGStore) GetMessageLabels(ctx context.Context, messageID uuid.UUID) ([]*models.Label, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.domain_id, l.user_id, l.name, l.color, l.is_system, l.created_at
		FROM labels l
		JOIN message_labels ml ON ml.label_id = l.id
		WHERE ml.message_id=$1 ORDER BY l.name
	`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.Label
	for rows.Next() {
		var l models.Label
		if err := rows.Scan(&l.ID, &l.DomainID, &l.UserID, &l.Name, &l.Color, &l.IsSystem, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *PGStore) GetThread(ctx context.Context, domainID, userID, threadID uuid.UUID) (*models.Thread, []*models.Message, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, domain_id, user_id, subject_hash, message_ids, created_at, updated_at
		FROM threads WHERE id=$1 AND domain_id=$2 AND user_id=$3
	`, threadID, domainID, userID)
	var t models.Thread
	if err := row.Scan(&t.ID, &t.DomainID, &t.UserID, &t.SubjectHash, &t.MessageIDs, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	msgs, _, err := s.ListMessages(ctx, domainID, userID, nil, ListOptions{Limit: 1000, SortField: "date", SortDesc: true})
	return &t, msgs, err
}

func (s *PGStore) FindOrCreateThread(ctx context.Context, domainID, userID uuid.UUID, subject, messageID, inReplyTo string, references []string) (*models.Thread, error) {
	// Use a transaction with SELECT FOR UPDATE to prevent race conditions.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var threadID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT t.id FROM threads t
		WHERE t.domain_id=$1 AND t.user_id=$2 AND (
			$3 = ANY(t.message_ids) OR t.subject_hash=$4
		)
		LIMIT 1
		FOR UPDATE
	`, domainID, userID, inReplyTo, subject).Scan(&threadID)
	if err == nil {
		_, _ = tx.Exec(ctx, `
			UPDATE threads SET message_ids=array_append(message_ids,$3), updated_at=now()
			WHERE id=$1 AND domain_id=$2
		`, threadID, domainID, messageID)
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &models.Thread{ID: threadID, DomainID: domainID, UserID: userID}, nil
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}

	// create new thread with ON CONFLICT protection
	threadID = uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	res, err := tx.Exec(ctx, `
		INSERT INTO threads (id, domain_id, user_id, subject_hash, message_ids, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$6)
		ON CONFLICT (domain_id, user_id, subject_hash) DO NOTHING
	`, threadID, domainID, userID, subject, []string{messageID}, now)
	if err != nil {
		return nil, err
	}
	if res.RowsAffected() == 0 {
		// Conflict: another transaction created the thread. Select it.
		err = tx.QueryRow(ctx, `
			SELECT id FROM threads
			WHERE domain_id=$1 AND user_id=$2 AND subject_hash=$3
		`, domainID, userID, subject).Scan(&threadID)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &models.Thread{ID: threadID, DomainID: domainID, UserID: userID}, nil
}

func (s *PGStore) CreateAttachments(ctx context.Context, attachments []*models.Attachment) error {
	if len(attachments) == 0 {
		return nil
	}
	for _, att := range attachments {
		if att.ID == uuid.Nil {
			att.ID = uuid.Must(uuid.NewV7())
		}
	}
	now := time.Now().UTC()
	cols := []string{"id", "message_id", "filename", "content_type", "size_bytes", "data", "content_id", "created_at"}
	_, err := s.pool.CopyFrom(ctx, pgx.Identifier{"attachments"}, cols,
		pgx.CopyFromSlice(len(attachments), func(i int) ([]any, error) {
			att := attachments[i]
			return []any{att.ID, att.MessageID, att.Filename, att.ContentType, att.SizeBytes, att.Data, att.ContentID, now}, nil
		}))
	return err
}

func (s *PGStore) GetAttachment(ctx context.Context, attachmentID uuid.UUID) (*models.Attachment, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, message_id, filename, content_type, size_bytes, data, content_id, created_at
		FROM attachments WHERE id=$1
	`, attachmentID)
	var a models.Attachment
	if err := row.Scan(&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.Data, &a.ContentID, &a.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (s *PGStore) SetFlag(ctx context.Context, messageID uuid.UUID, flag string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO message_flags (message_id, flag) VALUES ($1,$2) ON CONFLICT DO NOTHING
	`, messageID, flag)
	return err
}

func (s *PGStore) ClearFlag(ctx context.Context, messageID uuid.UUID, flag string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM message_flags WHERE message_id=$1 AND flag=$2`, messageID, flag)
	return err
}

func (s *PGStore) GetFlags(ctx context.Context, messageID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT flag FROM message_flags WHERE message_id=$1`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *PGStore) GetFlagsBatch(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := make(map[uuid.UUID][]string, len(messageIDs))
	if len(messageIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT message_id, flag FROM message_flags WHERE message_id = ANY($1)`, messageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var f string
		if err := rows.Scan(&id, &f); err != nil {
			return nil, err
		}
		out[id] = append(out[id], f)
	}
	return out, rows.Err()
}

func (s *PGStore) Search(ctx context.Context, domainID, userID uuid.UUID, query string, opts SearchOptions) ([]*models.Message, int64, error) {
	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages
		WHERE domain_id=$1 AND user_id=$2 AND search_vector @@ plainto_tsquery('english',$3)
	`, domainID, userID, query).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, domain_id, user_id, thread_id, postmark_message_id, mailbox,
			message_id_header, in_reply_to, references, subject,
			from_address, from_name, to_addresses, cc_addresses, bcc_addresses, reply_to,
			date, plain_text, html_body, source, size_bytes,
			is_draft, is_outbound, is_read, is_flagged, is_answered, created_at, updated_at
		FROM messages
		WHERE domain_id=$1 AND user_id=$2 AND search_vector @@ plainto_tsquery('english',$3)
		ORDER BY ts_rank_cd(search_vector, plainto_tsquery('english',$3), 32) DESC, date DESC
		LIMIT $4 OFFSET $5
	`, domainID, userID, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*models.Message
	for rows.Next() {
		var m models.Message
		var threadID *uuid.UUID
		if err := rows.Scan(
			&m.ID, &m.DomainID, &m.UserID, &threadID, &m.PostmarkMessageID, &m.Mailbox,
			&m.MessageIDHeader, &m.InReplyTo, &m.References, &m.Subject,
			&m.FromAddress, &m.FromName, &m.ToAddresses, &m.CcAddresses, &m.BccAddresses, &m.ReplyTo,
			&m.Date, &m.PlainText, &m.HTMLBody, &m.Source, &m.SizeBytes,
			&m.IsDraft, &m.IsOutbound, &m.IsRead, &m.IsFlagged, &m.IsAnswered, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		m.ThreadID = threadID
		out = append(out, &m)
	}
	return out, total, rows.Err()
}

func (s *PGStore) UpdateSearchVector(ctx context.Context, messageID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE messages SET
			search_vector =
				setweight(to_tsvector('english', coalesce(subject, '')), 'A') ||
				setweight(to_tsvector('english', coalesce(from_address, '')), 'B') ||
				setweight(to_tsvector('english', coalesce(from_name, '')), 'B') ||
				setweight(to_tsvector('english', coalesce(plain_text, '')), 'C') ||
				setweight(to_tsvector('simple', coalesce(to_addresses::text, '')), 'D')
		WHERE id=$1
	`, messageID)
	return err
}

func (s *PGStore) CountUnreadByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id
		WHERE m.domain_id=$1 AND m.user_id=$2 AND ml.label_id=$3 AND m.is_read=false
	`, domainID, userID, labelID).Scan(&count)
	return count, err
}

func (s *PGStore) CountTotalByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages m
		JOIN message_labels ml ON ml.message_id = m.id
		WHERE m.domain_id=$1 AND m.user_id=$2 AND ml.label_id=$3
	`, domainID, userID, labelID).Scan(&count)
	return count, err
}

func (s *PGStore) CreateDeliveryLog(ctx context.Context, log *models.DeliveryLog) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO delivery_logs (id, message_id, domain_id, recipient, status, postmark_message_id, details, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, log.ID, log.MessageID, log.DomainID, log.Recipient, log.Status, log.PostmarkMessageID, log.Details, log.CreatedAt, log.UpdatedAt)
	return err
}

// ErrNotFound indicates the requested resource does not exist.
var ErrNotFound = fmt.Errorf("not found")
