# Component Design — Go-PostNest Postmark Mail Platform

## 1. Package Layout

```
cmd/
  server/           # Main application: IMAP + SMTP + HTTP (webmail + DAV + webhook)
    main.go
  worker/           # Background worker pool
    main.go

internal/
  auth/             # Password hashing, sessions, API keys, multi-domain auth
  mailstore/        # Canonical mail persistence interface + PostgreSQL impl
  smtp/             # SMTP proxy server (submission + relay)
  imap/             # IMAP4rev1 server implementation
  webmail/          # HTTP handlers for REST API
  postmark/         # Postmark API client + inbound parsing
  webhook/          # HTTP receivers for Postmark webhooks
  contacts/         # Contact CRUD + vCard conversion
  reputation/       # Spam rules engine + contact scoring
  search/           # PostgreSQL tsvector search abstraction
  workers/          # Job definitions + processor implementations
  dav/              # WebDAV/CalDAV/CardDAV HTTP handlers
  api/              # Shared REST middleware (auth, CORS, JSON, rate limit, errors)
  models/           # Pure data structures (Message, User, Label, etc.)
  config/           # Environment-based configuration
  db/               # PostgreSQL connection pool + migrations
  redis/            # Redis client + queue helpers
  logger/           # Structured logging setup
```

## 2. Dependency Rules

```
          ┌─────────────┐
          │   cmd/*     │  (main only, wires dependencies)
          └──────┬──────┘
                 │
    ┌────────────┼────────────┐
    ▼            ▼            ▼
┌───────┐   ┌───────┐   ┌───────┐
│ imap  │   │ smtp  │   │webmail│   (protocol adapters)
└───┬───┘   └───┬───┘   └───┬───┘
    │           │           │
    └───────────┼───────────┘
                ▼
        ┌───────────────┐
        │   mailstore   │   (canonical storage abstraction)
        └───────┬───────┘
                ▼
        ┌───────────────┐
        │   db (pgx)    │   (PostgreSQL driver)
        └───────────────┘

┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐
│ webhook │   │ workers │   │ search  │   │reputation│  (async / side-effect)
└────┬────┘   └────┬────┘   └────┬────┘   └────┬────┘
     │             │             │             │
     └─────────────┴──────┬──────┴─────────────┘
                          ▼
                   ┌────────────┐
                   │ redis, db  │
                   └────────────┘
```

- **No circular imports**: `mailstore` never imports `imap` or `smtp`.
- **Models are pure**: `internal/models` has no imports except standard library.
- **API layer is shared**: `internal/api` provides middleware reusable by `webmail`, `webhook`, and `dav`.

## 3. Key Go Interfaces

### 3.1 Models (Pure Structs)

```go
package models

import (
    "time"
    "github.com/google/uuid"
)

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

type Domain struct {
    ID             uuid.UUID
    Name           string
    PostmarkToken  string
    PostmarkStream string
    CreatedAt      time.Time
    UpdatedAt      time.Time
    Settings       map[string]any
}

type DomainMember struct {
    DomainID  uuid.UUID
    UserID    uuid.UUID
    Role      string
    CreatedAt time.Time
}

type Message struct {
    ID              uuid.UUID
    DomainID        uuid.UUID
    UserID          uuid.UUID
    ThreadID        *uuid.UUID
    PostmarkMessageID string
    Mailbox         string
    MessageIDHeader string
    InReplyTo       string
    References      []string
    Subject         string
    FromAddress     string
    FromName        string
    ToAddresses     []string
    CcAddresses     []string
    BccAddresses    []string
    ReplyTo         string
    Date            time.Time
    PlainText       string
    HTMLBody        string
    Source          []byte
    SizeBytes       int
    IsDraft         bool
    IsOutbound      bool
    IsRead          bool
    IsFlagged       bool
    IsAnswered      bool
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type Label struct {
    ID        uuid.UUID
    DomainID  uuid.UUID
    UserID    uuid.UUID
    Name      string
    Color     string
    IsSystem  bool
    CreatedAt time.Time
}

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

type Thread struct {
    ID            uuid.UUID
    DomainID      uuid.UUID
    UserID        uuid.UUID
    SubjectHash   string
    MessageIDs    []string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type Contact struct {
    ID           uuid.UUID
    DomainID     uuid.UUID
    UserID       uuid.UUID
    Email        string
    Name         string
    GivenName    string
    FamilyName   string
    Organization string
    Phone        string
    VCardData    string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

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
```

