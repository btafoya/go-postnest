# PostNest Technology Stack

## Language & Runtime

| Layer | Choice | Version |
|---|---|---|
| Language | Go | 1.25.0 |
| Module path | `github.com/go-postnest/postnest` | — |

Go is used throughout with `CGO_ENABLED=0` for static binary builds targeting `linux`.

## Build & Deployment

| Tool | Purpose |
|---|---|
| `Dockerfile.server` | Multi-stage build: `golang:1.25-alpine` → `gcr.io/distroless/static-debian12:nonroot` |
| `Dockerfile.worker` | Same pattern for worker binary |
| `Dockerfile.migrate` | Same pattern for migration binary |
| `docker-compose.yml` | Full stack: PostgreSQL 16, Redis 7, server, worker, migrate |
| `flake.nix` / `nix/module.nix` | NixOS module + dev shell with `go`, `golangci-lint`, `air`, `postgresql_16`, `redis`, `go-migrate` |

**Exposed ports (server container):** 8080 (HTTP), 143 (IMAP), 587 (SMTP), 993 (IMAP TLS), 465 (SMTP TLS)

## Frameworks & Routers

| Package | Role |
|---|---|
| `github.com/go-chi/chi/v5` | HTTP router for REST API, webhooks, and DAV mounts |
| `github.com/emersion/go-imap` | IMAP4rev1 server framework |
| `github.com/emersion/go-smtp` | SMTP submission server framework |
| `github.com/emersion/go-webdav` | WebDAV/CardDAV/CalDAV server framework |
| `github.com/emersion/go-sasl` | SASL authentication (PLAIN, LOGIN) for SMTP/IMAP |
| `github.com/emersion/go-message` | RFC 822/MIME parsing and generation |

## Data Stores

| Store | Driver / Client | Usage |
|---|---|---|
| **PostgreSQL** | `github.com/jackc/pgx/v5` + `pgxpool` | Primary datastore: users, domains, messages, threads, labels, contacts, delivery logs, reputation, webhook events |
| **Redis** | `github.com/redis/go-redis/v9` | Job queue (`LPush`/`BRPop`), delayed jobs (`ZAdd`/`ZRangeByScore`), dead-letter queue, IMAP IDLE pub/sub |

### PostgreSQL Features Used
- `pgcrypto` extension (UUID generation)
- Full-text search (`tsvector`, `GIN` index)
- Triggers for search vector updates (migration `000004_fts_trigger.up.sql`)
- Composite indexes for domain-scoped queries

### Migration Tool
- `github.com/golang-migrate/migrate/v4` (embedded via `internal/migrate`)
- Migrations embedded with `//go:embed migrations/*.sql`
- Files: `000001_init.up.sql`, `000002_fts.up.sql`, `000003_seed_labels.up.sql`, `000004_fts_trigger.up.sql`, `000005_search_composite.up.sql`

## Configuration

| Source | Details |
|---|---|
| **Primary** | TOML file (`/etc/postnest/postnest.conf` or `POSTNEST_CONFIG_PATH`) via `github.com/BurntSushi/toml` |
| **Overrides** | Environment variables with pattern `POSTNEST_<SECTION>_<KEY>`; legacy names (`POSTGRES_DSN`, `SESSION_KEY`, etc.) supported for backward compatibility |
| **Schema** | `internal/config/template.go` prints documented TOML template; `internal/config/loader.go` implements reflective env override logic |

Key config sections: `server`, `database`, `redis`, `security`, `postmark`, `tls`, `worker`, `acme`.

## Security & Cryptography

| Concern | Implementation |
|---|---|
| Password hashing | Argon2id (`golang.org/x/crypto/argon2`) |
| Session tokens | Random 32-byte tokens, SHA-256 hashed in DB |
| API keys | Same token scheme as sessions |
| TLS certificates | File-based (`POSTNEST_TLS_CERT_PATH` / `POSTNEST_TLS_KEY_PATH`) or ACME via `github.com/go-acme/lego/v4` |
| Rate limiting | In-memory token bucket (`internal/api/middleware.go`) |
| CORS | Configurable origin whitelist |

## Background Job System

Custom Redis-backed worker pool in `internal/workers`:
- Job types: `inbound`, `bounce`, `delivery`
- Concurrency: configurable (`POSTNEST_WORKER_CONCURRENCY`, default 10)
- Polling interval: configurable (`POSTNEST_WORKER_POLL_INTERVAL`, default 5s)
- Retry logic with exponential backoff and dead-letter queue

## Logging

Structured JSON logging via Go stdlib `log/slog` (`internal/logger`).

## Testing

| Tool | Usage |
|---|---|
| `github.com/alicebob/miniredis/v2` | In-memory Redis for unit tests |
| `pgx` | PostgreSQL driver also used in integration contexts |

## Notable Absences

- No ORM (raw SQL via `pgx`)
- No message queue aside from Redis lists/sorted sets
- No caching layer aside from Redis job structures
- No frontend framework (API-only backend)
