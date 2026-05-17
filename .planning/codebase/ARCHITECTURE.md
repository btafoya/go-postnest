# Postnest Architecture

## Overview

Postnest is a multi-protocol email platform written in Go 1.25.0 with a React 19 frontend. It provides HTTP REST APIs, IMAP, SMTP, CardDAV/CalDAV, and a web UI, backed by PostgreSQL and Redis. Outbound email is relayed through Postmark; inbound email is received via Postmark webhooks.

The codebase follows a **service-oriented, multi-binary architecture** with interface-driven persistence layers and explicit dependency injection in each `main()`.

---

## Patterns

### Multi-Binary Deployment
The project ships as five separate binaries, each with a single responsibility:

| Binary | Source | Responsibility |
|--------|--------|--------------|
| `server` | `cmd/server/main.go` | HTTP API, IMAP, SMTP, DAV servers |
| `worker` | `cmd/worker/main.go` | Background job processors |
| `webui` | `cmd/webui/main.go` | Static SPA host + API reverse proxy |
| `admin` | `cmd/admin/main.go` | CLI user/domain administration |
| `migrate` | `cmd/migrate/main.go` | Database schema migrations |

Each binary imports only the internal packages it needs and constructs its own dependency graph. There is no shared runtime or service locator.

### Interface-Driven Persistence
Persistence is defined by interfaces in the domain package, implemented by a PostgreSQL-backed concrete type:

```go
// internal/mailstore/mailstore.go
type Store interface {
    CreateMessage(ctx context.Context, msg *models.Message, labelIDs []uuid.UUID, attachments []*models.Attachment) error
    GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*models.Message, error)
    // ... 25+ methods
}

// internal/mailstore/pgstore.go
type PGStore struct { pool *pgxpool.Pool }
func NewPGStore(pool *pgxpool.Pool) *PGStore
```

The same pattern is used for `contacts.Store` (`internal/contacts/contacts.go`).

### Dependency Injection via Constructors
Every service exposes a constructor that accepts its dependencies explicitly:

```go
// cmd/server/main.go
authService := auth.NewService(pgPool.Pool, cfg.Argon2idTime, cfg.Argon2idMemory, cfg.Argon2idThreads, cfg.SessionKey)
mailStore := mailstore.NewPGStore(pgPool.Pool)
webmailHandler := webmail.NewHandler(mailStore, authService, redisClient)
```

There is no reflection-based DI container.

### Embedded Static Assets
The React frontend is built into `internal/webui/dist`, which is embedded into the `webui` binary via `//go:embed all:dist`:

```go
// internal/webui/router.go
//go:embed all:dist
var distFS embed.FS
```

This allows the `webui` binary to serve the SPA and proxy API calls without external file dependencies.

---

## Layers

### 1. Models (`internal/models/models.go`)
Pure data structures with no behavior:

- `User`, `Domain`, `DomainMember`
- `Message`, `Thread`, `Label`, `Attachment`
- `Contact`, `DeliveryLog`, `AuthSession`

All entities use `github.com/google/uuid` v7 UUIDs for primary keys.

### 2. Persistence (`internal/db`, `internal/mailstore`, `internal/contacts`)

- `db.Pool` — thin wrapper around `pgxpool.Pool` with lifecycle helpers.
- `mailstore.PGStore` — implements `mailstore.Store`; handles messages, threads, labels, attachments, flags, delivery logs, and full-text search.
- `contacts.PGStore` — implements `contacts.Store`; handles vCard-backed contacts.

Key patterns:
- Transactions via `pgx.Tx` for multi-table writes (e.g., `CreateMessage` inserts into `messages`, `message_labels`, and `attachments`).
- `ON CONFLICT` for upserts (contacts, thread creation).
- `pgx.CopyFrom` for bulk attachment inserts.
- PostgreSQL `TSVECTOR` with weighted components for full-text search (`Search`, `UpdateSearchVector`).

### 3. Services (`internal/auth`, `internal/reputation`, `internal/certmanager`)

- `auth.Service` — password hashing (Argon2id), session/API-key creation and validation, domain membership checks.
- `reputation.Engine` — whitelist/blacklist/greylist evaluation for inbound mail.
- `certmanager.Manager` — ACME certificate lifecycle via `lego/v4` (Cloudflare DNS-01 challenge), background renewal loop.

### 4. Protocol Handlers (`internal/api`, `internal/webmail`, `internal/webhook`, `internal/smtp`, `internal/imap`, `internal/dav`)

