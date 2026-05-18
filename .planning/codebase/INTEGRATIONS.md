# External Integrations

**Analysis Date:** 2026-05-18

## APIs & External Services

**Email Delivery (Outbound & Inbound):**
- Postmark - Transactional email delivery and inbound processing
  - SDK/Client: `github.com/mrz1836/postmark` (`internal/postmark/postmark.go`)
  - Outbound API: `api.postmarkapp.com` (used in `internal/postmark/postmark.go:SendEmail`)
  - Auth: Per-domain `postmark_token` stored in `domains` table; API token passed per send call
  - Webhook secret: `POSTNEST_POSTMARK_WEBHOOK_SECRET` env var (`internal/webhook/webhook.go`)

**TLS Certificate Automation:**
- Let's Encrypt (via ACME) - Automatic TLS certificate provisioning
  - Client: `github.com/go-acme/lego/v4` (`cmd/server/main.go`)
  - Configuration: `POSTNEST_ACME_ENABLED`, `POSTNEST_ACME_EMAIL`, `POSTNEST_ACME_DOMAIN`, `POSTNEST_ACME_DIRECTORY`, `POSTNEST_ACME_DNS_PROVIDER`
  - Used for IMAPS (993) and SMTPS (465) TLS termination

## Data Storage

**Primary Database:**
- PostgreSQL 16
  - Connection: `POSTNEST_DATABASE_DSN` / `POSTNEST_POSTGRES_DSN` env var
  - Client: `github.com/jackc/pgx/v5` (`internal/db/db.go`)
  - Migrations: `golang-migrate/migrate/v4` with embedded SQL files in `internal/migrate/migrations/`
  - Features used: Full-text search (`tsvector`/`tsquery`), `gen_random_uuid()` (pgcrypto), triggers

**Cache & Job Queue:**
- Redis 7
  - Connection: `POSTNEST_REDIS_URL` / `WEBUI_REDIS_URL` (default `redis://localhost:6379/0`)
  - Client: `github.com/redis/go-redis/v9` (`internal/redis/redis.go`)
  - Uses:
    - Background job queue (`queue:jobs`, `queue:jobs:delayed`, `queue:jobs:dead`) (`internal/workers/workers.go`)
    - Webhook deduplication (`webhook:` prefix keys with 5-minute TTL) (`internal/webhook/webhook.go`)
    - SSE pub/sub (`mailbox:updates`, `message:new`, `delivery:events` channels) (`internal/webui/sse.go`)
    - Rate limiting (via `api.NewRateLimiter`) (`cmd/server/main.go`)

**File Storage:**
- Local filesystem / embedded only
- No external object storage (S3, MinIO, etc.) detected
- Attachments appear to be handled as base64-encoded data or uploaded via multipart to the backend; storage target is the PostgreSQL database

**Search:**
- PostgreSQL native full-text search (`tsvector`)
  - Indexing: `internal/search/search.go` updates `search_vector` column directly with weighted tsvectors
  - No external search engine (Elasticsearch, Typesense, etc.)

**Caching:**
- Redis (as noted above)
- No additional caching layer (Memcached, etc.)

## Authentication & Identity

**Auth Provider:**
- Custom implementation (no external IdP)
  - Password hashing: Argon2id (`golang.org/x/crypto/argon2`) (`internal/auth/auth.go`)
  - Session management: Random 32-byte tokens stored as SHA-256 hashes in `auth_sessions` table, returned as HTTP-only cookies
  - CSRF protection: Cookie-based CSRF tokens validated on mutating requests (`internal/api/` middleware)
  - DAV auth: HTTP Basic Auth against the same password hash store (`internal/dav/dav.go`)

**API Key Support:**
- `api_key` type in `auth_sessions` table (`internal/auth/auth.go:ValidateAPIKey`)

**Roles:**
- `user`, `admin`, `readonly` domain roles (`domain_members` table)
- Super-admin flag on `users` table (`is_super_admin`)

## Monitoring & Observability

