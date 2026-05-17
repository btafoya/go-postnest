# External Integrations — PostNest

## Postmark (Email Transport)

Postmark is the sole email transport provider. PostNest does not directly deliver to MX records; it delegates all outbound delivery to Postmark and receives inbound mail exclusively via Postmark webhooks.

### Outbound Send
- **Library:** `github.com/mrz1836/postmark`
- **Package:** `internal/postmark`
- **Flow:** SMTP submission or webmail REST → persist Sent copy → enqueue `send` job → worker calls Postmark Send API
- **Authentication:** Per-domain API token (passed at send time, not globally configured)
- **Endpoint used:** Postmark REST API (`POST /email`)

### Inbound Webhook
- **Source:** Postmark Inbound Processing
- **Handler:** `internal/webhook` — `/webhooks/postmark/inbound`
- **Security:** HMAC-SHA256 signature verification using `POSTNEST_POSTMARK_WEBHOOK_SECRET`
- **Flow:** Postmark → webhook handler → deduplication check → enqueue `inbound` job → worker parses MIME, stores message/attachments, updates threads

### Event Webhooks
All Postmark event webhooks are registered on the same router group under `/webhooks/postmark/`:

| Event | Route | Job Type | Processor |
|-------|-------|----------|-----------|
| Bounce | `/webhooks/postmark/bounce` | `bounce` | `internal/workers.BounceProcessor` |
| Delivery | `/webhooks/postmark/delivery` | `delivery` | `internal/workers.DeliveryProcessor` |
| Spam Complaint | `/webhooks/postmark/spam` | `spam` | `internal/workers.SpamProcessor` |

All webhook handlers verify the Postmark signature, deduplicate by `MessageID`, and enqueue Redis jobs for asynchronous worker processing.

## PostgreSQL

- **Role:** Primary persistent datastore for all application state
- **Driver:** `github.com/jackc/pgx/v5` (connection pool)
- **Schema management:** `golang-migrate/migrate/v4` with embedded SQL files
- **Version required:** PostgreSQL 16+
- **Extensions:**
  - `pgcrypto` — used for UUID generation in schema
  - `pg_trgm` — trigram similarity for search
- **Tables:** domains, users, domain_members, auth_sessions, messages, threads, labels, message_labels, attachments, message_flags, imap_uids, contacts, contact_reputation, whitelist, greylist, blacklist, delivery_logs, webhook_events, bounce_events

### Connection Configuration
- **DSN:** `POSTNEST_DATABASE_DSN` (required)
- **Read replica:** `POSTNEST_DATABASE_READ_DSN` (optional; not currently wired to read-only queries)
- **Max connections:** `POSTNEST_DATABASE_MAX_CONNS` (default 25)

## Redis

- **Role:** Background job queue, delayed job scheduling, dead-letter queue, IMAP IDLE pub/sub
- **Library:** `github.com/redis/go-redis/v9`
- **Version required:** Redis 7+
- **Connection:** `POSTNEST_REDIS_URL` (default `redis://localhost:6379/0`)

### Queue Patterns
- **Main job queue:** Redis list (`queue:jobs`), consumed via `BRPop` with timeout
- **Delayed queue:** Redis sorted set (`queue:delayed`) scored by Unix timestamp; promoted by `PromoteReadyDelayed`
- **Dead-letter queue:** Redis list (`queue:dead`) for jobs exceeding max retries
- **IMAP IDLE:** Redis pub/sub channel `imap_idle:<user_id>` for mailbox change notifications

## ACME / Let's Encrypt (Optional TLS)

- **Library:** `github.com/go-acme/lego/v4`
- **Package:** `internal/certmanager`
- **Purpose:** Automatic TLS certificate provisioning and renewal
- **Trigger:** Enabled when `POSTNEST_ACME_ENABLED=true`

### Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `POSTNEST_ACME_ENABLED` | `false` | Enable automatic TLS |
| `POSTNEST_ACME_EMAIL` | — | ACME account email |
| `POSTNEST_ACME_DOMAIN` | — | Domain to certify |
| `POSTNEST_ACME_DIRECTORY` | `staging` | `staging` or `production` |
| `POSTNEST_ACME_CERT_DIR` | `/var/lib/postnest/certs` | Local cert cache |
| `POSTNEST_ACME_DNS_PROVIDER` | `cloudflare` | DNS-01 provider (Cloudflare only) |
| `POSTNEST_ACME_RENEW_INTERVAL` | `24h` | Background renewal check frequency |
| `POSTNEST_ACME_RENEW_BEFORE` | `720h` | Renew when within this duration of expiry |

### Behavior
- On startup: loads existing certificate or obtains new one via ACME
- Background goroutine: periodic `needsRenewal()` check + `renew()`
- TLS config injected into IMAP and SMTP listeners via `tls.Config.GetCertificate`

## Docker Compose (Local Deployment)

```yaml
services:
  postgres:   postgres:16-alpine
  redis:      redis:7-alpine
  server:     build: Dockerfile.server
  worker:     build: Dockerfile.worker
  migrate:    build: Dockerfile.migrate
```

- **Networking:** Services communicate over Docker bridge network
- **Volumes:** `postgres_data`, `redis_data`
- **Healthchecks:** `pg_isready` (postgres), `redis-cli ping` (redis)
- **Dependencies:** server/worker/migrate wait for postgres and redis to report healthy

## Systemd (Native Linux Deployment)

- **Script:** `scripts/install-systemd.sh`
- **Units generated:** `postnest-server.service`, `postnest-worker.service`
- **User:** `postnest` (dedicated system user)
- **Config path:** `/etc/postnest/postnest.conf`
- **Binaries:** `/usr/local/bin/postnest-server`, `/usr/local/bin/postnest-worker`
- **Prerequisites:** PostgreSQL and Redis must be installed externally

## NixOS Module (Declarative Deployment)

- **Entry:** `flake.nix` exposes `nixosModules.postnest`
- **Module:** `nix/module.nix`
- **Options:** `services.postnest.enable`, database password file, config path, user/group
- **Systemd units:** auto-generated from NixOS option declarations

## Authentication Providers

PostNest uses **local authentication only**. No external OAuth, SAML, or LDAP integrations exist.

### Session & API Key Auth
- **Password hashing:** Argon2id (`golang.org/x/crypto/argon2`)
- **Session tokens:** Random 32-byte base64 tokens, SHA-256 hashed in `auth_sessions` table
- **API keys:** Same token format as sessions, stored with `is_api_key=true`
- **Cookie transport:** Secure, HttpOnly, SameSite=Lax session cookies
- **Header transport:** Bearer token via `Authorization: Bearer <token>`

## Reputation & Filtering

Reputation data is entirely self-managed in PostgreSQL. No external RBL (Real-time Blackhole List), Spamhaus, or third-party reputation APIs are integrated.

- **Whitelist:** Per-domain email, domain, or IP allow-list
- **Blacklist:** Per-domain email, domain, or IP block-list
- **Greylist:** Triplets `(domain_id, sender_email, sender_ip, recipient_email)` with `passed_at` tracking
- **Contact reputation:** Per-contact score updated on bounce, spam, delivery, and inbound events

## Webhook Security

All Postmark webhook endpoints enforce:
1. **HMAC-SHA256 signature verification** against `POSTNEST_POSTMARK_WEBHOOK_SECRET`
2. **Deduplication** by `MessageID` stored in Redis (5-minute TTL)
3. **Envelope validation** before enqueueing to worker pool

## Notable Absences

The following integrations are **not present** in the current codebase:
- External identity providers (OAuth 2.0, OIDC, SAML, LDAP)
- External spam/RBL services (Spamhaus, DNSBL lookups)
- Object storage (S3, GCS, MinIO) — attachments stored inline in PostgreSQL
- Message queues (RabbitMQ, NATS, SQS, Kafka) — Redis is the sole queue
- Monitoring/telemetry (Prometheus, OpenTelemetry, Sentry, Datadog)
- SMTP MX delivery — all outbound relayed through Postmark
- CalDAV is a stub; no external calendar provider sync