- `api` — shared HTTP middleware (request ID, structured logging, recovery, CORS, rate limiting, session extraction).
- `webmail.Handler` — REST API for labels, messages, threads, drafts, search. Uses `chi/v5` router.
- `webhook.Handler` — public POST endpoints for Postmark inbound, bounce, delivery, and spam events. Verifies HMAC-SHA256 signatures and deduplicates via Redis `SETNX`.
- `smtp.Server` — go-smtp server with PLAIN and LOGIN auth. Parses MIME, forwards outbound to Postmark, stores sent copy.
- `imap.Server` — go-imap backend mapping labels to mailboxes. Supports FETCH, SEARCH, STORE, EXPUNGE, COPY.
- `dav.Handler` — CardDAV (contacts) and CalDAV (stub) via go-webdav. Basic auth against `auth.Service`.

### 5. Workers (`internal/workers`)
A Redis-backed job queue with pluggable processors:

```go
pool := workers.NewPool(redisClient, log, cfg.WorkerConcurrency, cfg.WorkerPollInterval)
pool.Register("inbound", workers.NewInboundProcessor(...))
pool.Register("bounce",  workers.NewBounceProcessor(...))
pool.Register("delivery", workers.NewDeliveryProcessor(...))
pool.Register("send_draft", workers.NewSendProcessor(...))
pool.Register("spam", workers.NewSpamProcessor(...))
```

Queue semantics:
- Three Redis keys: `queue:jobs` (list), `queue:jobs:delayed` (sorted set), `queue:jobs:dead` (list).
- `BRPop` for blocking dequeue; delayed jobs promoted via `ZRangeByScore`.
- Exponential backoff on retry (5s * attempts); max 3 attempts before dead-lettering.

### 6. Frontend (`web/`)
React 19 SPA built with Vite and TailwindCSS. Communicates with backend via:
- `axios` client (`web/src/api.js`) with cookie-based auth.
- SSE client (`web/src/sse.js`) for real-time mailbox updates.
- React Router v7 for client-side routing.

The `webui` server proxies `/api/*`, `/admin/*`, `/webhooks/*`, `/healthz`, `/.well-known/*`, `/dav/*` to the backend API server.

---

## Data Flow

### Inbound Email (Postmark Webhook)

```
Postmark --POST--> /webhooks/postmark/inbound
  webhook.Handler.verifySignature()
  webhook.Handler.dedup() // Redis SETNX
  webhook.Handler.enqueue("inbound", payload)

Redis queue:jobs <-- worker.Pool.dequeue()
  InboundProcessor.Process()
    auth.GetDomainByName()
    auth.GetUserByEmail()
    reputation.EvaluateInbound() // whitelist/blacklist/greylist
    mailstore.FindOrCreateThread()
    mailstore.CreateMessage() + INBOX label
    mailstore.UpdateSearchVector()
```

### Outbound Email (SMTP or Web Draft)

**Via SMTP client:**
```
SMTP client --AUTH PLAIN/LOGIN--> smtp.Server
  smtpSession.Auth() -> auth.Authenticate()
  smtpSession.Mail() -> auth.IsDomainMember()
  smtpSession.Data() -> go-message parser
    postmark.SendEmail() -> Postmark API
    mailstore.CreateMessage() + SENT label
```

**Via Web UI draft:**
```
Browser POST /api/v1/drafts -> webmail.createDraft()
Browser POST /api/v1/drafts/{id}/send -> webmail.sendDraft()
  workers.Job enqueue to Redis "send_draft"
  SendProcessor.Process()
    postmark.SendEmail()
    mailstore.UpdateMessage() // is_draft=false, mailbox=SENT
    mailstore.CreateDeliveryLog()
```

### Web UI Request

```
Browser --> webui server (:3000)
  /events -> SSEHub (Redis pub/sub broadcast)
  /api/* -> APIProxy -> backend server (:8080)
  /* -> embed.FS static assets / index.html SPA fallback
```

### IMAP Session

```
IMAP client --AUTH--> imap.Server
  imapBackend.Login() -> auth.Authenticate() + GetUserDomains()
  imapUser.ListMailboxes() -> mailstore.GetLabels()
  imapMailbox.ListMessages() -> mailstore.ListMessages() + GetFlagsBatch()
```

---

## Abstractions

### Context Values for Request Scoping
`internal/api/middleware.go` defines typed context keys:

```go
type ctxKey string
const ctxKeyUser ctxKey = "user"
const ctxKeyDomainID ctxKey = "domain_id"
const ctxKeyRequestID ctxKey = "request_id"
```

