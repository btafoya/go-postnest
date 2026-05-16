# PostNest Architecture

## Overview

PostNest is a multi-tenant mail platform written in Go 1.25. It proxies email through Postmark while exposing standard protocols (IMAP4, SMTP, CardDAV) alongside a modern REST webmail API. PostgreSQL is the primary datastore; Redis powers background job queues.

---

## Entry Points

| Binary | Source | Responsibility |
|--------|--------|--------------|
| `postnest-server` | `cmd/server/main.go` | HTTP REST + webhooks, IMAP, SMTP, DAV, health checks, graceful shutdown |
| `postnest-worker` | `cmd/worker/main.go` | Redis-backed worker pool consuming background jobs |
| `postnest-migrate` | `cmd/migrate/main.go` | Embedded golang-migrate CLI (up/down/version/force) |

---

## Architectural Pattern: Layered Hexagonal-lite

The codebase follows a layered, interface-driven architecture without heavy framework ceremony:

```
┌─────────────────────────────────────────────────────────────────────┐
│  Transport Layer   (HTTP/chi, IMAP/go-imap, SMTP/go-smtp, DAV)    │
├─────────────────────────────────────────────────────────────────────┤
│  Handler Layer     (webmail, webhook, dav, imap, smtp)              │
├─────────────────────────────────────────────────────────────────────┤
│  Service Layer     (auth, postmark, certmanager)                    │
├─────────────────────────────────────────────────────────────────────┤
│  Store Layer       (mailstore.Store, contacts.Store)                  │
├─────────────────────────────────────────────────────────────────────┤
│  Infrastructure    (db.Pool, redis.Client, logger, config)         │
└─────────────────────────────────────────────────────────────────────┘
```

**Key patterns:**
- **Interface boundaries**: `mailstore.Store` and `contacts.Store` are canonical interfaces; only `PGStore` implementations exist today, but the seam is real.
- **Dependency injection via constructors**: every handler and service receives its dependencies explicitly (`NewHandler(store, auth, redis)`).
- **No global state**: configuration, pools, and clients are wired in `main()` and passed down.
- **Context propagation**: `context.Context` flows from HTTP/IMAP/SMTP handlers through to store queries.

---

## Data Flow

### Inbound Mail (Postmark → Webhook → Worker → PostgreSQL)

```
Postmark Inbound
      │
      ▼
POST /webhooks/postmark/inbound
      │
      ▼
webhook.Handler.verify() ──► webhook.Handler.dedup() ──► webhook.Handler.enqueue()
      │
      ▼
Redis queue:jobs
      │
      ▼
worker Pool.Dequeue()
      │
      ▼
InboundProcessor.Process()
      │
      ├──► auth.GetDomainByName() / auth.GetUserByEmail()
      ├──► mailstore.FindOrCreateThread()
      ├──► mailstore.GetLabelByName("INBOX")
      └──► mailstore.CreateMessage() + attachments
```

### Outbound Mail (SMTP/REST → Postmark)

**SMTP path:**
```
Client SMTP submission (AUTH PLAIN)
      │
      ▼
smtp.Server ──► smtpSession.Auth() ──► auth.Authenticate()
      │
      ▼
smtpSession.Mail() ──► auth.IsDomainMember()
      │
      ▼
smtpSession.Data() ──► parse RFC822 (go-message)
      │
      ├──► postmark.SendEmail() via domain token
      └──► mailstore.CreateMessage() with SENT label
```

**REST draft-send path:**
```
POST /api/v1/drafts/{id}/send
      │
      ▼
webmail.Handler.sendDraft() ──► redis.Enqueue("send_draft")
      │
      ▼
worker Pool ──► SendProcessor.Process()
      │
      ├──► mailstore.GetMessage()
      ├──► postmark.SendEmail()
      └──► mailstore.UpdateMessage() (clear draft, set outbound)
```

### Webmail REST API

```
Client HTTP
      │
      ▼
chi.Router
      │
      ├──► api.RequestID / StructuredLogger / Recovery / CORS / RateLimiter
      │
      ├──► Public: /healthz, /webhooks/*
      │
      ├──► Authenticated (api.RequireSession):
      │      └──► webmail.RegisterRoutes()
      │             ├── labels CRUD
      │             ├── messages CRUD + batch + labels
      │             ├── threads
      │             ├── drafts
      │             └── search
      │
      └──► Admin (api.RequireSession + api.RequireDomainAdmin):
             └──► /admin/api/v1/domains
```

---

## Abstractions & Key Interfaces

### `mailstore.Store` (`internal/mailstore/mailstore.go`)

The canonical mail persistence contract. ~20 methods covering:
- Messages: Create, Get, List, Update (patch), Delete, Move
- Labels: Create, Get, Update, Delete
- Labeling: ApplyLabels, GetMessageLabels
- Threads: GetThread, FindOrCreateThread
- Attachments: Create, Get
- Flags: Set/Clear/Get (batch)
- Search: full-text via PostgreSQL `tsvector`
- Counters: unread/total by label

**Implementation**: `mailstore.PGStore` (`pgstore.go`) — direct SQL via `pgxpool`, no ORM.

### `contacts.Store` (`internal/contacts/contacts.go`)

Contact persistence contract:
- Create, GetByID, GetByEmail, List, Delete

**Implementation**: `contacts.PGStore` — upsert-on-conflict for sync semantics.

### `auth.Service` (`internal/auth/auth.go`)

Monolithic auth service handling:
- Argon2id password hashing
- Session creation/validation (SHA-256 token hash in DB)
- API key validation
- User/Domain CRUD helpers
- Role checking (`IsDomainAdmin`, `IsDomainMember`)

