# Architecture

**Analysis Date:** 2026-05-18

## Pattern Overview

**Overall:** Modular monolith with domain-driven internal packages, multi-binary Go deployment, and a separate React SPA frontend proxy.

**Key Characteristics:**
- Five standalone binaries built from `cmd/*` entry points sharing `internal/` packages
- Domain-scoped multi-tenancy: users belong to domains via `domain_members`; most data is scoped to `(domain_id, user_id)`
- Interface-driven persistence: `mailstore.Store`, `contacts.Store`, `calendar.Store` define contracts; `PGStore` implements them
- Redis-backed job queue for asynchronous worker processing
- Dual protocol support: HTTP REST API + raw IMAP/SMTP servers

## Layers

**Entry Points / Commands:**
- Purpose: Standalone deployable binaries
- Location: `cmd/`
- Contains: `main.go` files for server, worker, webui, admin CLI, migrate CLI
- Depends on: internal packages
- Used by: Operations/deployment

**API / Transport Layer:**
- Purpose: HTTP handlers, middleware, routing, protocol adapters
- Location: `internal/api/`, `internal/webmail/`, `internal/calendar/`, `internal/contacts/`, `internal/webhook/`, `internal/dav/`, `internal/imap/`, `internal/smtp/`, `internal/webui/`
- Contains: Chi router handlers, Gin router for webui proxy, IMAP/SMTP backends, CardDAV/CalDAV adapters
- Depends on: auth service, stores, models, redis, postmark
- Used by: Entry points

**Service Layer:**
- Purpose: Business logic, authentication, orchestration
- Location: `internal/auth/`, `internal/workers/`, `internal/reputation/`, `internal/certmanager/`
- Contains: Auth service with Argon2id hashing, session management, worker pool with job processors, reputation engine, ACME certificate manager
- Depends on: db, redis, models
- Used by: API layer, entry points

**Data / Persistence Layer:**
- Purpose: Database access, caching, external API clients
- Location: `internal/db/`, `internal/redis/`, `internal/mailstore/`, `internal/postmark/`, `internal/search/`
- Contains: pgx pool wrapper, Redis queue client, mailstore interface + PGStore, Postmark client wrapper
- Depends on: models
- Used by: Service layer, API layer

**Models:**
- Purpose: Domain entities shared across all layers
- Location: `internal/models/`
- Contains: User, Domain, DomainMember, Message, Label, Attachment, Thread, Contact, DeliveryLog, AuthSession, Calendar, CalendarEvent
- Depends on: uuid
- Used by: All layers

**Configuration:**
- Purpose: Environment-based configuration loading
- Location: `internal/config/`
- Contains: `Config` struct, `Load()` from env vars, template helpers
- Depends on: none
- Used by: All entry points

**Infrastructure:**
- Purpose: Logging, migrations
- Location: `internal/logger/`, `internal/migrate/`
- Contains: Structured slog JSON logger, golang-migrate wrapper
- Depends on: none
- Used by: All entry points

## Data Flow

**Inbound Email (Postmark Webhook):**
1. Postmark sends webhook to `POST /webhooks/postmark/inbound` (`internal/webhook/webhook.go`)
2. Webhook handler verifies HMAC signature, deduplicates via Redis, enqueues job to `queue:jobs`
3. Worker binary (`cmd/worker/main.go`) polls Redis and routes to `workers.NewInboundProcessor`
4. Inbound processor parses payload, stores message via `mailstore.Store.CreateMessage`
5. Message appears in webmail inbox with INBOX label

**Outbound Email (SMTP Submit):**
1. Mail client connects to IMAP/SMTP server (`cmd/server/main.go`)
2. SMTP backend (`internal/smtp/smtp.go`) authenticates via `auth.Service.Authenticate`
3. On DATA, parses MIME, validates sender domain membership, forwards to Postmark
4. Stores copy in SENT mailbox via `mailstore.Store.CreateMessage`

**Webmail Compose (Draft + Send):**
1. Frontend POST `/api/v1/drafts` -> `webmail.Handler.createDraft`
2. Draft stored with `is_draft=true`, `mailbox=DRAFTS`
3. Frontend POST `/api/v1/drafts/{id}/send` -> enqueues `send_draft` job to Redis
4. Worker `send_draft` processor (`internal/workers/send.go`) fetches draft, sends via Postmark, updates to non-draft

**CalDAV/CardDAV:**
1. DAV client authenticates with Basic Auth via `internal/dav/dav.go` middleware
2. `carddavBackend`/`caldavBackend` translate DAV operations to `contacts.Store`/`calendar.Store`
3. Data persisted in PostgreSQL

