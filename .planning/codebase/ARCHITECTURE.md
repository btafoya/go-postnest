# Architecture

**Analysis Date:** 2025-06-10

## Pattern Overview

**Overall:** Multi-Protocol Monolithic Service with Background Workers

**Key Characteristics:**
- Three separate binaries (`server`, `worker`, `migrate`) sharing `internal/` packages
- Protocol-layer servers (IMAP, SMTP, HTTP) co-located in a single process
- PostgreSQL as the single source of truth; Redis for queues and pub/sub
- Multi-tenant by domain; users belong to one or more domains
- Background job processing via Redis-backed worker pool

## Layers

**Transport Layer (Protocol Servers):**
- Purpose: Accept client connections over IMAP4rev1, SMTP, and HTTP
- Contains: IMAP server (`internal/imap`), SMTP server (`internal/smtp`), HTTP router (`cmd/server/main.go`)
- Depends on: Service layer (`auth`, `mailstore`), API middleware (`internal/api`)
- Used by: External email clients, web browsers, DAV clients

**Handler Layer (Route/Request Handlers):**
- Purpose: HTTP route handlers for webmail API, webhooks, and DAV
- Contains: `internal/webmail`, `internal/webhook`, `internal/dav`
- Depends on: Service layer (`auth`, `mailstore`, `contacts`), `internal/api` middleware
- Used by: Transport layer (chi router in `cmd/server/main.go`)

**Service Layer (Business Logic):**
- Purpose: Core domain operations—authentication, mail persistence, contact management, reputation evaluation
- Contains: `internal/auth`, `internal/mailstore`, `internal/contacts`, `internal/reputation`, `internal/search`
- Depends on: Database (`internal/db`), models (`internal/models`), external APIs (`internal/postmark`)
- Used by: Handler layer, protocol servers, background workers