### 3.2 Mailstore Interface

```go
package mailstore

import (
    "context"
    "time"
    "github.com/google/uuid"
    "github.com/go-postnest/internal/models"
)

type ListOptions struct {
    Limit     int
    Offset    int
    SortField string // "date", "from", "subject", "size"
    SortDesc  bool
}

type SearchOptions struct {
    LabelID      *uuid.UUID
    From         string
    To           string
    HasAttachment bool
    DateAfter    *time.Time
    DateBefore   *time.Time
}

type MessagePatch struct {
    IsRead     *bool
    IsFlagged  *bool
    IsAnswered *bool
    IsDraft    *bool
    Mailbox    *string
}

type Store interface {
    // Messages
    CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error
    GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*models.Message, error)
    ListMessages(ctx context.Context, domainID, userID uuid.UUID, labelID *uuid.UUID, opts ListOptions) ([]*models.Message, int64, error)
    UpdateMessage(ctx context.Context, domainID, userID, messageID uuid.UUID, patch MessagePatch) error
    DeleteMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) error
    MoveToMailbox(ctx context.Context, domainID, userID, messageID uuid.UUID, mailbox string) error

    // Labels
    CreateLabel(ctx context.Context, label *models.Label) error
    GetLabels(ctx context.Context, domainID, userID uuid.UUID) ([]*models.Label, error)
    GetLabelByName(ctx context.Context, domainID, userID uuid.UUID, name string) (*models.Label, error)
    DeleteLabel(ctx context.Context, domainID, userID, labelID uuid.UUID) error

    // Message Labels
    ApplyLabels(ctx context.Context, messageID uuid.UUID, addLabelIDs, removeLabelIDs []uuid.UUID) error
    GetMessageLabels(ctx context.Context, messageID uuid.UUID) ([]*models.Label, error)

    // Threading
    GetThread(ctx context.Context, domainID, userID, threadID uuid.UUID) (*models.Thread, []*models.Message, error)
    FindOrCreateThread(ctx context.Context, domainID, userID uuid.UUID, subject, messageID, inReplyTo string, references []string) (*models.Thread, error)

    // Attachments
    CreateAttachments(ctx context.Context, attachments []*models.Attachment) error
    GetAttachment(ctx context.Context, attachmentID uuid.UUID) (*models.Attachment, error)

    // Flags
    SetFlag(ctx context.Context, messageID uuid.UUID, flag string) error
    ClearFlag(ctx context.Context, messageID uuid.UUID, flag string) error
    GetFlags(ctx context.Context, messageID uuid.UUID) ([]string, error)

    // Search
    Search(ctx context.Context, domainID, userID uuid.UUID, query string, opts SearchOptions) ([]*models.Message, int64, error)
    UpdateSearchVector(ctx context.Context, messageID uuid.UUID) error

    // Counts
    CountUnreadByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error)
    CountTotalByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error)
}
```

### 3.3 Auth Interface

```go
package auth

import (
    "context"
    "time"
    "github.com/google/uuid"
    "github.com/go-postnest/internal/models"
)

type Service interface {
    Authenticate(ctx context.Context, email, password string) (*models.User, error)
    CreateSession(ctx context.Context, userID uuid.UUID, ip, userAgent string, expiry time.Duration) (*models.AuthSession, string, error)
    ValidateSession(ctx context.Context, token string) (*models.AuthSession, *models.User, error)
    ValidateAPIKey(ctx context.Context, key string) (*models.AuthSession, *models.User, error)
    Logout(ctx context.Context, token string) error
    CreateUser(ctx context.Context, user *models.User, password string) error
    UpdatePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error
    GetUserDomains(ctx context.Context, userID uuid.UUID) ([]*models.DomainMember, error)
    IsDomainAdmin(ctx context.Context, userID, domainID uuid.UUID) (bool, error)
}
```

### 3.4 Postmark Client Interface

