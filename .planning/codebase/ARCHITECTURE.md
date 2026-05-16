# PostNest Architecture

## 1. High-Level Pattern

PostNest follows a **layered architecture with hexagonal influences**:

- **Entry layer** (`cmd/`) — thin `main.go` binaries that wire dependencies and start services.
- **Protocol layer** (`internal/imap`, `internal/smtp`, `internal/webmail`, `internal/dav`, `internal/webhook`) — adapters that translate external protocols into domain operations.
- **Domain layer** (`internal/mailstore`, `internal/contacts`, `internal/auth`, `internal/reputation`, `internal/search`) — business logic and canonical interfaces.
- **Infrastructure layer** (`internal/db`, `internal/redis`, `internal/postmark`, `internal/logger`, `internal/certmanager`) — concrete implementations of external systems.

There is no `pkg/` directory; shared code lives in small, focused `internal/` packages.

## 2. System Layers and Boundaries

| Layer | Packages | Responsibility |
|---|---|---|
| **Commands** | `cmd/server`, `cmd/worker`, `cmd/migrate` | Binary entry points. No business logic. |
| **Protocol Adapters** | `internal/imap`, `internal/smtp`, `internal/webmail`, `internal/dav`, `internal/webhook`, `internal/api` | Translate IMAP/SMTP/HTTP/WebDAV into Go method calls. |
| **Domain Services** | `internal/auth`, `internal/mailstore`, `internal/contacts`, `internal/reputation`, `internal/search` | Core business rules, persistence interfaces, and multi-tenancy enforcement. |
| **Infrastructure** | `internal/db`, `internal/redis`, `internal/postmark`, `internal/certmanager`, `internal/logger` | Concrete drivers for PostgreSQL, Redis, Postmark API, ACME/Let's Encrypt, and structured logging. |
| **Models** | `internal/models` | Pure data structs used across all layers. |

Key boundary rule: protocol packages depend on domain interfaces (e.g., `mailstore.Store`), never on infrastructure directly.

## 3. Data Flow

### 3.1 Inbound Email (Postmark → Storage → IMAP/REST)

```
Postmark API
    ↓ POST /webhook/inbound
internal/webhook/webhook.go   (verify HMAC, enqueue job)
    ↓ LPUSH queue:jobs
internal/redis/redis.go
    ↓ BRPOP queue:jobs
internal/workers/inbound.go   (parse RFC822, extract attachments, thread)
    ↓
internal/mailstore/pgstore.go (INSERT messages, labels, attachments, threads)
    ↓
internal/search/search.go     (queue tsvector update)
    ↓
Redis pub/sub                 (IMAP IDLE notify)
    ↓
IMAP clients / REST clients   (see new message)
```

Actual files:
- `cmd/server/main.go` starts the webhook HTTP listener.
- `internal/webhook/webhook.go` defines `Handler` with `handleInbound()`.
- `internal/workers/inbound.go` defines `InboundProcessor.Process()`.
- `internal/mailstore/pgstore.go` has `CreateMessage()`, `FindOrCreateThread()`, `CreateAttachments()`.
- `internal/imap/backend.go` implements `imapMailbox.ListMessages()` and subscribes to Redis for IDLE.

### 3.2 Outbound Email (SMTP/REST → Postmark)

**SMTP path:**
```
Email Client
    ↓ AUTH PLAIN + MAIL FROM / RCPT TO / DATA
internal/smtp/smtp.go         (authenticate via internal/auth)
    ↓
internal/mailstore/pgstore.go (persist as Sent item with outbox label)
    ↓
internal/postmark/postmark.go (SendEmail via mrz1836/postmark)
    ↓
Postmark API
    ↓ webhook bounce/delivery
internal/webhook/webhook.go   (enqueue bounce/delivery jobs)
    ↓
internal/workers/bounce.go / delivery.go
    ↓
internal/mailstore/pgstore.go (update delivery_log / message flags)
```

**REST path:**
- `internal/webmail/webmail.go` `createDraft()` → `sendDraft()` queues to worker, which follows the SMTP relay path above.

## 4. Key Abstractions and Interfaces

