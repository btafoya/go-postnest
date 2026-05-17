# Technology Stack — PostNest

## Language & Runtime

| Item | Version / Details |
|------|-------------------|
| **Language** | Go 1.25 |
| **Module path** | `github.com/go-postnest/postnest` |
| **Build mode** | `CGO_ENABLED=0` static binaries |
| **Base image** | `gcr.io/distroless/static-debian12:nonroot` (server, worker, migrate) |
| **Builder image** | `golang:1.25-alpine` |

## Application Binaries

Three binaries are produced from `cmd/`:

- **`postnest-server`** — HTTP REST + WebDAV/CardDAV/CalDAV + IMAP + SMTP
- **`postnest-worker`** — Redis-backed background job processors
- **`postnest-migrate`** — Embedded SQL migration runner

## Core Frameworks & Libraries

### HTTP & Routing
- **`github.com/go-chi/chi/v5`** — Router and middleware chain

### Email Protocols
- **`github.com/emersion/go-imap`** — IMAP4rev1 server (backend, IDLE, SEARCH, FETCH, APPEND, EXPUNGE, COPY)
- **`github.com/emersion/go-smtp`** — SMTP submission server
- **`github.com/emersion/go-sasl`** — SASL authentication (PLAIN, LOGIN)
- **`github.com/emersion/go-message`** — MIME parsing and RFC822 generation

### DAV & Contacts
- **`github.com/emersion/go-webdav`** — WebDAV framework (CardDAV + CalDAV stubs)
- **`github.com/emersion/go-vcard`** — vCard 4.0 parsing/serialization
- **`github.com/emersion/go-ical`** — iCalendar parsing (CalDAV dependency)

### Database & Migrations
- **`github.com/jackc/pgx/v5`** — PostgreSQL driver and connection pool (`pgxpool`)
- **`github.com/golang-migrate/migrate/v4`** — Schema migrations with embedded SQL (`embed.FS`)
- **`github.com/lib/pq`** — Indirect dependency (used by migrate)

### Cache & Queues
- **`github.com/redis/go-redis/v9`** — Redis client (job queues, IMAP IDLE pub/sub)

### External Services
- **`github.com/mrz1836/postmark`** — Postmark REST API client (outbound send)
- **`github.com/go-acme/lego/v4`** — ACME/Let's Encrypt certificate lifecycle
  - `certcrypto`, `certificate`, `lego`, `registration`
  - DNS-01 provider: `providers/dns/cloudflare`

### Security & Crypto
- **`golang.org/x/crypto`** — Argon2id password hashing
- Standard library: `crypto/tls`, `crypto/sha256`, `crypto/rand`

### Utilities
- **`github.com/google/uuid`** — UUID generation and parsing
- **`github.com/BurntSushi/toml`** — TOML configuration parsing
- **`log/slog`** — Structured JSON logging (standard library)

### Indirect Dependencies
- `github.com/cenkalti/backoff/v5`, `github.com/cespare/xxhash/v2`, `github.com/go-jose/go-jose/v4`
- `github.com/miekg/dns` — DNS resolution (ACME/lego)
- `github.com/teambition/rrule-go` — Recurrence rules (ical)
- `github.com/microcosm-cc/bluemonday` — HTML sanitization (indirect)

## Datastores

### PostgreSQL 16+
- **Primary datastore** for all persistent data
- **Extensions used:**
  - `pgcrypto` — UUID generation
  - `pg_trgm` — Trigram similarity
- **Full-text search:** `tsvector`/`tsquery` with weighted ranks (A=subject, B=from, C=body, D=to)
- **Trigger:** `messages_update_search_vector()` (BEFORE INSERT OR UPDATE)
- **Indexes:** GIN on `search_vector`, composite on `(domain_id, user_id, ...)`, partial indexes on `postmark_message_id`

### Redis 7+
- **Background job queue** — Redis lists (`LPush` / `BRPop`)
- **Delayed jobs** — Redis sorted sets (`ZAdd` / `ZRangeByScore`)
- **Dead-letter queue** — Failed job retry exhaustion
- **IMAP IDLE pub/sub** — `Publish`/`Subscribe` for mailbox change notifications

## Configuration

### Sources (precedence: env overrides > TOML file > defaults)
1. **TOML file** — `/etc/postnest/postnest.conf` (or `POSTNEST_CONFIG_PATH`)
2. **Environment variables** — `POSTNEST_<SECTION>_<KEY>` (e.g., `POSTNEST_DATABASE_DSN`)
3. **Legacy env vars** — Backward-compatible names (`POSTGRES_DSN`, `SESSION_KEY`, etc.)

### Config sections
- `[server]` — HTTP/IMAP/SMTP listen addresses, timeouts, CORS origins
- `[database]` — DSN, read replica DSN, max connections
- `[redis]` — Connection URL
- `[tls]` — Certificate/key paths
- `[acme]` — Automated TLS (email, domain, directory, DNS provider, renew intervals)
- `[worker]` — Concurrency and poll interval
- `[security]` — Session key, Argon2id params, max message/attachment sizes
- `[postmark]` — Webhook secret

## Build & Deployment Artifacts

### Docker Compose (`docker-compose.yml`)
| Service | Image / Build | Ports | Purpose |
|---------|---------------|-------|---------|
| `postgres` | `postgres:16-alpine` | — | Primary database |
| `redis` | `redis:7-alpine` | — | Cache & job queue |
| `server` | `Dockerfile.server` | 8080, 143, 587 | HTTP + IMAP + SMTP |
| `worker` | `Dockerfile.worker` | — | Background workers |
| `migrate` | `Dockerfile.migrate` | — | One-shot migrations |

### Dockerfiles
- Multi-stage: `golang:1.25-alpine` builder → `distroless/static-debian12:nonroot`
- Exposed ports in server image: `8080 143 587 993 465`

### Nix
- **`flake.nix`** — Dev shell, package derivations, NixOS module entrypoint
- **`nix/module.nix`** — NixOS service declarations for server, worker, migrate

### Scripts
- `scripts/install-docker.sh` — Docker Compose bootstrap
- `scripts/install-systemd.sh` — Native systemd unit installation (Linux)

## Testing Utilities
- **`github.com/alicebob/miniredis/v2`** — In-memory Redis for unit tests
- Standard `testing` package with `httptest`

## Schema Migrations (Embedded)
Files under `internal/migrate/migrations/`:
1. `000001_init.up.sql` — Core schema (domains, users, messages, labels, contacts, reputation, delivery logs, webhook events, bounce events)
2. `000002_fts.up.sql` — `pg_trgm` extension + `messages_update_search_vector()` function
3. `000003_seed_labels.up.sql` — System labels (INBOX, SENT, DRAFTS, TRASH, JUNK, IMPORTANT, STARRED, ALL_MAIL)
4. `000004_fts_trigger.up.sql` — Trigger binding search vector function to messages table
5. `000005_search_composite.up.sql` — Composite index `messages(domain_id, user_id)`
