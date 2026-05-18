package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a platform user.
type User struct {
	ID           uuid.UUID
	Email        string
	DisplayName  string
	PasswordHash string
	Timezone     string
	Locale       string
	IsSuperAdmin bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Settings     map[string]any
}

// Domain represents an email domain managed by the platform.
type Domain struct {
	ID             uuid.UUID
	Name           string
	PostmarkToken  string
	PostmarkStream string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Settings       map[string]any
}

// DomainMember links a user to a domain with a role.
type DomainMember struct {
	DomainID  uuid.UUID
	UserID    uuid.UUID
	Role      string
	CreatedAt time.Time
}

// Message is an email message stored in the system.
type Message struct {
	ID                uuid.UUID
	DomainID          uuid.UUID
	UserID            uuid.UUID
	ThreadID          *uuid.UUID
	PostmarkMessageID string
	Mailbox           string
	MessageIDHeader   string
	InReplyTo         string
	References        []string
	Subject           string
	FromAddress       string
	FromName          string
	ToAddresses       []string
	CcAddresses       []string
	BccAddresses      []string
	ReplyTo           string
	Date              time.Time
	PlainText         string
	HTMLBody          string
	Source            []byte
	SizeBytes         int
	IsDraft           bool
	IsOutbound        bool
	IsRead            bool
	IsFlagged         bool
	IsAnswered        bool
	SearchVector      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Label is a Gmail-style label for messages.
type Label struct {
	ID        uuid.UUID
	DomainID  uuid.UUID
	UserID    uuid.UUID
	Name      string
	Color     string
	IsSystem  bool
	CreatedAt time.Time
}

// Attachment is a file attached to a message.
type Attachment struct {
	ID          uuid.UUID
	MessageID   uuid.UUID
	Filename    string
	ContentType string
	SizeBytes   int
	Data        []byte
	ContentID   string
	CreatedAt   time.Time
}

// Thread groups related messages together.
type Thread struct {
	ID          uuid.UUID
	DomainID    uuid.UUID
	UserID      uuid.UUID
	SubjectHash string
	MessageIDs  []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Contact is an address book entry.
type Contact struct {
	ID           uuid.UUID `json:"id"`
	DomainID     uuid.UUID `json:"domain_id"`
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	GivenName    string    `json:"given_name"`
	FamilyName   string    `json:"family_name"`
	Organization string    `json:"organization"`
	Phone        string    `json:"phone"`
	VCardData    string    `json:"vcard_data"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DeliveryLog tracks the delivery status of an outbound message.
type DeliveryLog struct {
	ID                uuid.UUID
	MessageID         uuid.UUID
	DomainID          uuid.UUID
	Recipient         string
	Status            string
	PostmarkMessageID string
	Details           map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// AuthSession represents a user session or API key.
type AuthSession struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	Type       string
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	IPAddress  string
	UserAgent  string
	CreatedAt  time.Time
}


// Calendar is a user's calendar collection (CalDAV).
type Calendar struct {
	ID          uuid.UUID
	DomainID    uuid.UUID
	UserID      uuid.UUID
	Name        string
	Color       string
	Description string
	CTag        int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CalendarEvent is a single event (VEVENT) within a calendar.
type CalendarEvent struct {
	ID          uuid.UUID
	CalendarID  uuid.UUID
	DomainID    uuid.UUID
	UserID      uuid.UUID
	UID         string
	Summary     string
	Description string
	Location    string
	StartsAt    time.Time
	EndsAt      time.Time
	AllDay      bool
	RRule       string
	Status      string
	Organizer   string
	Attendees   []string
	Sequence    int
	ETag        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ParseUUID parses a UUID string.
func ParseUUID(s string) (uuid.UUID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, err
	}
	return u, nil
}