### 4.1 Mail Persistence — `internal/mailstore/`

`internal/mailstore/mailstore.go` defines the canonical `Store` interface:

```go
type Store interface {
    CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error
    GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*models.Message, error)
    ListMessages(ctx context.Context, domainID, userID uuid.UUID, labelID *uuid.UUID, opts ListOptions) ([]*models.Message, int64, error)
    UpdateMessage(ctx context.Context, domainID, userID, messageID uuid.UUID, patch MessagePatch) error
    DeleteMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) error
    MoveToMailbox(ctx context.Context, domainID, userID, messageID uuid.UUID, mailbox string) error
    CreateLabel(ctx context.Context, label *models.Label) error
    GetLabels(ctx context.Context, domainID, userID uuid.UUID) ([]*models.Label, error)
    GetLabelByName(ctx context.Context, domainID, userID uuid.UUID, name string) (*models.Label, error)
    DeleteLabel(ctx context.Context, domainID, userID, labelID uuid.UUID) error
    ApplyLabels(ctx context.Context, messageID uuid.UUID, addLabelIDs, removeLabelIDs []uuid.UUID) error
    GetMessageLabels(ctx context.Context, messageID uuid.UUID) ([]*models.Label, error)
    GetThread(ctx context.Context, domainID, userID, threadID uuid.UUID) (*models.Thread, []*models.Message, error)
    FindOrCreateThread(ctx context.Context, domainID, userID uuid.UUID, subject, messageID, inReplyTo string, references []string) (*models.Thread, error)
    CreateAttachments(ctx context.Context, attachments []*models.Attachment) error
    GetAttachment(ctx context.Context, attachmentID uuid.UUID) (*models.Attachment, error)
    SetFlag(ctx context.Context, messageID uuid.UUID, flag string) error
    ClearFlag(ctx context.Context, messageID uuid.UUID, flag string) error
    GetFlags(ctx context.Context, messageID uuid.UUID) ([]string, error)
    Search(ctx context.Context, domainID, userID uuid.UUID, query string, opts SearchOptions) ([]*models.Message, int64, error)
    UpdateSearchVector(ctx context.Context, messageID uuid.UUID) error
    CountUnreadByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error)
    CountTotalByLabel(ctx context.Context, domainID, userID uuid.UUID, labelID uuid.UUID) (int64, error)
}
```

`PGStore` in `internal/mailstore/pgstore.go` is the sole production implementation. It uses `*pgxpool.Pool` directly.

### 4.2 Contact Persistence — `internal/contacts/`

`internal/contacts/contacts.go` defines a smaller `Store` interface:

```go
type Store interface {
    Create(ctx context.Context, c *models.Contact) error
    GetByEmail(ctx context.Context, domainID, userID uuid.UUID, email string) (*models.Contact, error)
    List(ctx context.Context, domainID, userID uuid.UUID, limit, offset int) ([]*models.Contact, int64, error)
    GetByID(ctx context.Context, domainID, userID, contactID uuid.UUID) (*models.Contact, error)
    Delete(ctx context.Context, domainID, userID, contactID uuid.UUID) error
}
```

### 4.3 Models — `internal/models/models.go`

Pure structs with no methods (except `ParseUUID`). Key types:
- `User` — platform user with Argon2id password hash.
- `Domain` — tenant boundary with Postmark token.
- `DomainMember` — links `User` + `Domain` + role (`admin`, `user`, `readonly`).
- `Message` — RFC822 source stored as `[]byte`, plus parsed headers.
- `Label` — Gmail-style tag (`INBOX`, `Sent`, etc.).
- `Attachment` — `[]byte` payload plus metadata.
- `Thread` — conversation grouping.
- `Contact` — address book entry with vCard data.
- `DeliveryLog` — outbound tracking.
- `AuthSession` — session or API key token hash.

## 5. Entry Points for Each Binary

### `cmd/server/main.go`