**Error Tracking:**
- None detected (no Sentry, Rollbar, etc.)

**Logs:**
- Structured JSON logging via Go `log/slog` (`internal/logger/logger.go`)
- Output: `os.Stdout`
- Level: `slog.LevelInfo` (default)
- Log entries include `request` events with method, path, status, latency, client IP (`internal/webui/router.go`)

**Health Checks:**
- `/healthz` - Checks PostgreSQL ping and Redis ping, returns `ok` or `degraded` (`cmd/server/main.go`)
- `/admin/api/v1/health` - Detailed component health with latencies for DB, Redis, IMAP, SMTP, worker queue depth, active users, messages today, total domains (`cmd/server/main.go`)

**Metrics:**
- No Prometheus, StatsD, or OpenTelemetry detected

## CI/CD & Deployment

**Hosting:**
- Self-hosted / on-premise (no cloud platform-specific code)
- Docker Compose for local/dev deployment
- Distroless container images for production (`gcr.io/distroless/static-debian12:nonroot`)

**CI Pipeline:**
- None detected (no `.github/workflows/`, `.gitlab-ci.yml`, etc.)

**Nix:**
- `flake.nix` defines `postnest-server`, `postnest-worker`, `postnest-migrate` packages
- `nixosModules.postnest` available for NixOS deployment (`nix/module.nix`)

## Environment Configuration

**Required env vars for runtime:**
- `POSTNEST_SECURITY_SESSION_KEY` - 32+ byte secret for session signing
- `POSTNEST_DATABASE_DSN` - PostgreSQL DSN
- `POSTNEST_REDIS_URL` - Redis URL
- `POSTNEST_POSTMARK_WEBHOOK_SECRET` - Required if receiving Postmark webhooks

**Optional but commonly used:**
- `POSTNEST_TLS_CERT_PATH` + `POSTNEST_TLS_KEY_PATH` - For static TLS
- `POSTNEST_ACME_*` - For auto TLS
- `POSTNEST_SECURITY_ALLOW_INSECURE_AUTH` - Allow plaintext IMAP/SMTP auth (development only)
- `POSTNEST_WORKER_CONCURRENCY` / `POSTNEST_WORKER_POLL_INTERVAL`

**Secrets location:**
- `.env` file present in repo root (not committed, in `.gitignore`)
- `.env.example` documents required variables
- CLI tools (`cmd/admin/main.go`) auto-load `.env` via `github.com/joho/godotenv`

## Webhooks & Callbacks

**Incoming:**
- Postmark inbound webhook - `POST /webhooks/postmark/inbound`
- Postmark bounce webhook - `POST /webhooks/postmark/bounce`
- Postmark delivery webhook - `POST /webhooks/postmark/delivery`
- Postmark spam webhook - `POST /webhooks/postmark/spam`
  - Handler: `internal/webhook/webhook.go`
  - Signature verification: HMAC-SHA256 (`X-Postmark-Signature`) or legacy token (`X-Postmark-Server-Token`)
  - Deduplication: Redis `SetNX` with 5-minute TTL on `webhook:<MessageID>`
  - All verified webhooks are enqueued as jobs in Redis `queue:jobs`

**Outgoing:**
- Postmark API (`api.postmarkapp.com`) for sending outbound email (`internal/postmark/postmark.go`)
- ACME/Let's Encrypt for certificate issuance (outbound to ACME directory URL)

## Network Ports

**Server (`cmd/server`):**
- 8080 - HTTP API and webhooks
- 143 - IMAP (plaintext or STARTTLS, depending on TLS config)
- 993 - IMAPS (if TLS enabled)
- 587 - SMTP submission (plaintext or STARTTLS)
- 465 - SMTPS (if TLS enabled)

**WebUI (`cmd/webui`):**
- 3000 - React SPA and API proxy (proxies to server:8080)

**Docker Compose:**
- Postgres exposed on host 5432
- WebUI exposed on host 2626 (maps to container 3000)

---

*Integration audit: 2026-05-18*
