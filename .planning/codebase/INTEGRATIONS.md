# PostNest External Integrations

## Email Transport: Postmark

Postmark is the sole email transport provider. All inbound and outbound email flows through Postmark's API and webhook infrastructure.

### Outbound Send API

| Detail | Value |
|---|---|
| Library | `github.com/mrz1836/postmark` |
| Wrapper | `internal/postmark/postmark.go` |
| Auth | Per-request API token (stored per-domain or per-message) |
| Features | HTML + text bodies, attachments, reply-to, tags, metadata |

**Flow:** SMTP client / webmail draft → `postmark.Client.SendEmail()` → Postmark REST API → recipient.

### Inbound Webhook

| Detail | Value |
|---|---|
| Endpoint | `POST /webhooks/postmark/inbound` |
| Handler | `internal/webhook/webhook.go` |
| Verification | HMAC-SHA256 signature against `POSTNEST_POSTMARK_WEBHOOK_SECRET` |
| Deduplication | SHA-256 hash of raw JSON body stored in `webhook_events` table |
| Enqueue | Raw payload pushed to Redis `queue:jobs` as `inbound` job type |
| Processor | `internal/workers/inbound.go` parses MIME, stores message + attachments, creates threads |

### Bounce Webhook

| Detail | Value |
|---|---|
| Endpoint | `POST /webhooks/postmark/bounce` |
| Processor | `internal/workers/bounce.go` |
| Action | Updates `delivery_logs` and `bounce_events` tables; triggers reputation update |

### Delivery Webhook

| Detail | Value |
|---|---|
| Endpoint | `POST /webhooks/postmark/delivery` |
| Processor | `internal/workers/delivery.go` |
| Action | Updates `delivery_logs` status to `delivered` |

### Spam Complaint Webhook

| Detail | Value |
|---|---|
| Endpoint | `POST /webhooks/postmark/spam` |
| Handler | `internal/webhook/webhook.go` (`handleSpam`) |
| Action | Updates reputation score and logs event |

## Database: PostgreSQL

| Detail | Value |
|---|---|
| Driver | `github.com/jackc/pgx/v5` / `pgxpool` |
| Min version | PostgreSQL 16+ |
| Connection | DSN from `POSTNEST_DATABASE_DSN` (or `POSTGRES_DSN` legacy) |
| Read replica | Optional `POSTNEST_DATABASE_READ_DSN` |

### Schema Overview

| Table | Purpose |
|---|---|
| `domains` | Multi-tenant email domains |
| `users` | Platform users with Argon2id password hashes |
| `domain_members` | Role-based domain membership (admin/member) |
| `auth_sessions` | Session tokens and API keys (SHA-256 hashed) |
| `messages` | Email messages with `tsvector` full-text search column |
| `threads` | Conversation groupings |
| `labels` | Gmail-style labels (system + custom) |
| `message_labels` | Many-to-many label assignment |
| `attachments` | File metadata (content stored inline or referenced) |
| `message_flags` | IMAP flag states (Seen, Answered, Flagged, Deleted, Draft, Recent) |
| `imap_uids` | IMAP UID mapping (defined, derived from message UUIDs) |
| `contacts` | Address book entries with vCard support |
| `contact_reputation` | Per-contact reputation scores |
| `whitelist` / `blacklist` / `greylist` | Reputation rule tables |
| `delivery_logs` | Outbound message delivery tracking |
| `webhook_events` | Raw webhook payload log with dedup hash |
| `bounce_events` | Structured bounce event records |

## Cache & Queue: Redis

| Detail | Value |
|---|---|
| Client | `github.com/redis/go-redis/v9` |
| Wrapper | `internal/redis/redis.go` |
| Min version | Redis 7+ |
| Connection | URL from `POSTNEST_REDIS_URL` (or `REDIS_URL` legacy) |

### Redis Data Structures

| Structure | Key Pattern | Purpose |
|---|---|---|
| List | `queue:jobs` | Main job queue (workers call `BRPop`) |
| Sorted Set | `queue:jobs:delayed` | Delayed retry jobs (score = unix timestamp) |
| List | `queue:jobs:dead` | Dead-letter queue for exhausted retries |
| Pub/Sub | `mailbox:<user_id>` | IMAP IDLE notifications |

## ACME / TLS: Let's Encrypt

| Detail | Value |
|---|---|
| Library | `github.com/go-acme/lego/v4` |
| Implementation | `internal/certmanager/manager.go` |
| Challenge | DNS-01 (configurable provider via lego) |
| Storage | Account key and certificate on local filesystem |
| Renewal | Background loop with 30-day threshold |

Configuration section `[acme]` in TOML: `email`, `accept_tos`, `provider`, `domain`, `storage_path`.

## Protocol Servers (Inbound Client Connections)

| Protocol | Port | Library | Status |
|---|---|---|---|
| HTTP REST | 8080 | `chi/v5` | ✅ Full |
| IMAP4rev1 | 143 / 993 | `go-imap` | ✅ LOGIN, LIST, STATUS, FETCH, SEARCH, APPEND, EXPUNGE, COPY, flag updates, IDLE |
| SMTP submission | 587 / 465 | `go-smtp` | ✅ AUTH PLAIN + LOGIN, DATA relay to Postmark, TLS |
| WebDAV / CardDAV | 8080 (mounted) | `go-webdav` | ✅ CardDAV list/get/put/delete |
| CalDAV | 8080 (mounted) | `go-webdav` / `go-ical` | 🚧 Stub (not implemented) |

### IMAP Integration Points
- IDLE uses Redis pub/sub (`mailbox:<user_id>`) to notify clients of new mail
- UIDs derived deterministically from message UUIDs
- MODSEQ tracked for `imap_uids` table

### SMTP Integration Points
- AUTH PLAIN and LOGIN mechanisms supported
- `DATA` handler parses MIME via `go-message`, persists to `mailstore`, then relays to Postmark
- Sent copy stored in user's mailbox immediately

## Identity & Access

| Mechanism | Implementation |
|---|---|
| Password auth | Argon2id (`golang.org/x/crypto/argon2`) |
| Session auth | Secure HTTP-only cookie or Bearer token header |
| Domain scoping | Every request carries an active domain ID (from `X-Domain-ID` header or session context) |
| Role check | `internal/auth` provides `IsDomainAdmin`, `IsDomainMember` |
| DAV auth | Basic Auth only (no Bearer token support yet) |

## Webhook Security

Postmark webhook signatures are verified in `internal/webhook/webhook.go` using HMAC-SHA256 of the raw JSON body against `POSTNEST_POSTMARK_WEBHOOK_SECRET`.

## Planned / Stub Integrations

| Integration | Status | Location |
|---|---|---|
| CalDAV calendar backend | Stub (all methods return "not implemented") | `internal/dav/dav.go` |
| STARTTLS on IMAP/SMTP | Not implemented | `cmd/server/main.go` TLS config present but STARTTLS missing |
| Admin REST handlers | Not implemented | Design documents reference; no handlers wired |
| Frontend UI | Not implemented | API-only; no SPA or server-rendered UI |
| Systemd service integration | Partial | `scripts/install-systemd.sh` exists; `kardianos/service` referenced but not confirmed wired |
