# PostNest Directory Structure

## 1. Top-Level Layout

```
/home/btafoya/projects/go-postnest/
├── cmd/                    # Binary entry points (server, worker, migrate)
├── design/                 # Architecture & design documentation
├── docs/                   # Additional docs (mirrors design/)
├── internal/               # All application code (Go module convention)
├── nix/                    # Nix flake and NixOS module
├── scripts/                # Installation helpers (systemd, Docker)
├── .env.example            # Required environment variables
├── docker-compose.yml      # Local orchestration (postgres, redis, server, worker, migrate)
├── Dockerfile.server       # OCI image for cmd/server
├── Dockerfile.worker       # OCI image for cmd/worker
├── Dockerfile.migrate      # OCI image for cmd/migrate
├── flake.nix               # Nix flake (packages + devShell + nixosModule)
├── go.mod                  # Module: github.com/go-postnest/postnest, Go 1.25
├── go.sum                  # Dependency checksums
├── INTEGRATION.md          # Implementation status & known limitations
├── README.md               # User-facing quick-start and feature list
├── PLAN.md                 # Development plan
├── AGENTS.md               # Agent instructions
└── .planning/codebase/     # This document set
```

## 2. `cmd/` — Binaries

| Path | Purpose | Artifact |
|---|---|---|
| `cmd/server/main.go` | Wires and starts HTTP, IMAP, SMTP, DAV, health checks, graceful shutdown | `postnest-server` binary / `Dockerfile.server` |
| `cmd/worker/main.go` | Starts Redis-backed worker pool with registered processors | `postnest-worker` binary / `Dockerfile.worker` |
| `cmd/migrate/main.go` | CLI for `up`, `down`, `version`, `force` using embedded migrations | `postnest-migrate` binary / `Dockerfile.migrate` |

No other files exist under `cmd/`; each subdirectory contains exactly one `main.go`.

## 3. `internal/` — Package Organization

### 3.1 Shared Infrastructure

| Package | Key Files | Purpose |
|---|---|---|
| `internal/config/` | `config.go`, `loader.go`, `loader_test.go`, `template.go` | TOML + env-var configuration with backward-compatible legacy env names |
| `internal/db/` | `db.go` | `pgxpool.Pool` wrapper with `Ping()` and `Close()` |
| `internal/redis/` | `redis.go` | `go-redis/v9` wrapper; adds `Enqueue`, `Dequeue`, `Publish` helpers |
| `internal/logger/` | `logger.go` | JSON `slog` constructor |
| `internal/certmanager/` | `manager.go` | ACME/Let's Encrypt certificate lifecycle (lego-based) |

### 3.2 Domain Models

| Package | Key Files | Purpose |
|---|---|---|
| `internal/models/` | `models.go` | Pure structs: `User`, `Domain`, `DomainMember`, `Message`, `Label`, `Attachment`, `Thread`, `Contact`, `DeliveryLog`, `AuthSession` |

### 3.3 Domain Services & Stores

| Package | Key Files | Purpose |
|---|---|---|
| `internal/mailstore/` | `mailstore.go` (interface), `pgstore.go` (PostgreSQL impl) | Canonical mail persistence abstraction |
| `internal/contacts/` | `contacts.go` | Contact store interface + PostgreSQL implementation |
| `internal/auth/` | `auth.go` | Argon2id hashing, session/API-key creation & validation, domain membership queries |
| `internal/reputation/` | `reputation.go` | Whitelist/blacklist/greylist decisions + per-contact reputation updates |
| `internal/search/` | `search.go` | Async `tsvector` indexing queue + batch processor |

### 3.4 Protocol Adapters

| Package | Key Files | Purpose |
|---|---|---|
| `internal/api/` | `middleware.go`, `errors.go` | Shared HTTP middleware (RequestID, logger, recovery, CORS, session auth, domain-admin guard) and unified error types |
| `internal/webmail/` | `webmail.go` | REST handlers for labels, messages, threads, drafts, batch ops, search |
| `internal/webhook/` | `webhook.go` | Postmark webhook receiver (inbound, bounce, delivery, spam) — validates signature and enqueues jobs |
| `internal/imap/` | `imap.go` (server wrapper), `backend.go` (backend impl) | `go-imap` server: LOGIN, LIST, STATUS, FETCH, SEARCH, APPEND, EXPUNGE, COPY, flag updates, IDLE |
| `internal/smtp/` | `smtp.go` | `go-smtp` server: AUTH PLAIN, DATA relay to Postmark, Sent-item persistence, TLS support |
| `internal/dav/` | `dav.go` | CardDAV backend (list/get/put/delete vCards); CalDAV stub; WebDAV placeholder |

### 3.5 External Integrations

| Package | Key Files | Purpose |
|---|---|---|
| `internal/postmark/` | `postmark.go` | Outbound send via `mrz1836/postmark`, inbound webhook payload parsing |

### 3.6 Background Workers

