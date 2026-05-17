package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/go-postnest/postnest/internal/models"
	"golang.org/x/crypto/argon2"
)

// Service handles authentication, sessions, and password hashing.
type Service struct {
	pool       *pgxpool.Pool
	argonTime  uint32
	argonMemory uint32
	argonThreads uint8
	sessionKey []byte
}

// NewService creates an auth service.
func NewService(pool *pgxpool.Pool, argonTime, argonMemory uint32, argonThreads uint8, sessionKey string) *Service {
	return &Service{
		pool:         pool,
		argonTime:    argonTime,
		argonMemory:  argonMemory,
		argonThreads: argonThreads,
		sessionKey:   []byte(sessionKey),
	}
}

// hashPassword creates an Argon2id hash.
func (s *Service) hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, s.argonTime, s.argonMemory, s.argonThreads, 32)
	encoded := base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
	return encoded, nil
}

// verifyPassword checks a password against its hash.
func (s *Service) verifyPassword(password, encodedHash string) bool {
	parts := splitN(encodedHash, "$", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[0])
	hash, err2 := base64.RawStdEncoding.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	computed := argon2.IDKey([]byte(password), salt, s.argonTime, s.argonMemory, s.argonThreads, uint32(len(hash)))
	return constantTimeCompare(computed, hash)
}

func splitN(s, sep string, n int) []string {
	// simple split for this use case
	idx := -1
	for i := range s {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			idx = i
			break
		}
	}
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+len(sep):]}
}

func constantTimeCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// Authenticate validates email and password.
func (s *Service) Authenticate(ctx context.Context, email, password string) (*models.User, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, email, password_hash, display_name, timezone, locale, is_super_admin, created_at, updated_at, settings FROM users WHERE email=$1`, email)
	var u models.User
	var settings []byte
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Timezone, &u.Locale, &u.IsSuperAdmin, &u.CreatedAt, &u.UpdatedAt, &settings); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, err
	}
	if !s.verifyPassword(password, u.PasswordHash) {
		return nil, fmt.Errorf("invalid credentials")
	}
	return &u, nil
}

// CreateSession generates a session token and stores its hash.
func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, ip, userAgent string, expiry time.Duration) (*models.AuthSession, string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := sha256.Sum256(tokenBytes)
	hashStr := base64.RawStdEncoding.EncodeToString(hash[:])

	session := &models.AuthSession{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		TokenHash: hashStr,
		Type:      "session",
		ExpiresAt: time.Now().UTC().Add(expiry),
		IPAddress: ip,
		UserAgent: userAgent,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, type, expires_at, ip_address, user_agent, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, session.ID, session.UserID, session.TokenHash, session.Type, session.ExpiresAt, session.IPAddress, session.UserAgent, session.CreatedAt)
	if err != nil {
		return nil, "", err
	}
	return session, token, nil
}

// ValidateSession checks a bearer/session token.
func (s *Service) ValidateSession(ctx context.Context, token string) (*models.AuthSession, *models.User, error) {
	tokenBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid token")
	}
	hash := sha256.Sum256(tokenBytes)
	hashStr := base64.RawStdEncoding.EncodeToString(hash[:])

	row := s.pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.token_hash, s.type, s.expires_at, s.last_used_at, s.ip_address, s.user_agent, s.created_at,
			u.id, u.email, u.display_name, u.timezone, u.locale, u.is_super_admin, u.created_at, u.updated_at
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash=$1 AND s.type='session' AND s.expires_at > now()
	`, hashStr)
	var session models.AuthSession
	var user models.User
	if err := row.Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.Type, &session.ExpiresAt, &session.LastUsedAt, &session.IPAddress, &session.UserAgent, &session.CreatedAt,
		&user.ID, &user.Email, &user.DisplayName, &user.Timezone, &user.Locale, &user.IsSuperAdmin, &user.CreatedAt, &user.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, fmt.Errorf("invalid or expired session")
		}
		return nil, nil, err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE auth_sessions SET last_used_at=now() WHERE id=$1`, session.ID)
	return &session, &user, nil
}

// ValidateAPIKey checks an API key.
func (s *Service) ValidateAPIKey(ctx context.Context, key string) (*models.AuthSession, *models.User, error) {
	tokenBytes, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid key")
	}
	hash := sha256.Sum256(tokenBytes)
	hashStr := base64.RawStdEncoding.EncodeToString(hash[:])

	row := s.pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.token_hash, s.type, s.expires_at, s.last_used_at, s.ip_address, s.user_agent, s.created_at,
			u.id, u.email, u.display_name, u.timezone, u.locale, u.is_super_admin, u.created_at, u.updated_at
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash=$1 AND s.type='api_key' AND s.expires_at > now()
	`, hashStr)
	var session models.AuthSession
	var user models.User
	if err := row.Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.Type, &session.ExpiresAt, &session.LastUsedAt, &session.IPAddress, &session.UserAgent, &session.CreatedAt,
		&user.ID, &user.Email, &user.DisplayName, &user.Timezone, &user.Locale, &user.IsSuperAdmin, &user.CreatedAt, &user.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, fmt.Errorf("invalid or expired api key")
		}
		return nil, nil, err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE auth_sessions SET last_used_at=now() WHERE id=$1`, session.ID)
	return &session, &user, nil
}

// Logout invalidates a session by token.
func (s *Service) Logout(ctx context.Context, token string) error {
	tokenBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return fmt.Errorf("invalid token")
	}
	hash := sha256.Sum256(tokenBytes)
	hashStr := base64.RawStdEncoding.EncodeToString(hash[:])
	_, err = s.pool.Exec(ctx, `DELETE FROM auth_sessions WHERE token_hash=$1`, hashStr)
	return err
}

// CreateUser creates a new user with hashed password and seeds system labels.
func (s *Service) CreateUser(ctx context.Context, user *models.User, password string) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.Must(uuid.NewV7())
	}
	user.CreatedAt = time.Now().UTC()
	user.UpdatedAt = user.CreatedAt

	hash, err := s.hashPassword(password)
	if err != nil {
		return err
	}
	user.PasswordHash = hash

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, timezone, locale, is_super_admin, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, user.ID, user.Email, user.PasswordHash, user.DisplayName, user.Timezone, user.Locale, user.IsSuperAdmin, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return err
	}

	// seed labels for user if domain_id is set in caller
	_, _ = tx.Exec(ctx, `
		INSERT INTO labels (id, domain_id, user_id, name, color, is_system, created_at)
		SELECT gen_random_uuid(), dm.domain_id, $1, unnest.label, '#4285f4', true, now()
		FROM domain_members dm
		CROSS JOIN LATERAL unnest(ARRAY['INBOX','SENT','DRAFTS','TRASH','JUNK','IMPORTANT','STARRED','ALL_MAIL']) AS unnest(label)
		WHERE dm.user_id=$1
		ON CONFLICT (domain_id, user_id, name) DO NOTHING
	`, user.ID)

	return tx.Commit(ctx)
}

// UpdatePassword changes a user's password.
func (s *Service) UpdatePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error {
	row := s.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, userID)
	var current string
	if err := row.Scan(&current); err != nil {
		return err
	}
	if !s.verifyPassword(oldPassword, current) {
		return fmt.Errorf("invalid current password")
	}
	newHash, err := s.hashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, userID, newHash)
	return err
}

// AdminResetPassword resets a user's password without verifying the old one.
func (s *Service) AdminResetPassword(ctx context.Context, userID uuid.UUID, newPassword string) error {
	newHash, err := s.hashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, userID, newHash)
	return err
}

// GetUserDomains returns domain memberships for a user.
func (s *Service) GetUserDomains(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error) {
	rows, err := s.pool.Query(ctx, `SELECT domain_id, user_id, role, created_at FROM domain_members WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.DomainMember
	for rows.Next() {
		var dm models.DomainMember
		if err := rows.Scan(&dm.DomainID, &dm.UserID, &dm.Role, &dm.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &dm)
	}
	return out, rows.Err()
}