```go
package postmark

import (
    "context"
    "github.com/go-postnest/internal/models"
)

type OutboundMessage struct {
    From        string
    To          []string
    Cc          []string
    Bcc         []string
    Subject     string
    TextBody    string
    HTMLBody    string
    Attachments []Attachment
    MessageStream string
}

type Attachment struct {
    Name        string
    ContentType string
    Content     []byte
}

type SendResponse struct {
    MessageID string
    ErrorCode int
    Message   string
}

type Client interface {
    SendEmail(ctx context.Context, apiToken string, msg *OutboundMessage) (*SendResponse, error)
    SendBatch(ctx context.Context, apiToken string, msgs []*OutboundMessage) ([]*SendResponse, error)
    GetDeliveryStatus(ctx context.Context, apiToken, messageID string) (*DeliveryStatus, error)
}

// Inbound parsing helpers
type InboundPayload struct {
    FromName          string
    From              string
    To                string
    Subject           string
    TextBody          string
    HTMLBody          string
    MessageID         string
    OriginalRecipient string
    Date              string
    Headers           []InboundHeader
    Attachments       []InboundAttachment
}

type InboundHeader struct {
    Name  string
    Value string
}

type InboundAttachment struct {
    Name        string
    ContentType string
    Content     []byte // base64 decoded
}

func ParseInbound(payload map[string]any) (*InboundPayload, error)
```

### 3.5 Reputation Engine Interface

```go
package reputation

import (
    "context"
    "net"
    "github.com/google/uuid"
)

type Decision string

const (
    DecisionPass     Decision = "pass"
    DecisionGreylist Decision = "greylist"
    DecisionBlock    Decision = "block"
    DecisionJunk     Decision = "junk"
)

type Engine interface {
    EvaluateInbound(ctx context.Context, domainID uuid.UUID, from, to string, senderIP net.IP) (Decision, *Result, error)
    UpdateReputation(ctx context.Context, domainID uuid.UUID, email string, event EventType) error
    GetReputation(ctx context.Context, domainID uuid.UUID, email string) (*Score, error)
}

type EventType string
const (
    EventReceived   EventType = "received"
    EventSent       EventType = "sent"
    EventBounced    EventType = "bounced"
    EventComplained EventType = "complained"
)

type Result struct {
    MatchedRule string
    Reason      string
}

type Score struct {
    Email            string
    SentCount        int
    ReceivedCount    int
    BounceCount      int
    ComplaintCount   int
    Score            int // 0-100
    LastInteraction  time.Time
}
```

### 3.6 Search Indexer Interface

```go
package search

import (
    "context"
    "github.com/google/uuid"
)

type Indexer interface {
    Queue(ctx context.Context, messageID uuid.UUID) error
    ProcessQueue(ctx context.Context, batchSize int) (processed int, err error)
    Search(ctx context.Context, domainID, userID uuid.UUID, query string, opts QueryOptions) ([]uuid.UUID, int64, error)
}

type QueryOptions struct {
    Limit      int
    Offset     int
    LabelID    *uuid.UUID
    DateAfter  *time.Time
    DateBefore *time.Time
}
```

### 3.7 Contact Store Interface

```go
package contacts

import (
    "context"
    "github.com/google/uuid"
    "github.com/go-postnest/internal/models"
)

type ListOptions struct {
    Limit  int
    Offset int
    Sort   string
    Query  string
}

type ContactPatch struct {
    Name         *string
    GivenName    *string
    FamilyName   *string
    Organization *string
    Phone        *string
    VCardData    *string
}

type Store interface {
    Create(ctx context.Context, contact *models.Contact) error
    Get(ctx context.Context, domainID, userID, contactID uuid.UUID) (*models.Contact, error)
    GetByEmail(ctx context.Context, domainID, userID uuid.UUID, email string) (*models.Contact, error)
    List(ctx context.Context, domainID, userID uuid.UUID, opts ListOptions) ([]*models.Contact, int64, error)
    Update(ctx context.Context, domainID, userID, contactID uuid.UUID, patch ContactPatch) error
    Delete(ctx context.Context, domainID, userID, contactID uuid.UUID) error
    SyncFromVCard(ctx context.Context, domainID, userID uuid.UUID, vcard string) (*models.Contact, error)
    ToVCard(ctx context.Context, contact *models.Contact) (string, error)
}
```

## 4. Worker Job Definitions

