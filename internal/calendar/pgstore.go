package calendar

import (
	"context"
	"encoding/json"
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

// NewPGStore creates a PostgreSQL-backed calendar store.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) ListCalendars(ctx context.Context, domainID, userID uuid.UUID) ([]*models.Calendar, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, domain_id, user_id, name, color, description, ctag, created_at, updated_at
		FROM calendars WHERE domain_id=$1 AND user_id=$2 ORDER BY name ASC
	`, domainID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.Calendar
	for rows.Next() {
		var c models.Calendar
		if err := rows.Scan(&c.ID, &c.DomainID, &c.UserID, &c.Name, &c.Color, &c.Description, &c.CTag, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (s *PGStore) GetCalendar(ctx context.Context, domainID, userID, calID uuid.UUID) (*models.Calendar, error) {
	var c models.Calendar
	err := s.pool.QueryRow(ctx, `
		SELECT id, domain_id, user_id, name, color, description, ctag, created_at, updated_at
		FROM calendars WHERE id=$1 AND domain_id=$2 AND user_id=$3
	`, calID, domainID, userID).Scan(&c.ID, &c.DomainID, &c.UserID, &c.Name, &c.Color, &c.Description, &c.CTag, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (s *PGStore) CreateCalendar(ctx context.Context, cal *models.Calendar) error {
	if cal.ID == uuid.Nil {
		cal.ID = uuid.Must(uuid.NewV7())
	}
	if cal.Color == "" {
		cal.Color = "#4285f4"
	}
	now := time.Now().UTC()
	cal.CreatedAt = now
	cal.UpdatedAt = now
	cal.CTag = 1
	_, err := s.pool.Exec(ctx, `
		INSERT INTO calendars (id, domain_id, user_id, name, color, description, ctag, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
	`, cal.ID, cal.DomainID, cal.UserID, cal.Name, cal.Color, cal.Description, cal.CTag, now)
	return err
}

func (s *PGStore) DeleteCalendar(ctx context.Context, domainID, userID, calID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM calendars WHERE id=$1 AND domain_id=$2 AND user_id=$3`, calID, domainID, userID)
	return err
}

func (s *PGStore) ListEvents(ctx context.Context, calID uuid.UUID, from, to time.Time) ([]*models.CalendarEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, calendar_id, domain_id, user_id, uid, summary, description, location,
			starts_at, ends_at, all_day, rrule, status, organizer, attendees, sequence, etag, created_at, updated_at
		FROM calendar_events
		WHERE calendar_id=$1
			AND ($2::timestamptz IS NULL OR ends_at >= $2)
			AND ($3::timestamptz IS NULL OR starts_at <= $3)
		ORDER BY starts_at ASC
	`, calID, nullTime(from), nullTime(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.CalendarEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *PGStore) GetEvent(ctx context.Context, calID uuid.UUID, uid string) (*models.CalendarEvent, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, calendar_id, domain_id, user_id, uid, summary, description, location,
			starts_at, ends_at, all_day, rrule, status, organizer, attendees, sequence, etag, created_at, updated_at
		FROM calendar_events WHERE calendar_id=$1 AND uid=$2
	`, calID, uid)
	ev, err := scanEvent(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return ev, nil
}

func (s *PGStore) PutEvent(ctx context.Context, ev *models.CalendarEvent) error {
	if ev.ID == uuid.Nil {
		ev.ID = uuid.Must(uuid.NewV7())
	}
	if ev.Status == "" {
		ev.Status = "CONFIRMED"
	}
	now := time.Now().UTC()
	if ev.Attendees == nil {
		ev.Attendees = []string{}
	}
	attendeesJSON, err := json.Marshal(ev.Attendees)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO calendar_events (
			id, calendar_id, domain_id, user_id, uid, summary, description, location,
			starts_at, ends_at, all_day, rrule, status, organizer, attendees, sequence, etag, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$18)
		ON CONFLICT (calendar_id, uid) DO UPDATE SET
			summary=EXCLUDED.summary, description=EXCLUDED.description, location=EXCLUDED.location,
			starts_at=EXCLUDED.starts_at, ends_at=EXCLUDED.ends_at, all_day=EXCLUDED.all_day,
			rrule=EXCLUDED.rrule, status=EXCLUDED.status, organizer=EXCLUDED.organizer,
			attendees=EXCLUDED.attendees, sequence=EXCLUDED.sequence, etag=EXCLUDED.etag,
			updated_at=EXCLUDED.updated_at
	`, ev.ID, ev.CalendarID, ev.DomainID, ev.UserID, ev.UID, ev.Summary, ev.Description, ev.Location,
		ev.StartsAt, ev.EndsAt, ev.AllDay, ev.RRule, ev.Status, ev.Organizer, attendeesJSON, ev.Sequence, ev.ETag, now)
	return err
}

func (s *PGStore) DeleteEvent(ctx context.Context, calID uuid.UUID, uid string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM calendar_events WHERE calendar_id=$1 AND uid=$2`, calID, uid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) BumpCTag(ctx context.Context, calID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE calendars SET ctag=ctag+1, updated_at=now() WHERE id=$1`, calID)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanEvent(row scannable) (*models.CalendarEvent, error) {
	var e models.CalendarEvent
	var attendees []byte
	if err := row.Scan(
		&e.ID, &e.CalendarID, &e.DomainID, &e.UserID, &e.UID, &e.Summary, &e.Description, &e.Location,
		&e.StartsAt, &e.EndsAt, &e.AllDay, &e.RRule, &e.Status, &e.Organizer, &attendees, &e.Sequence, &e.ETag,
		&e.CreatedAt, &e.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(attendees) > 0 {
		_ = json.Unmarshal(attendees, &e.Attendees)
	}
	return &e, nil
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
