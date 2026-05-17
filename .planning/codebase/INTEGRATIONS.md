# Integrations

## Overview

PostNest integrates with external email transport, certificate authorities, databases, and pub-sub systems. All outbound network calls are explicit, typed, and wrapped in thin internal packages. There is no OAuth, SAML, LDAP, or third-party identity provider integration; authentication is entirely local.

## External APIs

### Postmark (`internal/postmark/postmark.go`)

Postmark is the primary email transport layer for both outbound sending and inbound receiving.

**Outbound send**
- Thin wrapper around `github.com/mrz1836/postmark`.
- `SendEmail` constructs a `lib.Email`, attaches base64-encoded attachments, and returns a `SendResponse` with `MessageID`, `ErrorCode`, and `Message`.
- Timeout: 30 seconds (`http.Client{Timeout: 30 * time.Second}`).
- The domain owner supplies a per-domain `PostmarkToken` (stored in the `domains` table) and a `MessageStream`.

```go
// internal/postmark/postmark.go
func (c *Client) SendEmail(ctx context.Context, apiToken string, msg *OutboundMessage) (*SendResponse, error) {
    client := lib.NewClient(apiToken, "")
    client.HTTPClient = &http.Client{Timeout: 30 * time.Second}
    // ...
}
```

**Inbound parsing**
- `ParseInbound` converts a raw JSON map into a typed `InboundPayload` (From, To, Subject, TextBody, HTMLBody, Attachments, Headers, etc.).
- Used by the worker pool (`internal/workers/inbound.go`) to process inbound webhooks.

### ACME / Let's Encrypt (`internal/certmanager/manager.go`)

Optional automatic TLS via `github.com/go-acme/lego/v4`.

- **DNS-01 challenge** using the Cloudflare provider (`lego/v4/providers/dns/cloudflare`).
- **Account key** generated or loaded from `cert_dir`.
- **Renewal loop** runs on a configurable interval (default 24h) and renews before expiry (default 720h).
- **Staging vs production** controlled by `directory` config (`staging` or `production`).
- The manager exposes `GetCertificate` compatible with `tls.Config.GetCertificate`.

```go
// internal/certmanager/manager.go
type Config struct {
    Email         string
    Domain        string
    Directory     string // "staging" or "production"
    CertDir       string
    DNSProvider   string // e.g. "cloudflare"
    RenewInterval time.Duration
    RenewBefore   time.Duration
}
```

## Databases & Caching

### PostgreSQL (`internal/db/db.go`, `internal/migrate/migrations/000001_init.up.sql`)

- **Driver**: `pgx/v5` (`github.com/jackc/pgx/v5/pgxpool`).
- **DSN**: supplied via `POSTNEST_DATABASE_DSN` (or legacy `POSTGRES_DSN`).
- **Max connections**: default 25 (`MaxDBConns`).
- **Schema**: 20+ tables including `domains`, `users`, `messages`, `threads`, `labels`, `attachments`, `delivery_logs`, `contacts`, `whitelist`, `blacklist`, `greylist`, `bounce_events`, `webhook_events`.
- **Full-text search**: PostgreSQL native `tsvector`/`GIN` on `messages.search_vector`.
- **Migrations**: embedded `.sql` files via `golang-migrate/migrate/v4`.

```go
// internal/db/db.go
func New(dsn string, maxConns int) (*Pool, error) {
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil { return nil, err }
    cfg.MaxConns = int32(maxConns)
    pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
    // ...
}
```

### Redis (`internal/redis/redis.go`)

- **Library**: `go-redis/v9` (`github.com/redis/go-redis/v9`).
- **URL parsing**: `redis.ParseURL(...)` supports `redis://host:port/db`.
- **Queues**:
  - `queue:jobs` — main job list (LPUSH / BRPOP).
  - `queue:jobs:delayed` — sorted set for retry backoff (ZADD / ZREVRANGEBYSCORE).
  - `queue:jobs:dead` — dead-letter list.
- **Pub/Sub**: channels `mailbox:updates`, `message:new`, `delivery:events` used by SSE hub.
- **Deduplication**: `SETNX` with TTL in webhook handler (`internal/webhook/webhook.go`).

```go
// internal/redis/redis.go
func (c *Client) Dequeue(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
    res, err := c.UniversalClient.BRPop(ctx, timeout, queue).Result()
    if err == redis.Nil { return nil, nil }
    // ...
}
```

## Authentication & Authorization

PostNest does **not** integrate with external identity providers. All auth is local.

