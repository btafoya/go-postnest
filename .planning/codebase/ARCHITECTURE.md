# Architecture

**Analysis Date:** 2026-05-18

## Pattern Overview

**Overall:** Modular monolith with protocol-facing gateways and interface-driven stores.

**Key Characteristics:**
- Multiple compiled binaries (`server`, `webui`, `worker`, `admin`, `migrate`) share `internal/` packages.
- Store interfaces abstract PostgreSQL persistence; implementations live in the same package (`mailstore.Store` -> `PGStore`).
- No ORM; all SQL is explicit via `pgx/v5`.
- Redis is used exclusively for job queues, delayed retries, and SSE pub/sub — not primary storage.
- Postmark is the single external mail transport; no local MTA.
- React SPA is embedded into the `webui` binary via `go:embed` and reverse-proxies API calls to the `server` binary.

## Layers

**Entry Points (cmd/):**
- Purpose: Compile-time boundaries for deployment roles.
- Location: `cmd/server/`, `cmd/webui/`, `cmd/worker/`, `cmd/admin/`, `cmd/migrate/`
- Contains: `main.go` binaries only.
- Depends on: All `internal/` packages.
- Used by: Operating system / container orchestrator.

**Models:**
- Purpose: Canonical domain types used across all packages.
- Location: `internal/models/models.go`
- Contains: `User`, `Domain`, `Message`, `Label`, `Attachment`, `Thread`, `Contact`, `Calendar`, `CalendarEvent`, `DeliveryLog`, `AuthSession`.
- Depends on: `github.com/google/uuid`.
- Used by: Every internal package.

**Configuration:**
- Purpose: Unified TOML + environment variable loading with backward-compatible legacy env names.
- Location: `internal/config/config.go`, `internal/config/loader.go`
- Contains: `Config` struct, `Loader`, `applyEnvOverrides` reflect-based env mapping.
- Depends on: `os`, `reflect`, `github.com/BurntSushi/toml`.
- Used by: All `cmd/*` binaries.

**Infrastructure:**
- Purpose: Wrapped external clients with lifecycle helpers.
- Location: `internal/db/db.go`, `internal/redis/redis.go`, `internal/logger/logger.go`
- Contains: `db.Pool` (pgxpool wrapper), `redis.Client` (go-redis wrapper with queue helpers), `slog` JSON logger.
- Depends on: `pgx/v5`, `go-redis/v9`.
- Used by: Services and stores.

**Auth Service:**
- Purpose: Authentication, sessions, password hashing, domain membership.
- Location: `internal/auth/auth.go`
- Contains: `auth.Service` with Argon2id hashing, session token creation/validation, domain queries.
- Depends on: `pgxpool`, `models`, `golang.org/x/crypto/argon2`.
- Used by: `api`, `webmail`, `smtp`, `imap`, `dav`, `admin`, `workers`.

**Stores:**
- Purpose: Persistence interfaces and PostgreSQL implementations.
- Location: `internal/mailstore/`, `internal/calendar/`, `internal/contacts/`, `internal/admin/`
- Contains:
  - `mailstore.Store` interface + `PGStore`
  - `calendar.Store` interface + `PGStore`
  - `contacts.Store` interface + `PGStore`
  - `admin.Store` interface + `PGStore`
- Depends on: `pgxpool`, `models`.
- Used by: Handlers, IMAP backend, SMTP backend, DAV backends, workers.

**Protocol Gateways:**
- Purpose: Accept client connections and translate to store operations.
- Location: `internal/imap/backend.go`, `internal/smtp/smtp.go`, `internal/dav/dav.go`
- Contains:
  - IMAP4rev1 backend using `go-imap` (labels as mailboxes, UID derived from UUID prefix).
  - SMTP submission proxy using `go-smtp` (AUTH PLAIN/LOGIN, validates domain membership, sends via Postmark, persists to Sent).
  - CardDAV/CalDAV handler using `go-webdav` with Basic Auth.
- Depends on: `mailstore`, `auth`, `calendar`, `contacts`, `postmark`.
- Used by: `cmd/server`.

**HTTP API:**
- Purpose: REST endpoints for the SPA and external integrations.
- Location: `internal/api/`, `internal/webmail/`, `internal/calendar/handler.go`, `internal/contacts/handler.go`, `internal/admin/handler.go`, `internal/webhook/webhook.go`
- Contains:
  - `api`: middleware (RequestID, logging, recovery, CORS, rate limiter, session/auth, CSRF), error types.
  - `webmail`: labels, messages, threads, drafts, attachments, search, batch operations.
  - `calendar`: calendars, events (with auto-created default calendar).
  - `contacts`: CRUD for address book entries.
  - `admin`: domain/user/settings management (requires domain admin role).
  - `webhook`: Postmark inbound/bounce/delivery/spam signature verification and job enqueueing.
