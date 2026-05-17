# Postnest Directory Structure

## Root Layout

```
.
├── cmd/                    # Application entry points
│   ├── server/
│   │   └── main.go         # HTTP + IMAP + SMTP + DAV server
│   └── worker/
│       └── main.go         # Background job worker pool
├── internal/               # Private application code
│   ├── api/                # HTTP middleware and shared API helpers
│   ├── auth/               # Authentication and session service
│   ├── certmanager/        # ACME TLS certificate lifecycle
│   ├── config/             # Configuration loader (TOML + env)
│   ├── contacts/           # Contact storage interface + PG implementation
│   ├── dav/                # CardDAV/CalDAV HTTP handler and backends
│   ├── db/                 # PostgreSQL connection pool wrapper
│   ├── imap/               # IMAP server and backend implementation
│   ├── logger/             # Structured JSON logger factory
│   ├── mailstore/          # Mail storage interface + PG implementation
│   ├── migrate/            # Database migrations (embedded SQL)
│   ├── models/             # Canonical domain structs
│   ├── postmark/           # Postmark API client and inbound payload parsing
│   ├── redis/              # Redis client wrapper + queue helpers
│   ├── reputation/         # Spam/reputation engine
│   ├── search/             # Full-text search indexer
│   ├── smtp/               # SMTP server and backend implementation
│   ├── webhook/            # Postmark webhook handlers
│   ├── webmail/            # REST API handlers for webmail
│   └── workers/            # Job queue pool and processors
├── docs/                     # Documentation
├── scripts/                  # Build / utility scripts
├── design/                   # Design documents
├── docker-compose.yml        # Local infrastructure stack
├── Dockerfile.server          # Server container image
├── Dockerfile.worker          # Worker container image
├── Dockerfile.migrate         # Migration container image
├── go.mod / go.sum           # Go module definition
├── flake.nix                 # Nix development shell
├── README.md
├── AGENTS.md
├── INTEGRATION.md
└── PLAN.md
```

## Key Locations

### Entry Points
- `cmd/server/main.go` — Initializes all server subsystems (HTTP, IMAP, SMTP, DAV) and handles graceful shutdown.
- `cmd/worker/main.go` — Initializes the Redis-backed worker pool and registers job processors.

### Configuration
- `internal/config/config.go` — The unified `Config` struct used throughout the application.
- `internal/config/loader.go` — TOML file parser + environment variable override logic.
- `internal/config/template.go` — (if present) Default configuration template generation.

### Domain Models
- `internal/models/models.go` — All domain structs: `User`, `Domain`, `Message`, `Label`, `Attachment`, `Thread`, `Contact`, `DeliveryLog`, `AuthSession`.

### Storage Layer
- `internal/mailstore/mailstore.go` — `Store` interface: the contract for all mail persistence.
- `internal/mailstore/pgstore.go` — `PGStore` implementation using raw `pgx` SQL.
- `internal/contacts/contacts.go` — `Store` interface and `PGStore` for contacts.
- `internal/db/db.go` — Thin `Pool` wrapper around `pgxpool.Pool` with lifecycle helpers.

### HTTP Layer
- `internal/api/middleware.go` — `RequestID`, `StructuredLogger`, `Recovery`, `CORS`, `RequireSession`, `RequireDomainAdmin`, `RateLimiter`.
- `internal/api/errors.go` — Unified `AppError` type and JSON error writer.
- `internal/webmail/webmail.go` — REST handlers for labels, messages, threads, drafts, search.
- `internal/webhook/webhook.go` — Postmark inbound/bounce/delivery/spam webhook receivers.
- `internal/dav/dav.go` — CardDAV (implemented) and CalDAV (stub) HTTP handler with Basic Auth middleware.

### Mail Protocols
- `internal/imap/imap.go` — IMAP server wrapper (`go-imap`).
- `internal/imap/backend.go` — IMAP backend: login, mailbox listing, message fetch, flags, search, copy, expunge.
- `internal/smtp/smtp.go` — SMTP server wrapper (`go-smtp`) with PLAIN and LOGIN SASL.

### Workers
- `internal/workers/workers.go` — `Pool`, `Job`, and `Processor` interfaces; Redis queue operations.
- `internal/workers/inbound.go` — Processes Postmark inbound payloads.
- `internal/workers/send.go` — Sends draft messages via Postmark.
- `internal/workers/bounce.go` — Records bounce events in `delivery_logs`.
- `internal/workers/delivery.go` — Records delivery confirmations.
- `internal/workers/spam.go` — Processes spam complaints.

### Infrastructure Clients
- `internal/postmark/postmark.go` — Outbound send wrapper and inbound payload parsing.
- `internal/redis/redis.go` — Redis list/sorted-set queue primitives (`Enqueue`, `Dequeue`, `PromoteReadyDelayed`, `EnqueueDead`).
- `internal/certmanager/manager.go` — ACME account management, certificate obtainment, and background renewal.
- `internal/logger/logger.go` — JSON `slog` factory.

### Migration Files
- `internal/migrate/migrate.go` — Migration runner using `golang-migrate/migrate` with embedded SQL.
- `internal/migrate/migrations/000001_init.up.sql`
- `internal/migrate/migrations/000002_fts.up.sql`
- `internal/migrate/migrations/000003_seed_labels.up.sql`
- `internal/migrate/migrations/000004_fts_trigger.up.sql`
- `internal/migrate/migrations/000005_search_composite.up.sql`

## Naming Conventions

- **Packages**: lowercase, no underscores (`mailstore`, `certmanager`).
- **Files**: lowercase with underscore separators (`pgstore.go`, `middleware_test.go`).
- **Interfaces**: Noun describing behavior (`Store`, `Processor`, `DomainLister`).
- **Implementations**: Prefix with technology (`PGStore`, `InboundProcessor`).
- **Constructors**: `New` or `New<T>` (`NewPool`, `NewInboundProcessor`).
- **Errors**: Package-level exported vars for common cases (`ErrNotFound`, `ErrUnauthorized`).
- **Context keys**: Unexported string type to avoid collisions (`ctxKeyUser`).
- **Test files**: `*_test.go` adjacent to the code under test.
- **Config env vars**: `POSTNEST_<SECTION>_<KEY>` (e.g., `POSTNEST_DATABASE_DSN`), with legacy fallback support.