// IsDomainAdmin checks whether the user has admin role for a domain.
func (s *Service) IsDomainAdmin(ctx context.Context, userID, domainID uuid.UUID) (bool, error) {
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT role FROM domain_members WHERE user_id=$1 AND domain_id=$2
	`, userID, domainID).Scan(&role)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return role == "admin" || role == "super_admin", nil
}

// IsDomainMember checks whether the user belongs to a domain (any role).
func (s *Service) IsDomainMember(ctx context.Context, userID, domainID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM domain_members WHERE user_id=$1 AND domain_id=$2)`, userID, domainID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// GetDomainByID fetches a domain by its ID.
func (s *Service) GetDomainByID(ctx context.Context, id uuid.UUID) (*models.Domain, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, postmark_token, postmark_stream, created_at, updated_at, settings
		FROM domains WHERE id=$1
	`, id)
	var d models.Domain
	err := row.Scan(&d.ID, &d.Name, &d.PostmarkToken, &d.PostmarkStream, &d.CreatedAt, &d.UpdatedAt, &d.Settings)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// GetDomainByName fetches a domain by its name.
func (s *Service) GetDomainByName(ctx context.Context, name string) (*models.Domain, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, postmark_token, postmark_stream, created_at, updated_at, settings
		FROM domains WHERE name=$1
	`, name)
	var d models.Domain
	var settings []byte
	if err := row.Scan(&d.ID, &d.Name, &d.PostmarkToken, &d.PostmarkStream, &d.CreatedAt, &d.UpdatedAt, &settings); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("domain not found")
		}
		return nil, err
	}
	return &d, nil
}

// GetUserByEmail fetches a user by their email address.
func (s *Service) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, email, password_hash, display_name, timezone, locale, is_super_admin, created_at, updated_at, settings FROM users WHERE email=$1`, email)
	var u models.User
	var settings []byte
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Timezone, &u.Locale, &u.IsSuperAdmin, &u.CreatedAt, &u.UpdatedAt, &settings); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	return &u, nil
}
