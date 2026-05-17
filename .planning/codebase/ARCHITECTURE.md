# Postnest Architecture

## Overview

Postnest is a Go-based email platform that provides webmail, IMAP/SMTP access, CardDAV contacts, and webhook-driven mail processing. It acts as a domain-aware mail host backed by Postmark for outbound delivery and inbound ingestion.

## Architectural Patterns

### Layered Hexagonal-ish Design
The codebase follows a layered pattern with explicit interfaces at storage boundaries:

- **Transport layer** (`cmd/server`, `cmd/worker`): Entry points that wire dependencies.
- **Handler layer** (`internal/api`, `internal/webmail`, `internal/webhook`, `internal/dav`, `internal/imap`, `internal/smtp`): Protocol-specific adapters.
- **Service layer** (`internal/auth`, `internal/reputation`): Domain logic, authentication, and policy engines.
- **Store layer** (`internal/mailstore`, `internal/contacts`): Persistence interfaces with PostgreSQL implementations.
- **Infrastructure layer** (`internal/db`, `internal/redis`, `internal/postmark`, `internal/certmanager`, `internal/logger`): External systems and shared utilities.

### Interface-Driven Stores
Storage is abstracted behind Go interfaces:
- `mailstore.Store` — message, label, thread, attachment, delivery log, and search operations.
- `contacts.Store` — CRUD for address book entries.

This allows the same business logic to be exercised by HTTP handlers, IMAP backends, SMTP backends, and background workers without coupling to PostgreSQL.

### Dependency Injection via Constructor Functions
Every component is initialized in `main()` and wired together explicitly:
```
db.Pool -> auth.Service -> mailstore.PGStore -> webmail.Handler
```
There is no global state, no service locator, and no magic container.

### Config Cascade
Configuration is loaded in three stages:
1. Hard-coded defaults in `internal/config/loader.go`.
2. TOML file at `/etc/postnest/postnest.conf` (or `POSTNEST_CONFIG_PATH`).
3. Environment variable overrides with `POSTNEST_<SECTION>_<KEY>` naming and legacy fallback mapping.

## Data Flow

### Inbound Mail (Postmark Webhook)
```
Postmark Inbound Webhook
    -> POST /webhooks/postmark/inbound
    -> webhook.Handler (dedup via Redis, enqueue)
    -> Redis queue:jobs
    -> worker Pool (InboundProcessor)
    -> reputation.Engine.EvaluateInbound (whitelist/blacklist/greylist)
    -> mailstore.PGStore.CreateMessage
    -> PostgreSQL (messages, message_labels, attachments, threads)
    -> search vector update
```

### Outbound Mail (SMTP Client Submission)
```
SMTP client (Thunderbird, etc.)
    -> SMTP server (go-smtp)
    -> smtpBackend.Auth (PLAIN / LOGIN via auth.Service)
    -> smtpBackend.Mail (domain membership check)
    -> smtpBackend.Data (parse MIME, extract text/html/attachments)
    -> postmark.Client.SendEmail
    -> Postmark API
    -> mailstore.PGStore.CreateMessage (SENT label)
```

### Outbound Draft Send (Web)
```
Web client
    -> POST /api/v1/drafts/{id}/send
    -> webmail.Handler.sendDraft
    -> Redis queue:jobs (job type: send_draft)
    -> worker Pool (SendProcessor)
    -> postmark.Client.SendEmail
    -> mailstore.PGStore.UpdateMessage (clear IsDraft, set IsOutbound, mailbox=SENT)
    -> mailstore.PGStore.CreateDeliveryLog
```

### Bounce / Delivery Tracking
```
Postmark Webhook (bounce or delivery)
    -> POST /webhooks/postmark/{bounce,delivery}
    -> webhook.Handler (dedup, enqueue)
    -> worker Pool (BounceProcessor / DeliveryProcessor)
    -> PostgreSQL delivery_logs UPDATE
```

### IMAP Access
```
IMAP client
    -> IMAP server (go-imap)
    -> imapBackend.Login -> auth.Service.Authenticate
    -> imapUser.ListMailboxes -> mailstore.GetLabels
    -> imapMailbox.ListMessages -> mailstore.ListMessages
    -> PostgreSQL
```

