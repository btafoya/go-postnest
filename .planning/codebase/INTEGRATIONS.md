# External Integrations

**Analysis Date:** 2026-05-18

## APIs & External Services

**Email Transport:**
- Postmark — Outbound sending, inbound parsing, bounce/delivery/spam event handling
  - SDK/Client: `github.com/mrz1836/postmark` (`internal/postmark/postmark.go`)
  - Auth: Per-domain Postmark API token stored in `domains.postmark_token` (configured via admin API)
  - Webhook secret: `POSTNEST_POSTMARK_WEBHOOK_SECRET` env var / TOML config

**Certificate Automation (ACME):**
- Let's Encrypt — TLS certificate provisioning and renewal
  - Client: `github.com/go-acme/lego/v4` (`internal/certmanager/manager.go`)
  - DNS-01 challenge provider: Cloudflare (default; `cloudflare.NewDNSProvider()` reads standard Cloudflare env vars)
  - Configuration: `ACME_ENABLED`, `ACME_EMAIL`, `ACME_DOMAIN`, `ACME_DIRECTORY` (staging/production), `ACME_DNS_PROVIDER`

## Data Storage

**Databases:**
- PostgreSQL 16+
  - Connection: DSN via `POSTNEST_DATABASE_DSN` (or legacy `POSTGRES_DSN`)
  - Read replica DSN: `POSTNEST_DATABASE_READ_DSN` (optional)
  - Client: `github.com/jackc/pgx/v5/pgxpool` (`internal/db/db.go`)
  - Schema: 8 migration files in `internal/migrate/migrations/000001_init.up.sql` through `000008_admin.up.sql`
  - Full-text search: PostgreSQL `tsvector` + GIN indexes (`internal/migrate/migrations/000002_fts.up.sql`)

**Cache / Job Queue:**
- Redis 7+
  - Connection: `POSTNEST_REDIS_URL` (default `redis://localhost:6379/0`)
  - Client: `github.com/redis/go-redis/v9` (`internal/redis/redis.go`)
  - Uses: worker job queue (`queue:jobs`), delayed retry queue (sorted set), dead-letter queue, webhook deduplication (`webhook:<MessageID>`), pub/sub for SSE (`internal/webui`)

**File Storage:**
- Local filesystem only — TLS certificates (`/var/lib/postnest/certs`), ACME account keys, embedded SPA dist
- Attachments and raw messages stored in PostgreSQL (`messages.raw_message BYTEA`, attachments table)

**Caching:**
- Redis (see above) — No additional caching layer (e.g., no Memcached or CDN)

## Authentication & Identity

**Auth Provider:**
- Custom local authentication — No external OAuth, SAML, or SSO integration
  - Password hashing: Argon2id via `golang.org/x/crypto/argon2` (`internal/auth/`)
  - Session management: Server-side sessions with cookie + `X-CSRF-Token` double-submit (`web/src/api.js`, `internal/api/csrf_test.go`)
  - DAV authentication: HTTP Basic Auth only (`INTEGRATION.md` notes no Bearer support yet)
  - Roles: `admin`, `user` (domain-scoped via `domain_members`)

## Monitoring & Observability

**Error Tracking:**
- None detected — No Sentry, Rollbar, Bugsnag, or DataDog integration

**Logs:**
- Structured JSON logging via Go `slog` (`internal/logger/`)
- Standard stdout/stderr in container deployments

**Health Checks:**
- HTTP endpoint `/healthz` verifies PostgreSQL and Redis connectivity (`cmd/server`)

## CI/CD & Deployment

**Hosting:**
- Self-hosted / on-premise — No cloud platform-specific integrations (AWS, GCP, Azure)

**CI Pipeline:**
- None detected — No `.github/workflows`, GitLab CI, Jenkinsfile, or similar

**Local Orchestration:**
- Docker Compose (`docker-compose.yml`) spins up `postgres`, `redis`, `server`, `webui`, `worker`, `migrate`

**Packaging:**
- Nix flake with NixOS module (`flake.nix`, `nix/module.nix`)
- Multi-stage Dockerfiles for each binary (`Dockerfile.server`, `Dockerfile.webui`, `Dockerfile.worker`, `Dockerfile.migrate`, `Dockerfile.admin`)

## Environment Configuration

**Required env vars:**
- `POSTNEST_SECURITY_SESSION_KEY` — Session encryption/signing key
- `POSTNEST_DATABASE_DSN` — PostgreSQL connection string

**Critical optional vars:**
- `POSTNEST_POSTMARK_WEBHOOK_SECRET` — HMAC verification for Postmark webhooks
- `POSTNEST_REDIS_URL` — Redis connection (defaults to localhost)
- `POSTNEST_TLS_CERT_PATH` / `POSTNEST_TLS_KEY_PATH` — Manual TLS certificates
- `POSTNEST_SECURITY_ALLOW_INSECURE_AUTH` — Allow plaintext IMAP/SMTP auth (default `false`)

**ACME / Auto-TLS vars:**
- `POSTNEST_ACME_ENABLED`, `POSTNEST_ACME_EMAIL`, `POSTNEST_ACME_DOMAIN`, `POSTNEST_ACME_DIRECTORY`
- Cloudflare DNS provider credentials (read by lego; standard `CLOUDFLARE_*` env vars)

**Secrets location:**
- Environment variables or TOML config file (`/etc/postnest/postnest.conf`)
- `.env` file exists at repo root but is **not** consumed by the application runtime; used by Docker Compose and local shell scripts only

## Webhooks & Callbacks

**Incoming:**
- `POST /webhooks/postmark/inbound` — Inbound email payload from Postmark (`internal/webhook/webhook.go`)
- `POST /webhooks/postmark/bounce` — Bounce event notification
- `POST /webhooks/postmark/delivery` — Delivery confirmation event
- `POST /webhooks/postmark/spam` — Spam complaint event
- Signature verification: HMAC-SHA256 via `X-Postmark-Signature` header (falls back to `X-Postmark-Server-Token`)
- Deduplication: 5-minute Redis `SetNX` window per `MessageID`
- All webhooks enqueue a JSON job to Redis `queue:jobs` for background worker processing

**Outgoing:**
- None — The system does not expose webhook callback URLs for external systems to call outbound

---

*Integration audit: 2026-05-18*