`RequireSession` middleware populates the user; `RequireDomainAdmin` populates the domain ID.

### Unified Errors
`internal/api/errors.go` provides a single `AppError` type with structured field errors:

```go
type AppError struct {
    Code       string     `json:"code"`
    Message    string     `json:"message"`
    Details    []FieldError `json:"details,omitempty"`
    StatusCode int        `json:"-"`
}
```

Common errors are package-level vars (`ErrNotFound`, `ErrUnauthorized`, `ErrRateLimited`).

### Store Patches
`mailstore.MessagePatch` uses pointer fields to distinguish "unset" from "false":

```go
type MessagePatch struct {
    IsRead    *bool
    IsFlagged *bool
    // ...
}
```

This allows `UPDATE ... SET is_read = coalesce($4, is_read)` semantics.

---

## Entry Points

### `cmd/server/main.go`
The largest entry point. It:
1. Loads config (`config.Load()`).
2. Opens PostgreSQL and Redis.
3. Constructs services (`auth`, `mailstore`, `contacts`, `postmark`).
4. Registers `chi` routes: public health, webhooks (public), authenticated webmail API, admin API.
5. Configures TLS: ACME, static cert, or insecure/plaintext.
6. Starts IMAP and SMTP listeners (optionally TLS-wrapped).
7. Registers DAV routes.
8. Blocks on `SIGINT`/`SIGTERM`, then gracefully shuts down all servers.

### `cmd/worker/main.go`
1. Loads config.
2. Opens PostgreSQL and Redis.
3. Constructs `workers.Pool` and registers all processors.
4. Calls `pool.Start(ctx)` and blocks on signals.

### `cmd/webui/main.go`
1. Parses env vars into `webui.Config`.
2. Creates a Gin router (`webui.NewRouter`).
3. Starts HTTP server and blocks on signals.

### `cmd/admin/main.go`
CLI tool with subcommands: `create-user`, `create-domain`, `add-member`, `setup`. Directly queries PostgreSQL with `pgxpool`.

### `cmd/migrate/main.go`
Wraps `golang-migrate/migrate/v4` with embedded SQL files. Supports `up`, `down`, `version`, `force`.

---

## Service Boundaries

### Authentication & Authorization (`internal/auth`)
- **Inbound**: email + password → Argon2id verify → `models.User`.
- **Session**: random 32-byte token → SHA-256 hash stored in `auth_sessions`.
- **API Key**: same table, `type='api_key'`.
- **Domain roles**: `domain_members(role)` — `admin`, `user`, `readonly`.

### Mail Storage (`internal/mailstore`)
- Single interface for all mail persistence.
- Gmail-style labels via `labels` + `message_labels` many-to-many.
- Threading by `subject_hash` + `message_ids` array.
- Full-text search via PostgreSQL `tsvector`.

### Contacts (`internal/contacts`)
- Separate `Store` interface.
- Upsert on `(domain_id, user_id, email)`.
- Exposed via CardDAV at `/dav/contacts/`.

### Workers (`internal/workers`)
- Redis-backed queue; no external job framework.
- Processors are stateless and receive `context.Context`.
- All processors accept dependencies via constructors (e.g., `mailstore.Store`, `auth.Service`, `postmark.Client`).

### Certificate Management (`internal/certmanager`)
- ACME via `lego/v4` with DNS-01 challenge.
- Supports Cloudflare out of the box.
- Stores account key and registration in `cfg.CertDir/accounts/<hash>/`.
- Background renewal ticker.

### Postmark Integration (`internal/postmark`)
- Thin wrapper around `mrz1836/postmark` library.
- `SendEmail` creates a per-call HTTP client (timeout 30s).
- `ParseInbound` unmarshals webhook payloads into a structured type.

---

## Notable Design Decisions

1. **No ORM**: All SQL is hand-written with `pgx` for explicit query control and full-text search.
2. **No shared state between binaries**: `server` and `worker` are separate processes communicating only through Redis and PostgreSQL.
3. **Web UI as separate binary**: The SPA is served by a dedicated `webui` process that reverse-proxies API calls, allowing independent scaling and caching strategies.
4. **IMAP labels-as-mailboxes**: The label system is exposed as IMAP mailboxes, making the system compatible with standard email clients.
5. **Search indexing is synchronous**: `UpdateSearchVector` runs inline after message creation. At scale this should be moved to an async worker.
6. **Greylist/reputation is synchronous in inbound worker**: The `InboundProcessor` evaluates reputation before storing the message.