| Package | Key Files | Purpose |
|---|---|---|
| `internal/workers/` | `workers.go` (pool), `inbound.go`, `bounce.go`, `delivery.go` | Redis-backed job pool with retry logic; three concrete processors |

### 3.7 Schema Migrations

| Package | Key Files | Purpose |
|---|---|---|
| `internal/migrate/` | `migrate.go`, `migrations/000001_init.up.sql`, `migrations/000002_fts.up.sql`, `migrations/000003_seed_labels.up.sql` | Embedded migration runner using `golang-migrate/migrate/v4` |

## 4. Module Boundaries and Package Organization

- **No `pkg/` directory** — all code is `internal/`, meaning it is not importable by external modules.
- **Interface-first within domain packages**:
  - `internal/mailstore/mailstore.go` declares the `Store` interface.
  - `internal/mailstore/pgstore.go` provides `PGStore`.
  - This pattern is repeated in `internal/contacts/contacts.go`.
- **Dependency direction**: protocol packages (`imap`, `smtp`, `webmail`, `dav`) depend on domain interfaces (`mailstore.Store`, `contacts.Store`, `auth.Service`). Domain packages never import protocol packages.
- **Infrastructure packages** (`db`, `redis`, `postmark`, `certmanager`) are at the bottom of the dependency graph.
- **Models package** (`internal/models`) is imported by almost everything and contains no logic.

## 5. Naming Conventions

- **Package names**: short, lowercase, no underscores (`mailstore`, `webmail`, `certmanager`).
- **Files**: named after the primary type or concern (`mailstore.go`, `pgstore.go`, `middleware.go`, `errors.go`).
- **Constructors**: `New*` or `NewPG*` (e.g., `NewPGStore`, `NewPool`, `NewHandler`, `NewServer`).
- **Interfaces**: noun describing capability (`Store`, `Processor`).
- **Implementations**: prefixed with concrete technology (`PGStore`, `InboundProcessor`, `carddavBackend`).
- **Context keys**: unexported `ctxKey` string type to avoid collisions.

## 6. Where New Features Typically Get Added

| Feature Type | Typical Packages | Notes |
|---|---|---|
| New REST endpoint | `internal/webmail/webmail.go`, `internal/api/middleware.go` | Mount route in `webmail.Handler.RegisterRoutes()`; reuse `mailstore.Store` |
| New IMAP capability | `internal/imap/backend.go`, `internal/imap/imap.go` | Extend `imapMailbox` or `imapUser` methods |
| New SMTP behavior | `internal/smtp/smtp.go` | Modify `smtpSession` methods |
| New background job type | `internal/workers/*.go` | Implement `Processor`, register in `cmd/worker/main.go` |
| New database entity | `internal/models/models.go`, `internal/migrate/migrations/`, relevant store | Add struct, migration, and store method |
| New search capability | `internal/search/search.go`, `internal/mailstore/pgstore.go` | Update `tsvector` columns or `Search()` query |
| New contact/CalDAV feature | `internal/dav/dav.go`, `internal/contacts/contacts.go` | Extend CardDAV backend or implement CalDAV stub |
| New auth mechanism | `internal/auth/auth.go`, `internal/api/middleware.go` | Add middleware guard or token type |
| New deployment target | `nix/module.nix`, `scripts/`, new `Dockerfile.*` | Follow existing systemd/Docker patterns |

## 7. Build Artifacts and Deployment Files

| File | Purpose |
|---|---|
| `Dockerfile.server` | Multi-stage build (`golang:1.25-alpine` → `distroless/static-debian12:nonroot`); exposes 8080, 143, 587, 993, 465 |
| `Dockerfile.worker` | Same builder pattern; no exposed ports |
| `Dockerfile.migrate` | Same builder pattern; runs migration CLI |
| `docker-compose.yml` | Defines `postgres`, `redis`, `server`, `worker`, `migrate` services with health checks and env var wiring |
| `scripts/install-docker.sh` | Creates `.env`, starts infra, runs migrations, starts app containers |
| `scripts/install-systemd.sh` | Interactive/non-interactive systemd installer: creates user, config, units, enables services |
| `nix/module.nix` | NixOS module declaring `services.postnest` with PostgreSQL + Redis integration and hardened systemd units |
| `flake.nix` | Exposes packages (`postnest-server`, `postnest-worker`), dev shell, and `nixosModules.postnest` |
| `.env.example` | Template for required secrets and DSN variables |

## 8. Config and Secrets

- **Primary config**: TOML file at `/etc/postnest/postnest.conf` (or `POSTNEST_CONFIG_PATH`).
- **Environment overrides**: `POSTNEST_<SECTION>_<KEY>` pattern; legacy names (`POSTGRES_DSN`, `SESSION_KEY`) still supported.
- **Template generator**: `internal/config/template.go` `PrintTemplate()` produces a documented TOML file.
- **Loader**: `internal/config/loader.go` reads TOML reflectively and applies env overrides.
- **Required secrets**: `POSTNEST_SECURITY_SESSION_KEY`, `POSTNEST_POSTMARK_WEBHOOK_SECRET`.
