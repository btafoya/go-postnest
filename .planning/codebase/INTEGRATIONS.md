# PostNest External Integrations

> Reference for all third-party APIs, protocol libraries, data stores, and authentication mechanisms used by PostNest.
> File paths are relative to the repository root.

## External APIs

### Postmark
- **Library**: `github.com/mrz1836/postmark` (v1.9.2).
- **Wrapper**: `internal/postmark/postmark.go` — Thin abstraction defining `OutboundMessage`, `Attachment`, `SendResponse`, and `InboundPayload` types.
- **Outbound**: `SendEmail(ctx, apiToken, msg)` relays messages through Postmark's transactional API. Used by `internal/smtp/smtp.go` after SMTP `DATA` ingestion.
- **Inbound**: Postmark inbound webhooks are received at `POST /webhooks/inbound`, parsed by `internal/postmark/postmark.go` (`ParseInbound`), and enqueued as background jobs.
- **Domain tokens**: Each `models.Domain` stores a `PostmarkToken` and `PostmarkStream` for per-domain API credentials.

### Webhook Handling
- **Handler**: `internal/webhook/webhook.go` — `Handler` struct receives and verifies Postmark webhooks.
- **Routes** (mounted via chi in `cmd/server/main.go`):
  - `POST /webhooks/inbound` — `handleInbound()` parses inbound email and enqueues `inbound` job.
  - `POST /webhooks/bounce` — `handleBounce()` enqueues `bounce` job.
  - `POST /webhooks/delivery` — `handleDelivery()` enqueues `delivery` job.
  - `POST /webhooks/spam` — `handleSpam()` enqueues `spam` job.
- **Verification**: `verify(r)` checks `X-Postmark-Signature` HMAC against `POSTNEST_POSTMARK_WEBHOOK_SECRET`.
- **Queueing**: Webhooks enqueue jobs via `internal/redis/redis.go` (`LPush`) for async processing by `cmd/worker/main.go`.

## Database Integrations

### PostgreSQL (pgx)
- **Driver**: `github.com/jackc/pgx/v5` (v5.9.2) with `pgxpool` for connection pooling.
- **Connection wrapper**: `internal/db/db.go` — `Pool` struct wraps `*pgxpool.Pool`; `New(dsn, maxConns)` creates the pool.
- **Primary consumers**:
  - `internal/mailstore/pgstore.go` — Implements `mailstore.Store` with 400+ lines of SQL for messages, labels, threads, attachments, flags, and search.
  - `internal/contacts/contacts.go` — `PGStore` implements `contacts.Store` with CRUD for address book entries.
  - `internal/auth/auth.go` — Password hashes, session tokens, API keys, domain membership, and user creation all query PostgreSQL directly via `pgxpool`.
  - `internal/search/search.go` — Async indexer updates `tsvector` search vectors in the `messages` table.
  - `internal/reputation/reputation.go` — Queries domain-level whitelist/blacklist/greylist rules.
- **Migration tooling**: `github.com/golang-migrate/migrate/v4` (v4.19.1) with `lib/pq` driver.
  - `internal/migrate/migrate.go` — Embeds `migrations/*.sql` via `//go:embed`; exposes `Up`, `Down`, `Version`, `Force`.
  - `cmd/migrate/main.go` — CLI entry point.
  - Migration files: `internal/migrate/migrations/000001_init.up.sql`, `000002_fts.up.sql`, `000003_seed_labels.up.sql`.

## Cache and Queue Integrations

### Redis (go-redis)
- **Library**: `github.com/redis/go-redis/v9` (v9.19.0).
- **Wrapper**: `internal/redis/redis.go` — `Client` embeds `redis.UniversalClient` and exposes app-specific helpers:
  - `Publish(ctx, channel, message)` — Pub/sub for real-time notifications.
  - `Enqueue(ctx, queue, payload)` — `LPush` for job queueing.
  - `Dequeue(ctx, queue, timeout)` — `BRPop` for blocking job consumption.
- **Worker pool**: `internal/workers/workers.go` — `Pool` consumes jobs via Redis `Dequeue`, dispatches to registered `Processor` implementations.
- **Job processors**:
  - `internal/workers/inbound.go` — Processes inbound email payloads.
  - `internal/workers/bounce.go` — Updates delivery status on bounces.
  - `internal/workers/delivery.go` — Records successful delivery events.

## Authentication Integrations

### Argon2id Password Hashing
- **Library**: `golang.org/x/crypto` (v0.51.0), specifically `argon2` and `bcrypt` packages.
- **Implementation**: `internal/auth/auth.go`.
  - `hashPassword(password)` — Generates Argon2id hash with configurable time, memory, and threads.
  - `verifyPassword(password, encodedHash)` — Constant-time verification using `crypto/subtle`.
- **Configuration**: TOML/env vars `POSTNEST_SECURITY_ARGON2ID_TIME`, `POSTNEST_SECURITY_ARGON2ID_MEMORY`, `POSTNEST_SECURITY_ARGON2ID_THREADS`.

### Session and API Key Management
- **Implementation**: `internal/auth/auth.go`.
  - `CreateSession()` — Generates a random token, stores SHA-256 hash in PostgreSQL.
  - `ValidateSession()` — Validates bearer token or cookie against hash.
  - `ValidateAPIKey()` — Validates API key (distinct from session tokens).
  - `Logout()` — Deletes session by token hash.

