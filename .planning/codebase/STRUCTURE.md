# PostNest Directory Structure

## Layout

```
.
├── cmd/
│   ├── migrate/main.go       # Database migration CLI entrypoint
│   ├── server/main.go        # HTTP + IMAP + SMTP + DAV server entrypoint
│   └── worker/main.go        # Background worker pool entrypoint
│
├── internal/
│   ├── api/
│   │   ├── errors.go           # AppError, sentinel errors, WriteError
│   │   ├── errors_test.go      # Error wrapping tests
│   │   ├── middleware.go       # RequestID, logger, recovery, CORS, auth, rate limit
│   │   └── middleware_test.go  # Middleware unit tests
│   │
│   ├── auth/
│   │   └── auth.go             # Argon2id, sessions, API keys, user/domain helpers
│   │
│   ├── certmanager/
│   │   └── manager.go          # ACME certificate lifecycle (lego + Cloudflare)
│   │
│   ├── config/
│   │   ├── config.go           # Config struct definition
│   │   ├── loader.go           # TOML + env override loader with validation
│   │   ├── loader_test.go      # Config loader tests
│   │   └── template.go         # PrintTemplate for generating sample TOML
│   │
│   ├── contacts/
│   │   └── contacts.go         # Store interface + PGStore implementation
│   │
│   ├── dav/
│   │   └── dav.go              # CardDAV/CalDAV handler + backends
│   │
│   ├── db/
│   │   └── db.go               # pgxpool wrapper with ping lifecycle
│   │
│   ├── imap/
│   │   ├── backend.go          # IMAP backend: user, mailbox, message ops
│   │   └── imap.go             # Server wrapper (go-imap)
│   │
│   ├── logger/
│   │   └── logger.go           # slog JSON handler factory
│   │
│   ├── mailstore/
│   │   ├── mailstore.go        # Store interface (canonical contract)
│   │   └── pgstore.go          # PostgreSQL implementation
│   │
│   ├── migrate/
│   │   ├── migrate.go          # Embedded migration runner (golang-migrate)
│   │   └── migrations/
│   │       ├── 000001_init.up.sql
│   │       ├── 000002_fts.up.sql
│   │       ├── 000003_seed_labels.up.sql
│   │       ├── 000004_fts_trigger.up.sql
│   │       └── 000005_search_composite.up.sql
│   │
│   ├── models/
│   │   └── models.go           # Pure data structs (User, Message, Label, Thread, etc.)
│   │
│   ├── postmark/
│   │   └── postmark.go         # Postmark client wrapper + inbound structs
│   │
│   ├── redis/
│   │   ├── redis.go            # Redis client wrapper + queue helpers
│   │   └── redis_test.go       # Redis helper tests
│   │
│   ├── reputation/
│   │   └── reputation.go       # Whitelist/blacklist/greylist engine
│   │
│   ├── search/
│   │   └── search.go           # PostgreSQL tsvector indexer helpers
│   │
│   ├── smtp/
│   │   ├── smtp.go             # SMTP server wrapper + session backend
│   │   └── smtp_test.go        # SMTP tests
│   │
│   ├── webhook/
│   │   ├── webhook.go          # Postmark webhook receiver + dedup + enqueue
│   │   └── webhook_test.go     # Webhook handler tests
│   │
│   ├── webmail/
│   │   ├── webmail.go          # REST API handlers (labels, messages, drafts, search)
│   │   └── webmail_test.go     # Webmail handler tests
│   │
│   └── workers/
│       ├── bounce.go           # Bounce job processor
│       ├── delivery.go           # Delivery confirmation processor
│       ├── inbound.go            # Inbound mail processor
│       ├── send.go               # Draft send processor
│       ├── workers.go            # Pool, Job, Processor definitions
│       └── workers_test.go       # Worker pool tests
│
├── design/
│   ├── ARCHITECTURE.md           # Original design architecture
│   ├── API-SPEC.md               # REST endpoint specification
│   ├── COMPONENT-DESIGN.md       # Package layout design doc
│   ├── DATABASE-SCHEMA.md        # Schema design document
│   ├── DEPLOYMENT-ARCHITECTURE.md # Deployment specs
│   ├── DESIGN-SUMMARY.md         # Executive design summary
│   ├── PROTOCOL-DESIGN.md        # IMAP/SMTP/DAV protocol details
│   └── REQUIREMENTS-DEPLOYMENT.md # Deployment requirements
│
├── docs/                         # Additional documentation
│
├── nix/
│   └── (flake.nix at root)       # Nix flake + NixOS module
│
├── scripts/
│   ├── install-docker.sh         # Docker Compose deployment script
│   └── install-systemd.sh        # systemd service installation script
│
├── Dockerfile.migrate            # Migration runner image
├── Dockerfile.server             # Server image
├── Dockerfile.worker             # Worker image
├── docker-compose.yml            # Full stack compose (postgres, redis, server, worker, migrate)
├── go.mod                        # Module definition (Go 1.25)
├── go.sum                        # Checksums
├── flake.nix                     # Nix flake entrypoint
└── README.md                     # Project README
```