- Depends on: `chi/v5`, `models`, `auth`, `mailstore`, `redis`.
- Used by: `cmd/server`, `cmd/webui` (proxy).

**Worker Pool:**
- Purpose: Background job processing with retry and dead-letter support.
- Location: `internal/workers/workers.go`, `internal/workers/inbound.go`, `internal/workers/send.go`, `internal/workers/bounce.go`, `internal/workers/delivery.go`, `internal/workers/spam.go`
- Contains:
  - `workers.Pool`: Redis-backed queue consumer with concurrency, delayed jobs, dead-letter queue.
  - `InboundProcessor`: Parses Postmark inbound, creates messages/threads/attachments, applies reputation.
  - `SendProcessor`: Fetches draft + attachments, sends via Postmark, updates delivery log.
  - `BounceProcessor`, `DeliveryProcessor`, `SpamProcessor`: Update delivery logs and reputation.
- Depends on: `redis`, `mailstore`, `auth`, `postmark`, `reputation`.
- Used by: `cmd/worker`.

**Cross-Cutting:**
- Purpose: Shared non-functional concerns.
- Location: `internal/search/search.go`, `internal/reputation/reputation.go`, `internal/certmanager/manager.go`, `internal/postmark/postmark.go`
- Contains:
  - `search.Indexer`: Direct `tsvector` update on PostgreSQL (no external search engine).
  - `reputation.Engine`: Whitelist/blacklist/greylist evaluation.
  - `certmanager.Manager`: ACME (Let's Encrypt) via `go-acme/lego/v4` with DNS-01 challenge.
  - `postmark.Client`: Thin wrapper around `mrz1836/postmark` for outbound send and inbound parsing.

## Data Flow

**Inbound Mail:**

1. Postmark receives email and POSTs to `/webhooks/postmark/inbound`
2. `webhook.Handler` verifies HMAC-SHA256 signature (or legacy token), deduplicates via Redis `SETNX`, enqueues Redis job.
3. `worker.Pool` dequeues job and hands to `InboundProcessor`.
4. `InboundProcessor` resolves recipient domain/user, evaluates `reputation.Engine`, parses MIME/attachments, calls `mailstore.CreateMessage`.
5. `mailstore.PGStore` inserts message + labels + attachments in a transaction.
6. `search.Indexer.Queue` updates `tsvector` column.
7. Message is visible via IMAP `ListMessages`, webmail REST, or DAV.

**Outbound Mail (Webmail):**

1. Frontend POSTs draft to `/api/v1/drafts`.
2. `webmail.Handler` creates draft message in `mailstore` (DRAFTS label).
3. User clicks Send -> frontend POSTs `/api/v1/drafts/{id}/send`.
4. `webmail.Handler` enqueues `send_draft` Redis job.
5. `worker.Pool` hands to `SendProcessor`.
6. `SendProcessor` fetches draft + attachments, builds `postmark.OutboundMessage`, calls `postmark.Client.SendEmail`.
7. On success, updates message state (removes draft flag, adds SENT label) and creates `DeliveryLog`.

**Outbound Mail (SMTP):**

1. Client connects to SMTP server (`internal/smtp/smtp.go`).
2. `smtpBackend` authenticates via `auth.Service`.
3. `smtpSession.Data` reads RFC822 body, parses MIME with `go-message/mail`, builds `postmark.OutboundMessage`.
4. Calls `postmark.Client.SendEmail`.
5. On success, persists copy to Sent via `mailstore.CreateMessage`.

**Web UI:**

1. Browser loads `webui` binary (embedded SPA assets).
2. `webui` Gin router serves static files; all `/api/*`, `/auth/*`, `/webhooks/*`, `/.well-known/*`, `/dav/*` routes reverse-proxy to `server` binary via `httputil.ReverseProxy`.
3. Frontend establishes SSE connection to `/events` on `webui` for real-time updates (Redis pub/sub -> SSE hub).
4. Frontend uses `axios` with `withCredentials` and CSRF double-submit (`X-CSRF-Token`).

## Key Abstractions

**Store Interface Pattern:**
- Purpose: Decouple handlers/protocol backends from PostgreSQL.
- Examples: `internal/mailstore/mailstore.go` (interface), `internal/calendar/calendar.go` (interface).
- Pattern: Each domain package defines a `Store` interface and a `PGStore` struct implementing it with explicit SQL.

**Message Patch Pattern:**
- Purpose: Support partial updates without nil-vs-zero-value ambiguity.
- Examples: `mailstore.MessagePatch` with pointer fields (`*bool`, `*string`).
- Pattern: Handlers decode JSON into pointer structs; store builds dynamic `UPDATE` clauses based on non-nil fields.

**Domain Scoping Pattern:**
- Purpose: Every operation is scoped to a `(domain_id, user_id)` pair.
- Examples: `mailstore.ListMessages(ctx, domainID, userID, ...)`; `contacts.Store.List(ctx, domainID, userID, ...)`.
- Pattern: Handlers resolve the user's primary domain at request time (from `domain_members` table) and pass it to stores.

**Job Queue Pattern:**
- Purpose: Async, retryable, idempotent background work.
- Examples: `workers.Job`, `workers.Processor` interface, `workers.Pool`.
- Pattern: Redis lists for ready jobs, sorted sets for delayed jobs, separate dead-letter list. Max 3 attempts with linear backoff.

## Entry Points

**HTTP/API Server:**
- Location: `cmd/server/main.go`
- Triggers: `go run ./cmd/server` or `server` binary.
- Responsibilities: Initializes `db.Pool`, `redis.Client`, all services/stores; starts chi HTTP server, IMAP server, SMTP server; mounts all route groups; handles graceful shutdown with 30s timeout.

**Web UI Server:**
- Location: `cmd/webui/main.go`
- Triggers: `go run ./cmd/webui` or `webui` binary.
- Responsibilities: Initializes Gin router with embedded SPA, SSE hub, reverse proxy to API server. Serves on `:3000` by default.

**Worker Pool:**
- Location: `cmd/worker/main.go`
- Triggers: `go run ./cmd/worker` or `worker` binary.
- Responsibilities: Initializes Redis, registers 5 job processors, starts concurrent consumers, blocks on OS signal, graceful shutdown.

**Admin CLI:**
- Location: `cmd/admin/main.go`
- Triggers: `go run ./cmd/admin <command>` or `admin` binary.
- Responsibilities: Direct PostgreSQL operations for user/domain/member creation and password resets. Seeds system labels on membership creation.

**Migrate CLI:**
- Location: `cmd/migrate/main.go`
- Triggers: `go run ./cmd/migrate` or `migrate` binary.
- Responsibilities: Runs `golang-migrate` up migrations from `internal/migrate/migrations/`.

## Error Handling

**Strategy:** Explicit error types in `internal/api/errors.go`, returned as structured JSON with HTTP status codes.

**Patterns:**
- `api.AppError` carries `Code`, `Message`, `StatusCode`.
- `api.WriteError(w, err)` inspects error type to determine HTTP status.
- Validation errors use `api.NewValidationError([]api.FieldError{...})`.
- Protocol backends (IMAP/SMTP) return protocol-specific error structs (e.g., `smtp.SMTPError`).
- Workers log errors and retry with backoff; dead-letter after max attempts.

## Cross-Cutting Concerns

**Logging:** Structured JSON logging via `internal/logger/logger.go` using `log/slog`. Every HTTP request is logged with method, path, duration, request_id.

**Validation:**
- Backend: Handlers decode JSON into structs and validate required fields inline. Email addresses validated with `net/mail.ParseAddress`.
- Frontend: `bluemonday.UGCPolicy().Sanitize()` applied to HTML draft bodies.

**Authentication:**
- HTTP: Session cookies (`session`) + CSRF cookies (`csrf`). Bearer token fallback from `Authorization` header.
- API keys stored in `auth_sessions` table with `type='api_key'`.
- IMAP/SMTP: Direct `auth.Service.Authenticate` with Argon2id password verification.
- DAV: HTTP Basic Auth against `auth.Service`.
- Role checks: `api.RequireDomainAdmin` middleware checks `domain_members.role`.

**Rate Limiting:** Simple per-IP token-bucket in memory (`api.RateLimiter`) on the HTTP router.

**TLS:**
- Three strategies: ACME (auto), static cert files, or insecure plaintext (development only, requires explicit env var).
- SMTP/IMAP listeners wrapped with `tls.NewListener` when configured.

---

*Architecture analysis: 2026-05-18*