```go
package workers

import "context"

// Job is the unit enqueued to Redis.
type Job struct {
    ID        string
    Type      string
    Payload   []byte
    Attempts  int
    MaxAttempts int
    CreatedAt int64
}

// Processor implementations

type InboundProcessor struct {
    Mailstore   mailstore.Store
    Reputation  reputation.Engine
    Search      search.Indexer
    Contacts    contacts.Store
}

func (p *InboundProcessor) Process(ctx context.Context, job *Job) error {
    // 1. Parse InboundPayload from job.Payload.
    // 2. Look up recipient user by OriginalRecipient.
    // 3. Create message in mailstore (INBOX label).
    // 4. Run reputation.EvaluateInbound.
    //    - If block: delete message or move to JUNK.
    //    - If greylist: enqueue greylist:retry with delay.
    //    - If pass: queue search indexer, update contact reputation.
    // 5. Create/update contact from sender.
    // 6. Send IDLE notification via Redis pub/sub.
}

type BounceProcessor struct {
    Mailstore  mailstore.Store
    Reputation reputation.Engine
}

func (p *BounceProcessor) Process(ctx context.Context, job *Job) error {
    // Update delivery_logs.status = bounced.
    // Update contact_reputation.bounce_count.
    // Recalculate score.
}

type DeliveryProcessor struct {
    Mailstore mailstore.Store
    Postmark  postmark.Client
}

func (p *DeliveryProcessor) Process(ctx context.Context, job *Job) error {
    // Poll Postmark for delivery status.
    // Update delivery_logs.
}

type ReputationUpdater struct {
    Reputation reputation.Engine
}

func (p *ReputationUpdater) Process(ctx context.Context, job *Job) error {
    // Batch recalculate contact_reputation.score for stale contacts.
}

type SpamEvaluator struct {
    Reputation reputation.Engine
}

func (p *SpamEvaluator) Process(ctx context.Context, job *Job) error {
    // Re-evaluate greylisted messages after delay.
    // If sender passes greylist, move to INBOX and notify.
}

type SearchUpdater struct {
    Search search.Indexer
}

func (p *SearchUpdater) Process(ctx context.Context, job *Job) error {
    // Process search index queue in batches.
    // Update messages.search_vector via PostgreSQL function.
}

type MailboxSync struct {
    Mailstore mailstore.Store
    Postmark  postmark.Client
}

func (p *MailboxSync) Process(ctx context.Context, job *Job) error {
    // Reconcile Postmark inbound stream with local messages.
    // Handle any missed webhooks.
}

type PostmarkSender struct {
    Postmark  postmark.Client
    Mailstore mailstore.Store
}

func (p *PostmarkSender) Process(ctx context.Context, job *Job) error {
    // Send outbound message via Postmark API.
    // Update delivery_logs with postmark_message_id.
}
```

## 5. Shared Middleware (internal/api)

```go
package api

import "net/http"

// Middleware stack applied to all HTTP handlers (webmail, webhook, DAV).
func DefaultMiddleware() []func(http.Handler) http.Handler {
    return []func(http.Handler) http.Handler{
        RequestID,          // Inject X-Request-ID header / context
        StructuredLogger,   // slog request logging
        Recovery,           // Panic recovery → 500 JSON error
        CORS,               // Configurable origins
        RateLimiter,        // Per-IP + per-user token bucket
        ContentTypeJSON,    // Validate application/json where expected
    }
}

// Auth middleware variants
func RequireSession(authService auth.Service) func(http.Handler) http.Handler
func RequireAPIKey(authService auth.Service) func(http.Handler) http.Handler
func RequireDomainAdmin(authService auth.Service) func(http.Handler) http.Handler

// Context helpers
func UserFromContext(ctx context.Context) *models.User
func DomainIDFromContext(ctx context.Context) uuid.UUID
func RequestIDFromContext(ctx context.Context) string
```

## 6. Configuration