- **Password hashing**: Argon2id (`golang.org/x/crypto/argon2`) with configurable time, memory, and threads (`internal/auth/auth.go`).
- **Sessions**: 16-byte random tokens, SHA-256 hashed in `auth_sessions` table, returned as `HttpOnly` cookies or Bearer tokens (`internal/api/middleware.go`).
- **API keys**: stored as hashed tokens in `auth_sessions`; validated as fallback when session cookie is absent.
- **Domain roles**: `domain_members` table links users to domains with `admin` or `member` roles. Super-admins bypass domain checks.
- **Middleware**:
  - `RequireSession` — validates session cookie or Bearer token.
  - `RequireDomainAdmin` — checks `X-Domain-ID` header or query param against `domain_members`.

```go
// internal/api/middleware.go
func RequireSession(svc *auth.Service) func(http.Handler) http.Handler { ... }
func RequireDomainAdmin(svc *auth.Service) func(http.Handler) http.Handler { ... }
```

## Webhooks

### Postmark Webhooks (`internal/webhook/webhook.go`)

Four webhook endpoints are registered under `/webhooks/postmark/`:

| Endpoint | Job Type | Processor |
|---|---|---|
| `/webhooks/postmark/inbound` | `inbound` | `InboundProcessor` |
| `/webhooks/postmark/bounce` | `bounce` | `BounceProcessor` |
| `/webhooks/postmark/delivery` | `delivery` | `DeliveryProcessor` |
| `/webhooks/postmark/spam` | `spam` | `SpamProcessor` |

**Signature verification**
- Primary: HMAC-SHA256 of the request body against `POSTNEST_POSTMARK_WEBHOOK_SECRET`, compared to `X-Postmark-Signature` header.
- Fallback: compare `X-Postmark-Server-Token` header to the same secret.

```go
// internal/webhook/webhook.go
func (h *Handler) verifySignature(body []byte, r *http.Request) bool {
    if sig := r.Header.Get("X-Postmark-Signature"); sig != "" && h.secret != "" {
        mac := hmac.New(sha256.New, []byte(h.secret))
        mac.Write(body)
        expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
        return hmac.Equal([]byte(expected), []byte(sig))
    }
    token := r.Header.Get("X-Postmark-Server-Token")
    return token != "" && token == h.secret
}
```

**Deduplication**
- Uses Redis `SETNX` with a 5-minute TTL keyed by `webhook:<MessageID>`.
- If dedup fails, the handler returns `200 OK` to prevent Postmark retries.

**Enqueueing**
- Each webhook unmarshals the JSON payload, deduplicates, then pushes a `workers.Job` JSON blob onto the Redis `queue:jobs` list.
- Job struct includes `ID`, `Type`, `Payload`, `MaxAttempts: 3`, `CreatedAt`, `ScheduledAt`.

## Third-Party Services

| Service | Usage | Config / Key | File |
|---|---|---|---|
| **Postmark** | Outbound email delivery, inbound webhooks, bounce/delivery/spam tracking | Per-domain `PostmarkToken`; global `PostmarkWebhookSecret` | `internal/postmark/postmark.go`, `internal/webhook/webhook.go` |
| **Let's Encrypt** | Automatic TLS certificates (optional) | `ACMEEmail`, `ACMEDomain`, `ACMEDNSProvider` | `internal/certmanager/manager.go` |
| **Cloudflare DNS** | DNS-01 challenge for ACME (optional) | `ACMEDNSProvider: "cloudflare"` | `internal/certmanager/manager.go` |

## Protocol Integrations

### IMAP4rev1 (`internal/imap/imap.go`, `internal/imap/backend.go`)

- **Library**: `emersion/go-imap` + `emersion/go-imap/backend`.
- **Auth**: PLAIN and LOGIN SASL mechanisms; credentials validated against `auth.Service`.
- **Commands supported**: LOGIN, LIST, STATUS, FETCH, SEARCH, APPEND, EXPUNGE, COPY, flag updates (\Seen, \Flagged, \Answered, \Draft, \Deleted).
- **UID mapping**: deterministic 32-bit UID derived from the first 4 bytes of the message UUID.
- **IDLE**: Redis pub/sub infrastructure is wired (`redis` passed to `NewServer`) but IDLE broadcast is not yet implemented.

### SMTP (`internal/smtp/smtp.go`)

