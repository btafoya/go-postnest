# PostNest Technology Stack

> Practical reference for the PostNest multi-tenant mail platform.
> All file paths are relative to the repository root.

## Languages and Runtime

- **Go 1.25.0** — Primary language. Declared in `go.mod` (line 3).
- **Build constraints** — `CGO_ENABLED=0` static binaries for all container images (`Dockerfile.server`, `Dockerfile.worker`, `Dockerfile.migrate`).
- **Target OS** — Linux (`GOOS=linux`), cross-compiled from Alpine builder stage.

## Core Frameworks and Libraries

- **HTTP routing** — `github.com/go-chi/chi/v5` (v5.2.5). Used in the REST API, webmail handler, and webhook handler.
  - `internal/api/middleware.go` — Request ID, structured logging, panic recovery, CORS, session/API-key auth middleware.
  - `internal/webmail/webmail.go` — Webmail REST routes mounted on chi.
  - `internal/webhook/webhook.go` — Postmark webhook routes mounted on chi.
  - `cmd/server/main.go` — Top-level router assembly.
- **MIME parsing** — `github.com/emersion/go-message` (v0.18.2). Used in SMTP session `Data()` for parsing inbound email.
- **SASL authentication** — `github.com/emersion/go-sasl` (v0.0.0-20241020182733-b788ff22d5a6). Used in `internal/smtp/smtp.go` for SMTP `AUTH PLAIN`.
- **UUID generation** — `github.com/google/uuid` (v1.6.0). Used pervasively across models, sessions, and messages.
- **Configuration parsing** — `github.com/BurntSushi/toml` (v1.6.0). Primary config format.
  - `internal/config/loader.go` — TOML loader with reflective env-var override.
  - `internal/config/config.go` — Legacy direct env-var loader (still present).
- **Structured logging** — Standard library `log/slog` via JSON handler. Wired in `cmd/server/main.go` and `cmd/worker/main.go`.

## Database and Storage

- **PostgreSQL 16** — Primary data store.
  - Container image: `postgres:16-alpine` (`docker-compose.yml`).
  - Go driver: `github.com/jackc/pgx/v5` (v5.9.2) with connection pooling (`pgxpool`).
  - `internal/db/db.go` — Thin wrapper around `pgxpool.Pool` with `New(dsn, maxConns)`.
  - `internal/mailstore/pgstore.go` — 400+ line PostgreSQL implementation of the mail store interface (messages, labels, threads, attachments, flags, search).
  - `internal/contacts/contacts.go` — PostgreSQL-backed contact store.
  - `internal/search/search.go` — Async full-text indexer backed by PostgreSQL `tsvector`.
  - `internal/reputation/reputation.go` — Reputation engine querying domain whitelist/blacklist rules from PostgreSQL.
- **Migrations** — `github.com/golang-migrate/migrate/v4` (v4.19.1).
  - `internal/migrate/migrate.go` — Wraps migrate with `embed.FS` for `migrations/*.sql`.
  - `cmd/migrate/main.go` — Standalone CLI for `up`, `down`, `version`, `force`.
  - Migration files live in `internal/migrate/migrations/` (e.g., `000001_init.up.sql`, `000002_fts.up.sql`, `000003_seed_labels.up.sql`).

## Cache and Queues

- **Redis 7** — Job queue and pub/sub.
  - Container image: `redis:7-alpine` (`docker-compose.yml`).
  - Go client: `github.com/redis/go-redis/v9` (v9.19.0).
  - `internal/redis/redis.go` — Thin wrapper providing `Publish`, `Enqueue` (LPush), and `Dequeue` (BRPop with timeout).
  - `internal/workers/workers.go` — Worker pool that consumes jobs via Redis blocking dequeue.
  - `internal/workers/bounce.go`, `internal/workers/inbound.go`, `internal/workers/delivery.go` — Job processors.

## Protocol Implementations

- **IMAP** — `github.com/emersion/go-imap` (v1.2.1).
  - `internal/imap/imap.go` — Server wrapper, listens on configurable address (default `:143`).
  - `internal/imap/backend.go` — Custom `backend.Backend` implementation mapping labels to mailboxes, messages to IMAP sequences.
- **SMTP** — `github.com/emersion/go-smtp` (v0.24.0).
  - `internal/smtp/smtp.go` — Server wrapper with custom `smtp.Backend` and `smtp.Session`.
  - Supports STARTTLS (ports 587/465) and `AUTH PLAIN` via go-sasl.
  - Outbound relay passes messages to Postmark client after SMTP `DATA` ingestion.