Not an interface today, but wired as a concrete type consumed by handlers.

### `postmark.Client` (`internal/postmark/postmark.go`)

Thin wrapper over `mrz1836/postmark`:
- `SendEmail(ctx, apiToken, *OutboundMessage) (*SendResponse, error)`
- Inbound webhook struct definitions + `ParseInbound()`

### Worker Pool (`internal/workers/workers.go`)

Redis-backed job queue with:
- Typed processors (`Processor` interface)
- Retry with exponential backoff (delayed sorted set)
- Dead-letter queue after max attempts
- Concurrency via goroutine pool

Registered processors:
| Job Type | Processor | Purpose |
|----------|-----------|---------|
| `inbound` | `InboundProcessor` | Parse Postmark inbound, store message |
| `bounce` | `BounceProcessor` | Update delivery log, create bounce event |
| `delivery` | `DeliveryProcessor` | Mark delivery log as delivered |
| `send_draft` | `SendProcessor` | Relay draft to Postmark, update status |

---

## Protocol Adapters

### IMAP (`internal/imap/`)

Wraps `github.com/emersion/go-imap/server`.
- `imapBackend` authenticates via `auth.Service`
- `imapUser` exposes labels as mailboxes
- `imapMailbox` implements FETCH, SEARCH, APPEND, EXPUNGE, COPY, flags
- UID validity derived from label ID bytes; UIDs from message ID bytes

### SMTP (`internal/smtp/`)

Wraps `github.com/emersion/go-smtp`.
- AUTH PLAIN + LOGIN mechanisms
- Domain membership check before accepting MAIL FROM
- Immediate foreground relay to Postmark (not queued)
- Parses RFC822 via `go-message/mail`; stores sent copy

### DAV (`internal/dav/`)

Wraps `github.com/emersion/go-webdav`:
- **CardDAV**: full vCard 3.0/4.0 read/write via `carddav.Handler`
- **CalDAV**: stub backend (returns `not implemented`)
- Basic auth against `auth.Authenticate()`

---

## Configuration System

Two-tier config (`internal/config/`):
1. **TOML file** at `/etc/postnest/postnest.conf` (or `POSTNEST_CONFIG_PATH`)
2. **Environment overrides** via `POSTNEST_<SECTION>_<KEY>` (e.g., `POSTNEST_DATABASE_DSN`)
3. **Legacy env fallback** for pre-TOML deployments (e.g., `POSTGRES_DSN`)

Validation enforces required fields (database DSN, session key) at load time.

---

## Security & Middleware

HTTP middleware stack (`internal/api/middleware.go`):
1. `RequestID` — injects/copies `X-Request-ID`
2. `StructuredLogger` — JSON request logging via slog
3. `Recovery` — panic catch → 500
4. `CORS` — origin-restricted preflight
5. `RateLimiter` — per-IP token bucket (in-memory, not distributed)

Auth modes:
- Session cookie (`session`)
- Bearer token (`Authorization: Bearer <token>`)
- API key (falls back from Bearer check)
- Basic auth (DAV only)

Password hashing: Argon2id with configurable time/memory/threads.

---

## TLS Strategy (cmd/server/main.go)

Three mutually exclusive modes selected at startup:
1. **ACME** (`ACMEEnabled=true`): lego-based DNS-01 (Cloudflare) with auto-renewal loop
2. **Static certificates** (`TLSCertPath` + `TLSKeyPath`): loaded at startup
3. **No TLS**: plaintext IMAP/SMTP with `AllowInsecureAuth=true`

TLS config is shared across HTTP (if ever enabled), IMAP, and SMTP.

---

## Database & Migrations

- **Driver**: `pgx/v5` with connection pool wrapper (`internal/db/db.go`)
- **Migrations**: embedded SQL files via `golang-migrate/migrate/v4` + `embed`
- ** FTS**: PostgreSQL `tsvector` with weighted fields (subject A, from B, plain_text C)
- **Schema**: multi-tenant with `domain_id` + `user_id` on every owned table

Migration files (`internal/migrate/migrations/`):
| File | Purpose |
|------|---------|
| `000001_init.up.sql` | Base schema (users, domains, messages, labels, contacts, delivery_logs, etc.) |
| `000002_fts.up.sql` | Full-text search vector column + GIN index |
| `000003_seed_labels.up.sql` | System label seeding (INBOX, SENT, DRAFTS, TRASH, JUNK, etc.) |
| `000004_fts_trigger.up.sql` | Trigger to auto-update search_vector |
| `000005_search_composite.up.sql` | Composite indexes for search performance |

---

## Error Handling

Unified `api.AppError` type (`internal/api/errors.go`) with:
- `Code` string for programmatic handling
- `StatusCode` HTTP mapping
- `Details` for validation field errors
- Sentinel errors: `ErrNotFound`, `ErrUnauthorized`, `ErrForbidden`, `ErrValidation`, `ErrRateLimited`, `ErrInternal`

`api.WriteError()` unwraps via `errors.As` and writes JSON. Panics are recovered in middleware and also serialized as `ErrInternal`.

---

## Observability

- **Logging**: structured JSON via `log/slog` (`internal/logger/logger.go`)
- **Health**: `/healthz` checks PostgreSQL ping + Redis ping; returns `ok` or `degraded` (503)
- **Request tracing**: `X-Request-ID` propagated via context

---

## Scaling Characteristics

- **Server** is horizontally stateless except for in-memory rate limiter (not distributed)
- **Worker** scales horizontally by adding more `cmd/worker` instances (all consume same Redis queue)
- **Database** supports read-replica DSN (`PostgresReadDSN`) but not yet wired to stores
- **Redis** is the single shared state between server and worker nodes