Wires and starts all public-facing services in a single process:
1. Loads config (`internal/config/config.go`).
2. Creates `db.Pool`, `redis.Client`.
3. Creates `auth.Service`, `mailstore.PGStore`, `contacts.PGStore`, `postmark.Client`.
4. Builds Chi router with middleware (`internal/api/middleware.go`).
5. Registers routes:
   - Public: `/healthz`, webhook routes (`internal/webhook/webhook.go`).
   - Authenticated: webmail API (`internal/webmail/webmail.go`).
   - Admin: placeholder group with `RequireDomainAdmin`.
   - DAV: `internal/dav/dav.go` mounts CardDAV/CalDAV routes.
6. Starts HTTP server, IMAP server (`internal/imap/imap.go`), SMTP server (`internal/smtp/smtp.go`).
7. Handles TLS via static certs or ACME (`internal/certmanager/manager.go`).
8. Graceful shutdown on SIGINT/SIGTERM (30s timeout).

### `cmd/worker/main.go`

Runs background job processors:
1. Loads config, creates `db.Pool`, `redis.Client`.
2. Creates `auth.Service`, `mailstore.PGStore`.
3. Creates `workers.Pool` with configured concurrency and poll interval.
4. Registers processors:
   - `"inbound"` → `workers.NewInboundProcessor(store, auth, log)`
   - `"bounce"` → `workers.NewBounceProcessor(pool, log)`
   - `"delivery"` → `workers.NewDeliveryProcessor(pool, log)`
5. Starts pool with `pool.Start(ctx)`.
6. Blocks on signal, cancels context, waits up to 30s for graceful drain.

### `cmd/migrate/main.go`

Standalone CLI for schema management:
- Commands: `up`, `down [n]`, `version`, `force V`.
- Uses `internal/migrate/migrate.go`, which embeds SQL files from `internal/migrate/migrations/*.sql` via `//go:embed`.
- Backed by `golang-migrate/migrate/v4`.

## 6. Concurrency Model

### 6.1 Worker Pool

`internal/workers/workers.go` implements a Redis-backed worker pool:
- `Pool` spawns `concurrency` goroutines (`worker()`), each blocking on `BRPOP` of `queue:jobs`.
- Jobs are JSON structs with `ID`, `Type`, `Payload`, `Attempts`, `MaxAttempts`.
- Failed jobs retry up to `MaxAttempts` (default 3), then dropped.
- `Enqueue()` in `internal/webhook/webhook.go` and `internal/search/search.go` uses `LPUSH`.

### 6.2 IMAP IDLE Pub/Sub

- `internal/imap/imap.go` creates the IMAP server with `go-imap`.
- `internal/imap/backend.go` implements `imapMailbox`.
- Mailbox updates broadcast to IDLE clients via Redis pub/sub channels (integration with `internal/redis/redis.go` `Publish`).
- Each IMAP connection runs in its own goroutine.

### 6.3 HTTP Server

- Standard Go `net/http.Server` with Chi router.
- Each request is a goroutine.
- Middleware injects request ID, structured logger, CORS, panic recovery, and session validation.

### 6.4 SMTP Server

- `go-smtp` server; each connection spawns a goroutine via `smtpBackend.NewSession()`.
- `smtpSession.Data()` parses the message with `go-message`, persists via `mailstore`, and relays to Postmark synchronously before returning `250 OK`.

## 7. Multi-Tenancy Approach

PostNest is **domain-scoped multi-tenant**:

- **`Domain`** (`models.Domain`) is the top-level tenant. Each domain has its own Postmark API token (`postmark_token`).
- **`User`** (`models.User`) can belong to multiple domains via `DomainMember`.
- **Roles**: `admin`, `user`, `readonly`.
- **Data scoping**: Every `mailstore.Store` method accepts `domainID` and `userID`. PostgreSQL queries filter by both columns.
- **IMAP login**: username is the user's email address; `auth.Service` resolves the user and their domains.
- **SMTP auth**: same email/password as IMAP; `auth.Service.Authenticate()` validates Argon2id hash.
- **Admin enforcement**: `api.RequireDomainAdmin` middleware in `internal/api/middleware.go` checks `auth.Service.IsDomainAdmin()`.

Known limitation (per `INTEGRATION.md`): some webmail handlers currently use `user.ID` as a placeholder for `domainID` pending full domain-context injection via header or membership lookup.