- **CardDAV/CalDAV/WebDAV** — `github.com/emersion/go-webdav` (v0.7.0), plus `go-vcard` and `go-ical`.
  - `internal/dav/dav.go` — Chi-mounted DAV handler with CardDAV backend fully implemented (contacts stored in PostgreSQL) and CalDAV stubbed (returns `not implemented`).
- **HTTP/REST** — Chi v5 router.
  - `cmd/server/main.go` assembles the HTTP server on `:8080`.
  - Route groups: webmail, contacts, DAV, webhooks, health.

## Deployment Tooling

- **Docker** — Multi-stage distroless builds.
  - `Dockerfile.server` — `golang:1.25-alpine` builder → `gcr.io/distroless/static-debian12:nonroot` runner. Exposes `8080`, `143`, `587`, `993`, `465`.
  - `Dockerfile.worker` — Same pattern, runs `cmd/worker`.
  - `Dockerfile.migrate` — Same pattern, runs `cmd/migrate`.
  - `docker-compose.yml` — Defines `postgres`, `redis`, `server`, `worker`, and `migrate` services with healthchecks and volume mounts.
- **Nix** — Reproducible builds and NixOS integration.
  - `flake.nix` — Defines `postnest-server`, `postnest-worker`, `postnest-migrate` packages; dev shell with `go`, `golangci-lint`, `air`, `postgresql_16`, `redis`, `go-migrate`.
  - `nix/module.nix` — NixOS module that generates `postnest.conf`, creates `postnest` user/group, provisions PostgreSQL/Redis, and installs systemd units (`postnest-server`, `postnest-worker`).
- **systemd** — Native Linux service installation.
  - `scripts/install-systemd.sh` — Bash installer that writes unit files for server and worker, creates DB/user, and generates TOML config.
- **TLS** — Two modes.
  - Static: mount host TLS cert/key paths into the container (`POSTNEST_TLS_CERT_PATH`, `POSTNEST_TLS_KEY_PATH`).
  - Dynamic ACME: `internal/certmanager/manager.go` uses `github.com/go-acme/lego/v4` to obtain and renew Let's Encrypt certificates via DNS-01 (Cloudflare provider).

## Configuration Approach

- **Primary format** — TOML file at `/etc/postnest/postnest.conf`.
  - `internal/config/loader.go` — Loads TOML, applies defaults, then overrides via `POSTNEST_<SECTION>_<KEY>` environment variables reflectively.
  - Legacy env vars are mapped for backward compatibility (e.g., `POSTGRES_DSN` → `POSTNEST_DATABASE_DSN`).
- **Environment variables** — Fully supported as overrides or standalone (legacy `internal/config/config.go` still exists).
- **Example environment** — `.env.example` documents required secrets (`POSTNEST_SECURITY_SESSION_KEY`, `POSTNEST_POSTMARK_WEBHOOK_SECRET`) and database password.

## Project Layout

```
cmd/server/main.go       # HTTP + IMAP + SMTP + DAV server
cmd/worker/main.go       # Background job worker
cmd/migrate/main.go      # Database migration CLI
internal/
  api/                   # HTTP middleware and error types
  auth/                  # Argon2id password hashing, sessions, API keys
  certmanager/           # ACME / lego TLS certificate lifecycle
  config/                # TOML + env configuration loading
  contacts/              # Contact persistence interface + PostgreSQL store
  dav/                   # CardDAV/CalDAV/WebDAV handler
  db/                    # pgxpool wrapper
  imap/                  # IMAP server and backend implementation
  logger/                # slog setup helpers
  mailstore/             # Mail persistence interface + PostgreSQL store
  migrate/               # golang-migrate wrapper with embedded SQL
  models/                # Domain models (User, Domain, Message, Label, etc.)
  postmark/              # Postmark API client wrapper
  redis/                 # go-redis wrapper for queues
  reputation/            # Spam evaluation / whitelist-blacklist engine
  search/                # PostgreSQL tsvector async indexer
  smtp/                  # SMTP server and session backend
  webhook/               # Postmark inbound/bounce/delivery/spam webhooks
  webmail/               # REST API for labels, messages, threads, drafts
  workers/               # Worker pool and job processors
```