- **Library**: `emersion/go-smtp` + `emersion/go-sasl`.
- **Auth**: PLAIN and LOGIN mechanisms.
- **Relay**: incoming messages are parsed with `emersion/go-message`, persisted as "Sent", then forwarded via Postmark `SendEmail`.
- **TLS**: optional STARTTLS / immediate TLS based on `tls.Config`.

### CardDAV / CalDAV / WebDAV (`internal/dav/dav.go`)

- **Library**: `emersion/go-webdav/carddav` and `emersion/go-webdav/caldav`.
- **CardDAV**: fully functional — list address books, get/put/delete vCard 4.0 contacts (`go-vcard`).
- **CalDAV**: stub — all operations return `fmt.Errorf("not implemented")`.
- **Auth**: Basic Auth only (no Bearer token support yet).

## Worker Queue Integration

### Pool (`internal/workers/workers.go`)

- Redis-backed worker pool with configurable concurrency (default 10) and poll interval (default 5s).
- Supports delayed retries with linear backoff (`attempts * 5s`).
- Dead-letter queue (`queue:jobs:dead`) after 3 failed attempts.
- Graceful shutdown via `context.CancelFunc` + `sync.WaitGroup`.

### Registered Processors

| Processor | File | Responsibility |
|---|---|---|
| `InboundProcessor` | `internal/workers/inbound.go` | Parses Postmark inbound payload, resolves domain/user, greylist/blacklist check, creates message + attachments, assigns INBOX label, updates search vector. |
| `SendProcessor` | `internal/workers/send.go` | Loads draft from `mailstore`, sends via Postmark, updates message to `IsDraft=false`, creates `DeliveryLog`. |
| `BounceProcessor` | `internal/workers/bounce.go` | Updates `delivery_logs` and inserts `bounce_events` by `postmark_message_id`. |
| `DeliveryProcessor` | `internal/workers/delivery.go` | Updates `delivery_logs.status = 'delivered'` by `postmark_message_id`. |
| `SpamProcessor` | `internal/workers/spam.go` | Logs spam complaint; reputation update is stubbed because domain ID is not enriched in the payload. |

### Reputation Engine (`internal/reputation/reputation.go`)

- **Whitelist / blacklist / greylist** evaluated at inbound time using PostgreSQL tables.
- **Greylist triplet** (`domain_id`, `sender_email`, `sender_ip`, `recipient_email`) with unique constraint.
- **Contact reputation** scoring via `contact_reputation` table (`sent_count`, `bounce_count`, `complaint_count`).

## Frontend Integrations

### REST API Client (`web/src/api.js`)

- **axios** instance configured with `baseURL: '/api/v1'` and `withCredentials: true`.
- Global 401 interceptor redirects to `/login`.
- Covers: labels, messages, threads, drafts, search, contacts, calendar events, admin domains.

### Server-Sent Events (`web/src/sse.js`, `internal/webui/sse.go`)

- Frontend `SSEClient` connects to `/events` with auto-reconnect and exponential backoff (capped at 30s).
- Backend `SSEHub` (Gin) subscribes to Redis channels `mailbox:updates`, `message:new`, `delivery:events` and broadcasts JSON events to all connected clients.
- Heartbeat every 30 seconds.

```js
// web/src/sse.js
this.eventSource = new EventSource('/events')
this.eventSource.addEventListener('connected', (e) => { ... })
this.eventSource.onmessage = (e) => {
    const event = JSON.parse(e.data)
    this.emit(event.type, event.payload)
}
```

## Summary Table

| Integration | Type | Direction | Status |
|---|---|---|---|
| Postmark (outbound) | HTTP API | Outbound | ✅ |
| Postmark (inbound webhook) | HTTP Webhook | Inbound | ✅ |
| Postmark (bounce/delivery/spam) | HTTP Webhook | Inbound | ✅ |
| Let's Encrypt (ACME) | ACME / DNS-01 | Outbound | ✅ optional |
| Cloudflare DNS | DNS API | Outbound | ✅ optional (via lego) |
| PostgreSQL | SQL / TCP | Bidirectional | ✅ required |
| Redis | TCP / RESP | Bidirectional | ✅ required |
| IMAP4rev1 | TCP / IMAP | Bidirectional | ✅ |
| SMTP (submission) | TCP / SMTP | Bidirectional | ✅ |
| CardDAV | HTTP / WebDAV | Bidirectional | ✅ |
| CalDAV | HTTP / WebDAV | Bidirectional | 🚧 stub |
| SSE (real-time UI) | HTTP / SSE | Server→Client | ✅ |
