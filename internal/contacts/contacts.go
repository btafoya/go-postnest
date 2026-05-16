package contacts


import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/go-postnest/postnest/internal/models"
)


// Store handles contact persistence.
type Store interface {
	Create(ctx context.Context, contact *models.Contact) error
	GetByID(ctx context.Context, domainID, userID, contactID uuid.UUID) (*models.Contact, error)
	GetByEmail(ctx context.Context, domainID, userID uuid.UUID, email string) (*models.Contact, error)
	List(ctx context.Context, domainID, userID uuid.UUID, limit, offset int) ([]*models.Contact, int64, error)
	Delete(ctx context.Context, domainID, userID, contactID uuid.UUID) error
}

// PGStore implements Store with PostgreSQL.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore creates a contact store.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// Create inserts a contact.
func (s *PGStore) Create(ctx context.Context, c *models.Contact) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.Must(uuid.NewV7())
	}
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt
	_, err := s.pool.Exec(ctx, `
		INSERT INTO contacts (id, domain_id, user_id, email, name, given_name, family_name, organization, phone, vcard_data, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (domain_id, user_id, email) DO UPDATE SET
			name=EXCLUDED.name, given_name=EXCLUDED.given_name, family_name=EXCLUDED.family_name,
			organization=EXCLUDED.organization, phone=EXCLUDED.phone, vcard_data=EXCLUDED.vcard_data,
			updated_at=now()
	`, c.ID, c.DomainID, c.UserID, c.Email, c.Name, c.GivenName, c.FamilyName, c.Organization, c.Phone, c.VCardData, c.CreatedAt, c.UpdatedAt)
	return err
}

// GetByEmail fetches a contact by email.
func (s *PGStore) GetByEmail(ctx context.Context, domainID, userID uuid.UUID, email string) (*models.Contact, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, domain_id, user_id, email, name, given_name, family_name, organization, phone, vcard_data, created_at, updated_at
		FROM contacts WHERE domain_id=$1 AND user_id=$2 AND email=$3
	`, domainID, userID, email)
	var c models.Contact
	if err := row.Scan(&c.ID, &c.DomainID, &c.UserID, &c.Email, &c.Name, &c.GivenName, &c.FamilyName, &c.Organization, &c.Phone, &c.VCardData, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("not found")
		}
		return nil, err
	}
	return &c, nil
}

// List returns contacts for a user.
func (s *PGStore) List(ctx context.Context, domainID, userID uuid.UUID, limit, offset int) ([]*models.Contact, int64, error) {
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE domain_id=$1 AND user_id=$2`, domainID, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, domain_id, user_id, email, name, given_name, family_name, organization, phone, vcard_data, created_at, updated_at
		FROM contacts WHERE domain_id=$1 AND user_id=$2 ORDER BY name ASC LIMIT $3 OFFSET $4
	`, domainID, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*models.Contact
	for rows.Next() {
		var c models.Contact
		if err := rows.Scan(&c.ID, &c.DomainID, &c.UserID, &c.Email, &c.Name, &c.GivenName, &c.FamilyName, &c.Organization, &c.Phone, &c.VCardData, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &c)
	}
	return out, total, rows.Err()
}

// GetByID fetches a contact by ID.
func (s *PGStore) GetByID(ctx context.Context, domainID, userID, contactID uuid.UUID) (*models.Contact, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, domain_id, user_id, email, name, given_name, family_name, organization, phone, vcard_data, created_at, updated_at
		FROM contacts WHERE id=$1 AND domain_id=$2 AND user_id=$3
	`, contactID, domainID, userID)
	var c models.Contact
	if err := row.Scan(&c.ID, &c.DomainID, &c.UserID, &c.Email, &c.Name, &c.GivenName, &c.FamilyName, &c.Organization, &c.Phone, &c.VCardData, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("not found")
		}
		return nil, err
	}
	return &c, nil
}

// Delete removes a contact.
func (s *PGStore) Delete(ctx context.Context, domainID, userID, contactID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM contacts WHERE id=$1 AND domain_id=$2 AND user_id=$3`, contactID, domainID, userID)
	return err
}