```go
package config

import "time"

type Config struct {
    // Server
    HTTPAddr     string        `env:"HTTP_ADDR" envDefault:":8080"`
    IMAPAddr     string        `env:"IMAP_ADDR" envDefault:":143"`
    IMAPSAddr    string        `env:"IMAPS_ADDR" envDefault:":993"`
    SMTPAddr     string        `env:"SMTP_ADDR" envDefault:":587"`
    SMTPSAddr    string        `env:"SMTPS_ADDR" envDefault:":465"`
    ReadTimeout  time.Duration `env:"READ_TIMEOUT" envDefault:"30s"`
    WriteTimeout time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`

    // TLS
    TLSCertPath  string `env:"TLS_CERT_PATH"`
    TLSKeyPath   string `env:"TLS_KEY_PATH"`
    TLSCAPath    string `env:"TLS_CA_PATH"`

    // Database
    PostgresDSN     string `env:"POSTGRES_DSN"`
    PostgresReadDSN string `env:"POSTGRES_READ_DSN"`
    MaxDBConns      int    `env:"MAX_DB_CONNS" envDefault:"25"`

    // Redis
    RedisURL string `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`

    // Auth
    Argon2idTime    uint32        `env:"ARGON2ID_TIME" envDefault:"3"`
    Argon2idMemory  uint32        `env:"ARGON2ID_MEMORY" envDefault:"65536"`
    Argon2idThreads uint8         `env:"ARGON2ID_THREADS" envDefault:"4"`
    SessionKey      string        `env:"SESSION_KEY"`
    SessionExpiry   time.Duration `env:"SESSION_EXPIRY" envDefault:"168h"` // 7 days

    // Postmark
    PostmarkWebhookSecret string `env:"POSTMARK_WEBHOOK_SECRET"`

    // Workers
    WorkerConcurrency int `env:"WORKER_CONCURRENCY" envDefault:"10"`
    WorkerPollInterval time.Duration `env:"WORKER_POLL_INTERVAL" envDefault:"5s"`

    // Limits
    MaxMessageSize    int64 `env:"MAX_MESSAGE_SIZE" envDefault:"52428800"` // 50MB
    MaxAttachmentSize int64 `env:"MAX_ATTACHMENT_SIZE" envDefault:"26214400"` // 25MB
}
```

## 7. Error Handling Patterns

### 7.1 Domain Errors

```go
package api

import "errors"

var (
    ErrNotFound       = errors.New("resource not found")
    ErrUnauthorized   = errors.New("unauthorized")
    ErrForbidden      = errors.New("forbidden")
    ErrValidation     = errors.New("validation failed")
    ErrConflict       = errors.New("conflict")
    ErrRateLimited    = errors.New("rate limited")
    ErrInternal       = errors.New("internal server error")
)

type AppError struct {
    Code       string
    Message    string
    Details    []FieldError
    StatusCode int
    Err        error
}

func (e *AppError) Error() string { return e.Message }
```

### 7.2 HTTP Translation

```go
func WriteError(w http.ResponseWriter, err error) {
    var appErr *AppError
    if errors.As(err, &appErr) {
        writeJSON(w, appErr.StatusCode, appErr)
        return
    }
    writeJSON(w, http.StatusInternalServerError, &AppError{
        Code: "internal_error", Message: "An unexpected error occurred", StatusCode: 500,
    })
}
```

## 8. Testing Strategy

| Layer | Approach | Tools |
|---|---|---|
| Models | Table-driven unit tests | `testing` |
| Store (pgx) | Integration tests against Testcontainers PostgreSQL | `testcontainers-go` |
| Auth | Unit tests with mocked password hash (fast bcrypt/argon2 params) | `testing` + `gomock` |
| IMAP | Protocol-level tests using `net/textproto` client | `testing` |
| SMTP | Protocol-level tests using `net/smtp` client | `testing` |
| Webmail API | HTTP handler tests with in-memory store mocks | `httptest` + `gomock` |
| Workers | Table-driven with mocked dependencies | `testing` + `gomock` |
| End-to-End | Docker Compose stack + `k6` or `go test` hitting all protocols | `docker-compose` |

## 9. Implementation Order (Recommended)

1. `internal/config`, `internal/db`, `internal/redis`, `internal/logger`
2. `internal/models`
3. `internal/mailstore` (PostgreSQL implementation)
4. `internal/auth` (Argon2id + sessions)
5. `internal/api` (middleware + error types)
6. `cmd/server` HTTP server scaffold
7. `internal/webmail` (REST API handlers)
8. `internal/postmark` (API client)
9. `internal/webhook` (inbound receiver)
10. `internal/workers` (inbound processor + postmark sender)
11. `internal/imap` (IMAP server)
12. `internal/smtp` (SMTP proxy)
13. `internal/contacts`
14. `internal/dav` (CardDAV first, then CalDAV)
15. `internal/search` (tsvector integration)
16. `internal/reputation` (spam rules)
17. `cmd/worker` (background worker binary)
