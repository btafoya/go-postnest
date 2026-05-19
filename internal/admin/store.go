package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/go-postnest/postnest/internal/models"
)

var ErrNotFound = fmt.Errorf("not found")
var ErrInvalidRole = fmt.Errorf("invalid role")

// Store handles admin persistence.
type Store interface {
	ListDomains(ctx context.Context, limit, offset int) ([]*DomainRow, int64, error)
	CreateDomain(ctx context.Context, name, token, stream string) (*models.Domain, error)
	UpdateDomain(ctx context.Context, id uuid.UUID, name, token, stream string, isActive bool) error
	DeleteDomain(ctx context.Context, id uuid.UUID) error
	ToggleDomainActive(ctx context.Context, id uuid.UUID, isActive bool) error

	ListUsers(ctx context.Context, limit, offset int) ([]*UserRow, int64, error)
	CreateUser(ctx context.Context, email, passwordHash, displayName string, isSuperAdmin bool) (*models.User, error)
	UpdateUser(ctx context.Context, id uuid.UUID, email, displayName string, isSuperAdmin bool) error
	DeleteUser(ctx context.Context, id uuid.UUID) error
	ResetPassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	GetUserDomainMemberships(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error)
	AddMember(ctx context.Context, userID, domainID uuid.UUID, role string) (*models.DomainMember, error)
	UpdateMemberRole(ctx context.Context, userID, domainID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, userID, domainID uuid.UUID) error

	GetSettings(ctx context.Context) (map[string]string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// PGStore implements Store with PostgreSQL.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore creates an admin store.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// DomainRow is a domain with computed user count.
type DomainRow struct {
	models.Domain
	IsActive  bool
	UserCount int64
}

// UserRow is a user with domain memberships.
type UserRow struct {
	models.User
	Memberships []*models.DomainMember
}

// ListDomains returns paginated domains with user counts.
func (s *PGStore) ListDomains(ctx context.Context, limit, offset int) ([]*DomainRow, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.name, d.postmark_token, d.postmark_stream, d.is_active, d.created_at, d.updated_at,
			COALESCE(m.cnt, 0)
		FROM domains d
		LEFT JOIN (SELECT domain_id, COUNT(*) AS cnt FROM domain_members GROUP BY domain_id) m ON m.domain_id = d.id
		ORDER BY d.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*DomainRow
	for rows.Next() {
		var d DomainRow
		var tok, stream *string
		err := rows.Scan(&d.ID, &d.Name, &tok, &stream, &d.IsActive, &d.CreatedAt, &d.UpdatedAt, &d.UserCount)
		if err != nil {
			return nil, 0, err
		}
		if tok != nil {
			d.PostmarkToken = *tok
		}
		if stream != nil {
			d.PostmarkStream = *stream
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM domains`).Scan(&total); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// CreateDomain inserts a new domain.
func (s *PGStore) CreateDomain(ctx context.Context, name, token, stream string) (*models.Domain, error) {
	id := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if stream == "" {
		stream = "outbound"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO domains (id, name, postmark_token, postmark_stream, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
	`, id, name, token, stream, now)
	if err != nil {
		return nil, err
	}
	return &models.Domain{ID: id, Name: name, PostmarkToken: token, PostmarkStream: stream, CreatedAt: now, UpdatedAt: now}, nil
}

// UpdateDomain modifies a domain.
func (s *PGStore) UpdateDomain(ctx context.Context, id uuid.UUID, name, token, stream string, isActive bool) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE domains SET name=$2, postmark_token=$3, postmark_stream=$4, is_active=$5, updated_at=$6
		WHERE id=$1
	`, id, name, token, stream, isActive, time.Now().UTC())
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDomain removes a domain (cascades via FK).
func (s *PGStore) DeleteDomain(ctx context.Context, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM domains WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ToggleDomainActive flips the is_active flag.
func (s *PGStore) ToggleDomainActive(ctx context.Context, id uuid.UUID, isActive bool) error {
	ct, err := s.pool.Exec(ctx, `UPDATE domains SET is_active=$2, updated_at=$3 WHERE id=$1`, id, isActive, time.Now().UTC())
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUsers returns paginated users with memberships.
func (s *PGStore) ListUsers(ctx context.Context, limit, offset int) ([]*UserRow, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.display_name, u.is_super_admin, u.created_at, u.updated_at,
			dm.domain_id, d.name, dm.role, dm.created_at
		FROM users u
		LEFT JOIN domain_members dm ON dm.user_id = u.id
		LEFT JOIN domains d ON d.id = dm.domain_id
		ORDER BY u.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	userMap := make(map[uuid.UUID]*UserRow)
	var order []uuid.UUID
	for rows.Next() {
		var uid uuid.UUID
		var email, displayName string
		var isSuperAdmin bool
		var createdAt, updatedAt time.Time
		var domainID *uuid.UUID
		var domainName *string
		var role *string
		var memCreatedAt *time.Time
		err := rows.Scan(&uid, &email, &displayName, &isSuperAdmin, &createdAt, &updatedAt,
			&domainID, &domainName, &role, &memCreatedAt)
		if err != nil {
			return nil, 0, err
		}
		u, exists := userMap[uid]
		if !exists {
			u = &UserRow{
				User: models.User{
					ID:           uid,
					Email:        email,
					DisplayName:  displayName,
					IsSuperAdmin: isSuperAdmin,
					CreatedAt:    createdAt,
					UpdatedAt:    updatedAt,
				},
			}
			userMap[uid] = u
			order = append(order, uid)
		}
		if domainID != nil && role != nil {
			m := &models.DomainMember{
				DomainID:  *domainID,
				UserID:    uid,
				Role:      *role,
				CreatedAt: *memCreatedAt,
			}
			if domainName != nil {
				m.DomainName = *domainName
			}
			u.Memberships = append(u.Memberships, m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	out := make([]*UserRow, 0, len(order))
	for _, uid := range order {
		out = append(out, userMap[uid])
	}

	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// CreateUser inserts a user.
func (s *PGStore) CreateUser(ctx context.Context, email, passwordHash, displayName string, isSuperAdmin bool) (*models.User, error) {
	id := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, is_super_admin, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
	`, id, email, passwordHash, displayName, isSuperAdmin, now)
	if err != nil {
		return nil, err
	}
	return &models.User{ID: id, Email: email, DisplayName: displayName, PasswordHash: passwordHash, IsSuperAdmin: isSuperAdmin, CreatedAt: now, UpdatedAt: now}, nil
}

// UpdateUser modifies a user.
func (s *PGStore) UpdateUser(ctx context.Context, id uuid.UUID, email, displayName string, isSuperAdmin bool) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE users SET email=$2, display_name=$3, is_super_admin=$4, updated_at=$5
		WHERE id=$1
	`, id, email, displayName, isSuperAdmin, time.Now().UTC())
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser removes a user (cascades via FK).
func (s *PGStore) DeleteUser(ctx context.Context, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetPassword updates a user's password hash.
func (s *PGStore) ResetPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=$3 WHERE id=$1`, id, passwordHash, time.Now().UTC())
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetUserDomainMemberships returns domain memberships for a user.
func (s *PGStore) GetUserDomainMemberships(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dm.domain_id, d.name, dm.user_id, dm.role, dm.created_at
		FROM domain_members dm
		JOIN domains d ON d.id = dm.domain_id
		WHERE dm.user_id=$1
		ORDER BY dm.created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.DomainMember
	for rows.Next() {
		var m models.DomainMember
		err := rows.Scan(&m.DomainID, &m.DomainName, &m.UserID, &m.Role, &m.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

var validRoles = map[string]bool{"admin": true, "user": true, "readonly": true}

// AddMember grants a user membership in a domain at the given role.
func (s *PGStore) AddMember(ctx context.Context, userID, domainID uuid.UUID, role string) (*models.DomainMember, error) {
	if !validRoles[role] {
		return nil, fmt.Errorf("%w: invalid role %q", ErrInvalidRole, role)
	}
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO domain_members (domain_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
	`, domainID, userID, role, now)
	if err != nil {
		return nil, err
	}
	var name string
	if err := s.pool.QueryRow(ctx, `SELECT name FROM domains WHERE id=$1`, domainID).Scan(&name); err != nil {
		return nil, err
	}
	return &models.DomainMember{DomainID: domainID, DomainName: name, UserID: userID, Role: role, CreatedAt: now}, nil
}

// UpdateMemberRole changes a user's role within a domain.
func (s *PGStore) UpdateMemberRole(ctx context.Context, userID, domainID uuid.UUID, role string) error {
	if !validRoles[role] {
		return fmt.Errorf("%w: invalid role %q", ErrInvalidRole, role)
	}
	ct, err := s.pool.Exec(ctx, `UPDATE domain_members SET role=$3 WHERE user_id=$1 AND domain_id=$2`, userID, domainID, role)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveMember revokes a user's membership in a domain.
func (s *PGStore) RemoveMember(ctx context.Context, userID, domainID uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM domain_members WHERE user_id=$1 AND domain_id=$2`, userID, domainID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSettings returns all system settings.
func (s *PGStore) GetSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM system_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetSetting upserts a system setting.
func (s *PGStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO system_settings (key, value, updated_at) VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value=$2, updated_at=$3
	`, key, value, time.Now().UTC())
	return err
}
