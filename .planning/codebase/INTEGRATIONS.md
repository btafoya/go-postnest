# External Integrations

**Analysis Date:** 2025-07-28

## APIs & External Services

**Email Transport:**
- Postmark — Inbound and outbound email transport layer
  - SDK/Client: `github.com/mrz1836/postmark` v1.9.2
  - Auth: Per-domain API token (`domain.postmark_token`) for outbound sends; webhook secret (`POSTNEST_POSTMARK_WEBHOOK_SECRET`) for inbound verification
  - Endpoints used: Send Email API (outbound), Inbound Webhooks (inbound mail), Bounce Webhooks, Delivery Webhooks, Spam Complaint Webhooks

**Certificate Automation:**
- Let's Encrypt (ACME) — Automatic TLS certificate provisioning and renewal
  - SDK/Client: `github.com/go-acme/lego/v4` v4.35.2
  - Challenge: DNS-01 via Cloudflare provider (default)
  - Config: `POSTNEST_ACME_EMAIL`, `POSTNEST_ACME_DOMAIN`, `POSTNEST_ACME_DIRECTORY` (`staging` or `production`), `POSTNEST_ACME_CERT_DIR`
  - Cloudflare DNS provider reads standard `CLOUDFLARE_API_TOKEN` or `CLOUDFLARE_EMAIL`/`CLOUDFLARE_API_KEY` environment variables

## Data Storage

**Databases:**
- PostgreSQL 16+ — Primary datastore for mail, contacts, users, domains, delivery logs, reputation
  - Connection: `POSTNEST_DATABASE_DSN` env var (e.g., `postgres://user:pass@host:5432/postnest?sslmode=disable`)
  - Optional read replica: `POSTNEST_DATABASE_READ_DSN`
  - Client: `github.com/jackc/pgx/v5` v5.9.2 with connection pooling (`max_conns` default 25)
  - Migrations: `github.com/golang-migrate/migrate/v4` v4.19.1 (embedded in `cmd/migrate`)
  - Schema includes: users, domains, messages, threads, labels, contacts, attachments, delivery_logs, auth_sessions, whitelist/blacklist/greylist, FTS via PostgreSQL `tsvector`

**Caching / Job Queue:**
- Redis 7+ — Background job queue (`queue:jobs`, `queue:delayed`, `queue:dead`) and IMAP IDLE pub/sub
  - Connection: `POSTNEST_REDIS_URL` env var (default `redis://localhost:6379/0`)
  - Client: `github.com/redis/go-redis/v9` v9.19.0
  - Patterns: Redis lists for job enqueue/dequeue; sorted sets for delayed jobs; SetNX for webhook deduplication

**File Storage:**
- None external — Attachments stored as bytea in PostgreSQL (`attachments` table)

## Authentication & Identity

**Auth Provider:**
- Custom implementation (no external identity provider)
  - Password hashing: Argon2id via `golang.org/x/crypto`
  - Session tokens: Random 32-byte tokens, SHA-256 hashed in `auth_sessions` table, delivered via secure httpOnly cookies or Bearer tokens
  - API keys: Same mechanism as sessions but with `type='api_key'`
  - DAV (CardDAV/CalDAV): Basic Auth against `users` table

**OAuth Integrations:**
- None currently

## Monitoring & Observability

**Error Tracking:**
- None external — Structured JSON logging via Go `log/slog`

**Analytics:**
- None external

**Logs:**
- JSON `slog` output to stdout/stderr only

**Health Checks:**
- `/healthz` endpoint verifies PostgreSQL (`pgxpool.Ping`) and Redis (`PING`) connectivity; returns 503 if degraded

## CI/CD & Deployment

**Hosting:**
- Docker Compose — Local and production container orchestration (`docker-compose.yml`)
- systemd — Native Linux services (`scripts/install-systemd.sh` creates `postnest-server.service` and `postnest-worker.service`)
- NixOS — Declarative module (`nix/module.nix`) with `services.postnest.enable`

**CI Pipeline:**
- None configured — No GitHub Actions, GitLab CI, or other pipeline files present

**Container Registry:**
- Uses `gcr.io/distroless/static-debian12:nonroot` as runtime base
- Build stage: `golang:1.25-alpine`

## Environment Configuration

**Development:**
- Required env vars: `POSTNEST_DATABASE_DSN`, `POSTNEST_SECURITY_SESSION_KEY`, `POSTNEST_POSTMARK_WEBHOOK_SECRET`
- Optional: `POSTNEST_REDIS_URL`, `POSTNEST_CONFIG_PATH`
- Mock/stub services: PostgreSQL local instance, Redis local instance, Postmark test account

**Production:**
- TLS: Either static certs (`POSTNEST_TLS_CERT_PATH`/`POSTNEST_TLS_KEY_PATH`) or ACME auto-provisioned
- Secrets management: Env vars or TOML config file (e.g., `/etc/postnest/postnest.conf` with restricted permissions)
- No failover/redundancy explicitly configured in codebase

## Webhooks & Callbacks

**Incoming:**
- Postmark — Endpoints:
  - `POST /webhooks/postmark/inbound` — Inbound mail processing
  - `POST /webhooks/postmark/bounce` — Bounce event handling
  - `POST /webhooks/postmark/delivery` — Delivery confirmation
  - `POST /webhooks/postmark/spam` — Spam complaint handling
  - Verification: HMAC-SHA256 signature check against `X-Postmark-Signature` header using `POSTNEST_POSTMARK_WEBHOOK_SECRET`; falls back to `X-Postmark-Server-Token` comparison
  - Deduplication: Redis `SetNX` keyed by `MessageID` with 5-minute TTL (fail-open on Redis error)
  - Enqueue: Each verified webhook enqueues a JSON job to Redis `queue:jobs` for background worker processing

**Outgoing:**
- None — No outgoing webhooks to external systems. SMTP relay to Postmark is performed via direct API call, not webhook.

---

*Integration audit: 2025-07-28*
*Update when adding/removing external services*