### ACME / Let's Encrypt TLS Certificates
- **Library**: `github.com/go-acme/lego/v4` (v4.35.2).
- **Implementation**: `internal/certmanager/manager.go` — Full certificate lifecycle manager.
  - `NewManager()` — Loads or generates ACME account key, sets up lego client.
  - `Start()` — Obtains certificate on first run, then starts background renewal loop.
  - `GetCertificate()` — Implements `tls.Config.GetCertificate` callback.
  - `renewalLoop()` — Polls `needsRenewal()` and re-obtains via DNS-01 before expiry.
- **DNS providers**: Cloudflare supported (`DNSProvider: "cloudflare"` in default config).
- **Storage**: Account keys and certificates stored on disk under `/var/lib/postnest/certs` (configurable via `POSTNEST_ACME_CERT_DIR`).

## Protocol Libraries

### IMAP
- **Library**: `github.com/emersion/go-imap` (v1.2.1).
- **Server**: `internal/imap/imap.go` — Wraps `imap.Server` with custom backend.
- **Backend**: `internal/imap/backend.go` — Custom `backend.Backend`, `backend.User`, and `backend.Mailbox` implementations.
  - Labels map to IMAP mailboxes.
  - `ListMessages`, `SearchMessages`, `CreateMessage`, `UpdateMessagesFlags`, and `CopyMessages` implemented.
  - UIDs derived deterministically from message UUIDs (`messageUID()`).

### SMTP
- **Library**: `github.com/emersion/go-smtp` (v0.24.0).
- **Server**: `internal/smtp/smtp.go` — Wraps `smtp.Server` with STARTTLS and `AUTH PLAIN`.
- **Backend**: `smtpBackend` and `smtpSession` implement `smtp.Backend` and `smtp.Session`.
  - `Mail()`, `Rcpt()`, `Data()` ingest inbound messages.
  - Outbound messages are forwarded to Postmark via `internal/postmark/postmark.go`.

### DAV (WebDAV / CardDAV / CalDAV)
- **Libraries**:
  - `github.com/emersion/go-webdav` (v0.7.0) — WebDAV framework.
  - `github.com/emersion/go-vcard` (v0.0.0-20241024213814-c9703dde27ff) — vCard parsing/generation.
  - `github.com/emersion/go-ical` (v0.0.0-20250609112844-439c63cef608) — iCalendar parsing.
- **Handler**: `internal/dav/dav.go` — Chi-mounted DAV handler.
  - `carddavBackend` — Full CardDAV implementation backed by `contacts.Store` (`internal/contacts/contacts.go`).
  - `caldavBackend` — Stubbed; all methods return `fmt.Errorf("not implemented")`.

## Search Integration

- **Engine**: PostgreSQL full-text search (`tsvector` / `tsquery`).
- **Implementation**: `internal/search/search.go` — `Indexer` queues messages for async indexing and batches updates.
- **Store integration**: `internal/mailstore/pgstore.go` — `Search()` executes `to_tsvector` queries; `UpdateSearchVector()` refreshes vectors.
- **Migration**: `internal/migrate/migrations/000002_fts.up.sql` — Adds search vector columns and GIN indexes.

## Reputation and Anti-Spam

- **Engine**: `internal/reputation/reputation.go` — `Engine` struct evaluates inbound messages.
- **Rules**: Domain-level whitelist, blacklist, and greylist stored in PostgreSQL.
- **Decisions**: `Allow`, `Deny`, `Greylist`.
- **Updates**: `UpdateReputation()` records events (e.g., bounces, spam reports) to adjust contact reputation.

## UUID and Crypto Utilities

- **UUID**: `github.com/google/uuid` (v1.6.0). Used in models, sessions, messages, labels, threads, contacts, and attachments.
- **Crypto**: `golang.org/x/crypto` (v0.51.0) — Argon2id, plus `crypto/subtle` for constant-time comparison in `internal/auth/auth.go`.
- **Backoff**: `github.com/cenkalti/backoff/v5` (v5.0.3) — Indirect dependency, used by lego for retry logic.
- **DNS**: `github.com/miekg/dns` (v1.1.72) — Indirect dependency via lego for ACME DNS-01 challenges.

## Integration Map by Package

| Package | Integration | File |
|---|---|---|
| `internal/postmark` | Postmark API | `internal/postmark/postmark.go` |
| `internal/webhook` | Postmark webhooks | `internal/webhook/webhook.go` |
| `internal/db` | PostgreSQL (pgx) | `internal/db/db.go` |
| `internal/migrate` | golang-migrate | `internal/migrate/migrate.go` |
| `internal/redis` | Redis (go-redis) | `internal/redis/redis.go` |
| `internal/workers` | Redis job queue | `internal/workers/workers.go` |
| `internal/auth` | Argon2id, sessions | `internal/auth/auth.go` |
| `internal/certmanager` | ACME / lego | `internal/certmanager/manager.go` |
| `internal/imap` | go-imap | `internal/imap/imap.go`, `internal/imap/backend.go` |
| `internal/smtp` | go-smtp | `internal/smtp/smtp.go` |
| `internal/dav` | go-webdav, go-vcard, go-ical | `internal/dav/dav.go` |
| `internal/search` | PostgreSQL tsvector | `internal/search/search.go` |
| `internal/reputation` | PostgreSQL rules | `internal/reputation/reputation.go` |