**Data/Integration Layer:**
- Purpose: Persistence, caching, external API clients, and infrastructure concerns
- Contains: `internal/db` (pgxpool wrapper), `internal/redis` (go-redis wrapper), `internal/postmark` (Postmark API client), `internal/certmanager` (ACME/Let's Encrypt)
- Depends on: External systems (PostgreSQL, Redis, Postmark API, ACME providers)
- Used by: Service layer, transport layer, workers

**Worker Layer (Background Processing):**
- Purpose: Async job processing for inbound mail, bounces, delivery tracking, spam evaluation, search indexing, and draft sending
- Contains: `internal/workers` (pool orchestration + processors)
- Depends on: Service layer (`mailstore`, `auth`, `reputation`, `postmark`), `internal/redis`
- Used by: Webhook receiver enqueues jobs; worker binary dequeues and processes

**Foundation Layer:**
- Purpose: Shared types, configuration, logging, and API primitives
- Contains: `internal/models`, `internal/config`, `internal/logger`, `internal/api` (errors, middleware)
- Depends on: Standard library and third-party utilities
- Used by: All other layers

## Data Flow

**Outbound Mail (SMTP Client → Postmark):**

1. Email client opens SMTP connection to `internal/smtp` (port 587/465)
2. `smtpBackend` authenticates via `auth.Service` (Argon2id password verification)
3. Client sends `MAIL FROM`, `RCPT TO`, `DATA`
4. `smtpSession.Data` parses MIME message, extracts text/html parts and attachments
5. Message stored in `mailstore` as draft with `SENT` label
6. `postmark.Client.SendEmail` relays to Postmark API
7. Delivery log created; worker later polls for status updates

**Inbound Mail (Postmark → IMAP/Webmail Client):**

1. Postmark delivers inbound email via HTTP POST to `internal/webhook`
2. Webhook validates HMAC-SHA256 signature (or server-token fallback)
3. Webhook enqueues `inbound` job to Redis queue (`queue:jobs`)
4. `InboundProcessor` worker dequeues job, parses RFC822 payload
5. `mailstore.CreateMessage` writes message, attachments, labels, thread
6. `reputation.Engine.EvaluateInbound` runs whitelist/blacklist/greylist rules
7. Redis pub/sub notifies IMAP IDLE clients of new mail
8. Webmail client sees new message on next API poll

**Webmail Read (Browser → REST API):**

1. Browser sends `GET /api/v1/messages?label_id=...` with session cookie
2. `api.RequireSession` middleware validates session or Bearer token via `auth.Service`
3. `webmail.Handler.listMessages` resolves domain/user from context
4. `mailstore.ListMessages` queries PostgreSQL with pagination
5. JSON response returned with messages and total count

**DAV Contact Sync (macOS/iOS → CardDAV):**

1. DAV client sends PROPFIND/REPORT to `/.well-known/carddav`
2. `internal/dav` auth middleware validates Basic/Bearer credentials
3. `carddavBackend` routes to `contacts.Store` CRUD operations
4. Contacts stored/retrieved as vCard in PostgreSQL

## Key Abstractions

**Service:**
- Purpose: Encapsulate business logic for a domain
- Examples: `auth.Service` (`internal/auth/auth.go`), `reputation.Engine` (`internal/reputation/reputation.go`)
- Pattern: Constructor-injected `*pgxpool.Pool`; methods accept `context.Context`

**Store (Repository Pattern):**
- Purpose: Abstract persistence behind an interface
- Examples: `mailstore.Store` (`internal/mailstore/mailstore.go`), `contacts.Store` (`internal/contacts/contacts.go`)
- Pattern: Interface + PostgreSQL implementation (`PGStore`); no mocking stubs in production

**Handler:**
- Purpose: HTTP request handling for a domain
- Examples: `webmail.Handler` (`internal/webmail/webmail.go`), `webhook.Handler` (`internal/webhook/webhook.go`)
- Pattern: Struct with service dependencies; `RegisterRoutes(r chi.Router)` method

**Processor:**
- Purpose: Background job handler
- Examples: `InboundProcessor`, `BounceProcessor`, `SendProcessor` (`internal/workers/*.go`)
- Pattern: Implements `workers.Processor` interface; registered by job type in `cmd/worker/main.go`

**Server Wrapper:**
- Purpose: Wrap third-party protocol servers with unified lifecycle
- Examples: `imap.Server` (`internal/imap/imap.go`), `smtp.Server` (`internal/smtp/smtp.go`)
- Pattern: Struct wrapping `go-imap`/`go-smtp` server; `Start()`/`Stop()` methods; TLS config injection

## Entry Points

**Server Entry (`cmd/server/main.go`):**
- Location: `cmd/server/main.go`
- Triggers: Direct execution or container start
- Responsibilities: Load config, open PostgreSQL + Redis connections, wire all services, start HTTP/IMAP/SMTP/DAV servers, handle graceful shutdown

**Worker Entry (`cmd/worker/main.go`):**
- Location: `cmd/worker/main.go`
- Triggers: Direct execution or container start
- Responsibilities: Load config, open connections, create `workers.Pool`, register all processors, start consuming jobs, handle shutdown

**Migrate Entry (`cmd/migrate/main.go`):**
- Location: `cmd/migrate/main.go`
- Triggers: CLI invocation (`postnest-migrate up|down|version|force`)
- Responsibilities: Load config, apply/rollback embedded SQL migrations via `golang-migrate`

## Error Handling

**Strategy:** Typed application errors at service boundary; HTTP status mapping in `internal/api/errors.go`; panics recovered by middleware

**Patterns:**
- `api.AppError` carries `Code`, `Message`, `StatusCode`, and optional `FieldError` details
- `api.WriteError(w, err)` inspects error type and writes appropriate JSON response
- `api.Recovery` middleware catches panics, logs stack trace, returns 500
- `mailstore.ErrNotFound` sentinel for missing records
- Workers log errors and retry with exponential backoff (max 3 attempts), then dead-letter

## Cross-Cutting Concerns

**Logging:**
- `internal/logger` produces structured JSON `slog.Logger`
- Injected into all services and handlers
- Request logging via `api.StructuredLogger` middleware (method, path, duration, request_id)

**Authentication:**
- Session cookies (`HttpOnly`, `Secure`, `SameSite=Lax`) or Bearer tokens
- `api.RequireSession` middleware validates via `auth.Service.ValidateSession` / `ValidateAPIKey`
- `api.RequireDomainAdmin` middleware for admin routes
- Argon2id for passwords, SHA-256 hash for session tokens

**Validation:**
- Manual validation in handlers (e.g., UUID parsing, required fields)
- `api.NewValidationError` for structured field-level errors

**Rate Limiting:**
- `api.RateLimiter` middleware: token-bucket per IP, 100 req/min default
- Periodic stale-entry cleanup to prevent unbounded map growth

**TLS/ACME:**
- Static TLS certs, ACME/Let's Encrypt with DNS-01, or unencrypted (dev only)
- `certmanager.Manager` handles certificate lifecycle and renewal

**Multi-Tenancy:**
- All data scoped by `domain_id` and `user_id`
- `domain_members` table links users to domains with roles (`admin`, `member`)
- `IsSuperAdmin` flag bypasses domain checks

---

*Architecture analysis: 2025-06-10*
*Update when major patterns change*