**State Management:**
- Server-side: PostgreSQL for persistent state, Redis for job queues and deduplication
- Client-side: React SPA with local component state (no global state library)
- Real-time: SSE hub in webui proxy (`internal/webui/sse.go`) subscribes to Redis pub/sub

## Key Abstractions

**mailstore.Store:**
- Purpose: Canonical persistence contract for all mail data
- Examples: `internal/mailstore/mailstore.go` (interface), `internal/mailstore/pgstore.go` (implementation)
- Pattern: Interface with PGStore implementing ~25 methods for messages, labels, attachments, threads, delivery logs

**auth.Service:**
- Purpose: Authentication, session management, domain authorization
- Examples: `internal/auth/auth.go`
- Pattern: Service struct with pgxpool, holds Argon2id parameters and session key; provides `Authenticate`, `ValidateSession`, `IsDomainAdmin`

**api.AppError:**
- Purpose: Unified error type for all HTTP handlers
- Examples: `internal/api/errors.go`
- Pattern: Struct with Code, Message, Details, StatusCode; predeclared errors like `ErrNotFound`, `ErrUnauthorized`; `WriteError` renders JSON

**workers.Processor / workers.Pool:**
- Purpose: Pluggable background job processing
- Examples: `internal/workers/workers.go`
- Pattern: Pool polls Redis list, dispatches to registered `Processor` by job type, handles retries with exponential backoff and dead-letter queue

**Domain + User Scoping:**
- Purpose: Multi-tenant data isolation
- Examples: Every store method signature includes `(ctx, domainID, userID, ...)`
- Pattern: UUID pair `(domain_id, user_id)` scopes all queries; users can belong to multiple domains via `domain_members` with roles (admin, user, readonly)

## Entry Points

**cmd/server/main.go:**
- Location: `cmd/server/main.go`
- Triggers: Direct execution, Docker container, systemd
- Responsibilities: Load config, connect Postgres + Redis, wire all services, start HTTP API server on `:8080`, start IMAP on `:143/:993`, start SMTP on `:587/:465`, handle TLS (static or ACME), graceful shutdown on SIGTERM

**cmd/worker/main.go:**
- Location: `cmd/worker/main.go`
- Triggers: Direct execution, separate worker container
- Responsibilities: Load config, connect Postgres + Redis, create worker pool, register 5 processors (inbound, bounce, delivery, send_draft, spam), consume from Redis queue

**cmd/webui/main.go:**
- Location: `cmd/webui/main.go`
- Triggers: Direct execution
- Responsibilities: Start Gin server on `:3000`, serve embedded React SPA (`internal/webui/dist`), proxy API routes to backend, run SSE hub

**cmd/admin/main.go:**
- Location: `cmd/admin/main.go`
- Triggers: CLI invocation
- Responsibilities: Bootstrap operations: create-user, create-domain, add-member, setup (all-in-one), reset-password

**cmd/migrate/main.go:**
- Location: `cmd/migrate/main.go`
- Triggers: CLI invocation, init containers
- Responsibilities: Run golang-migrate up/down/version/force against Postgres DSN

## Error Handling

**Strategy:** Unified `api.AppError` type with structured JSON responses. Lower layers return plain `error`; handlers map to `AppError` via `api.WriteError`.

**Patterns:**
- `api.As(err, &appErr)` attempts to cast to `AppError`; unhandled errors map to `ErrInternal` (500)
- pgx `ErrNoRows` typically mapped to `ErrNotFound` in handler layers
- SMTP/IMAP backends return protocol-specific error structs (`smtp.SMTPError`) with enhanced status codes
- Panics recovered by `api.Recovery` middleware, logged with stack trace, returned as 500

## Cross-Cutting Concerns

**Logging:** Structured JSON logging via `internal/logger/logger.go` using `log/slog`. Every request logged with method, path, duration, request_id. Worker jobs log failures with type and error.

**Validation:** Minimal validation in handlers: JSON decode + manual field checks. `api.NewValidationError` returns structured field errors. Email addresses validated with `net/mail.ParseAddress`. HTML sanitized with `bluemonday.UGCPolicy()`.

**Authentication:** Session cookies (HttpOnly, Secure, SameSite=Strict) or Bearer tokens. CSRF token required for mutating requests. DAV uses Basic Auth against same `auth.Service`. IMAP/SMTP use SASL PLAIN/LOGIN against same `auth.Service`. Sessions stored as SHA256(token_hash) in `auth_sessions` table.

**Rate Limiting:** In-memory per-IP token bucket in `api.RateLimiter` (100 requests/minute). No distributed rate limiting.

**CORS:** Origin-allowlist comparison in both server (`api.CORS`) and webui (`corsMiddleware`).

---

*Architecture analysis: 2026-05-18*