---

## Naming Conventions

### Packages
- Single-word, lowercase, descriptive of domain responsibility (`mailstore`, `webmail`, `certmanager`)
- No `pkg/` or `lib/` prefixes; everything lives under `internal/`

### Files
- `foo.go` — primary implementation
- `foo_test.go` — corresponding tests
- `migrations/NNNNNN_description.up.sql` — golang-migrate sequential numbering

### Types
- **Interfaces**: noun describing capability (`Store`, `Processor`, `DomainLister`)
- **Implementations**: `PG` + interface name (`PGStore`, `PGStore` for contacts)
- **Handlers**: `Handler` struct with `RegisterRoutes(r chi.Router)` method
- **Services**: `Service` struct with constructor `NewService(...)`
- **Servers**: `Server` struct with `Start() error` and `Stop() error`

### Functions
- Constructors: `New<Thing>(...)`
- HTTP handlers: lowercase verb-noun (`listLabels`, `getMessage`, `sendDraft`)
- Middleware factories: `Require<Thing>(svc)` returning `func(http.Handler) http.Handler`

### Variables
- Config struct fields use Go tags with `env:` and `envDefault:`
- Context keys: unexported `ctxKey` string type to avoid collisions
- Redis queue names: exported constants in `workers.go`

---

## Key File-to-Responsibility Map

| Concern | Primary File |
|---------|-------------|
| HTTP routing & middleware | `internal/api/middleware.go` |
| REST API (labels, messages, drafts, search) | `internal/webmail/webmail.go` |
| Webhook receiver (inbound, bounce, delivery, spam) | `internal/webhook/webhook.go` |
| IMAP protocol adapter | `internal/imap/backend.go` |
| SMTP protocol adapter | `internal/smtp/smtp.go` |
| CardDAV/CalDAV adapter | `internal/dav/dav.go` |
| Mail persistence interface | `internal/mailstore/mailstore.go` |
| Mail persistence implementation | `internal/mailstore/pgstore.go` |
| Auth (passwords, sessions, API keys) | `internal/auth/auth.go` |
| Background job pool | `internal/workers/workers.go` |
| Postmark client | `internal/postmark/postmark.go` |
| Config loading (TOML + env) | `internal/config/loader.go` |
| Database pool | `internal/db/db.go` |
| Redis client + queue ops | `internal/redis/redis.go` |
| ACME certificate manager | `internal/certmanager/manager.go` |
| Data models (structs) | `internal/models/models.go` |
| Migration runner | `internal/migrate/migrate.go` |

---

## Test Coverage Summary

| Package | Test Files |
|---------|-----------|
| `internal/api` | `errors_test.go`, `middleware_test.go` |
| `internal/config` | `loader_test.go` |
| `internal/redis` | `redis_test.go` |
| `internal/smtp` | `smtp_test.go` |
| `internal/webhook` | `webhook_test.go` |
| `internal/webmail` | `webmail_test.go` |
| `internal/workers` | `workers_test.go` |