### CardDAV Access
```
CardDAV client
    -> HTTP Basic Auth -> auth.Service.Authenticate
    -> carddav.Handler -> carddavBackend
    -> contacts.Store (PGStore)
    -> PostgreSQL (contacts table)
```

## Entry Points

### cmd/server/main.go
Wires and starts all long-running server processes:
1. Loads config.
2. Opens PostgreSQL pool and Redis client.
3. Constructs `auth.Service`, `mailstore.PGStore`, `contacts.PGStore`.
4. Starts HTTP server (Chi) with middleware, health checks, webmail API, admin API, webhook routes, DAV routes.
5. Configures TLS strategy (ACME, static cert, or plaintext).
6. Starts IMAP server.
7. Starts SMTP server.
8. Blocks on OS signal, then gracefully shuts down all subsystems.

### cmd/worker/main.go
Wires and starts the background job pool:
1. Loads config.
2. Opens PostgreSQL pool and Redis client.
3. Constructs `auth.Service`, `mailstore.PGStore`, `reputation.Engine`.
4. Creates `workers.Pool` with concurrency and poll interval.
5. Registers processors: `inbound`, `bounce`, `delivery`, `send_draft`, `spam`.
6. Starts pool (blocking).
7. On OS signal, stops pool with timeout.

## Key Abstractions

### mailstore.Store
The canonical storage interface for the mail subsystem. It exposes operations for messages, labels, threads, attachments, flags, and full-text search. All protocol handlers (HTTP, IMAP, SMTP) and workers program against this interface.

### auth.Service
Central identity and session manager. Handles Argon2id password hashing, session/API-key generation, validation, and domain membership/role checks. Used by HTTP middleware, IMAP/SMTP backends, and worker processors.

### workers.Pool / Processor
A Redis-backed job queue with pluggable processors. Each job type maps to a `Processor` implementation. The pool supports delayed retries, dead-lettering, and concurrent workers.

### postmark.Client
Thin wrapper around the `mrz1836/postmark` library. Normalizes send requests and inbound payload parsing. Isolates the rest of the codebase from Postmark-specific types.

### certmanager.Manager
ACME certificate lifecycle using `lego`. Handles account registration, DNS-01 challenge (Cloudflare), certificate obtainment, loading, and background renewal. Exposes `GetCertificate` for `tls.Config`.

## Middleware Stack (HTTP)

Applied in order:
1. `api.RequestID` — injects/copies `X-Request-ID`.
2. `api.StructuredLogger` — JSON request logging via `slog`.
3. `api.Recovery` — catches panics, returns 500.
4. `api.CORS` — origin-restricted CORS headers.
5. `api.RateLimiter` — per-IP token-bucket rate limiting (100/min) with periodic stale-entry cleanup.
6. `api.RequireSession` — Bearer or cookie session validation.
7. `api.RequireDomainAdmin` — domain-scoped RBAC.

## Authentication Models

- **Session cookie** (`HttpOnly`, `Secure`, `SameSite=Lax`) for browser clients.
- **Bearer token** (`Authorization: Bearer <token>`) for API clients.
- **Basic Auth** for DAV endpoints.
- **SMTP AUTH** (PLAIN, LOGIN) for mail submission.

Tokens are 32-byte random values stored as SHA-256 hashes in `auth_sessions`. Two session types exist: `session` and `api_key`.

## Database Patterns

- **Raw SQL with pgx**: No ORM. Queries are hand-written, parameterized, and scanned into `internal/models` structs.
- **Transactions**: Critical paths (message creation with labels/attachments, label application) use explicit `pgx.Tx` with deferred rollback.
- **CopyFrom**: Bulk attachment inserts use `pgx.CopyFrom` for efficiency.
- **Full-text search**: PostgreSQL `tsvector` with weighted fields (subject A, from B, plain text C, to D).
- **Migrations**: Embedded SQL files via `golang-migrate/migrate` and `embed.FS`.

## Security Considerations

- Passwords hashed with **Argon2id** (configurable time, memory, threads).
- Rate limiting at the HTTP edge.
- HTML sanitization via `bluemonday.UGCPolicy` for inbound HTML and drafts.
- Reputation engine evaluates whitelist, blacklist, and greylist for inbound messages.
- SMTP sessions require authentication and domain authorization before accepting mail.
- TLS is strongly encouraged: ACME or static certificates; plaintext mode allows insecure auth only for local testing.
