# Codebase Structure

**Analysis Date:** 2025-06-10

## Directory Layout

```
go-postnest/
├── cmd/                    # Executable entry points
│   ├── migrate/           # Database migration CLI
│   ├── server/            # Main server binary (HTTP+IMAP+SMTP)
│   └── worker/            # Background worker binary
├── internal/              # Private application code
│   ├── api/               # HTTP middleware, errors, context helpers
│   ├── auth/              # Authentication, sessions, password hashing
│   ├── certmanager/       # ACME/Let's Encrypt certificate manager
│   ├── config/            # Configuration loading (TOML + env)
│   ├── contacts/          # Contact/address book persistence
│   ├── dav/               # CardDAV/CalDAV/WebDAV server
│   ├── db/                # PostgreSQL connection pool wrapper
│   ├── imap/              # IMAP4rev1 server wrapper + backend
│   ├── logger/            # Structured JSON logger
│   ├── mailstore/         # Mail persistence interface + PostgreSQL impl
│   ├── migrate/           # Migration runner + embedded SQL files
│   │   └── migrations/    # golang-migrate SQL files
│   ├── models/            # Shared domain types
│   ├── postmark/          # Postmark API client
│   ├── redis/             # Redis client wrapper (queues, pub/sub)
│   ├── reputation/        # Spam evaluation engine
│   ├── search/            # Full-text search indexer
│   ├── smtp/              # SMTP server wrapper + backend
│   ├── webmail/           # Webmail REST API handlers
│   ├── webhook/           # Postmark webhook receiver
│   └── workers/           # Background job pool + processors
├── design/                # Design documents (requirements, specs)
├── docs/                  # Operational documentation
│   └── design/            # TLS/design docs
├── scripts/               # Deployment/install scripts
├── nix/                   # Nix module definition
├── .planning/             # Planning artifacts (this directory)
│   └── codebase/          # Architecture, structure, stack docs
├── docker-compose.yml     # Local development stack
├── Dockerfile.server      # Server container image
├── Dockerfile.worker      # Worker container image
├── Dockerfile.migrate     # Migration container image
├── flake.nix              # Nix flake for reproducible builds
├── go.mod                 # Go module definition
└── go.sum                 # Dependency checksums
```

## Directory Purposes

**`cmd/`: Executable entry points**
- Purpose: Three separate binaries sharing `internal/` packages
- Contains: `server/main.go` (starts all protocol servers), `worker/main.go` (job consumers), `migrate/main.go` (schema migrations)
- Key files: `cmd/server/main.go`, `cmd/worker/main.go`, `cmd/migrate/main.go`
- Subdirectories: `migrate/`, `server/`, `worker/`

**`internal/api/`: HTTP primitives**
- Purpose: Shared HTTP middleware, error types, request context helpers
- Contains: CORS, rate limiting, request ID, structured logging, panic recovery, auth middleware, error response writers
- Key files: `middleware.go`, `errors.go`
- Subdirectories: None

**`internal/auth/`: Identity and access**
- Purpose: User authentication, session management, password hashing, domain membership
- Contains: `auth.Service` with Argon2id passwords and SHA-256 session tokens
- Key files: `auth.go`, `auth_test.go`
- Subdirectories: None

**`internal/certmanager/`: TLS automation**
- Purpose: ACME certificate issuance and renewal
- Contains: `certmanager.Manager` using lego v4 with DNS-01 challenge
- Key files: `manager.go`
- Subdirectories: None

**`internal/config/`: Configuration**
- Purpose: Load and merge TOML config with environment variable overrides
- Contains: `Config` struct with env tags, `Loader` for TOML parsing, legacy env compatibility
- Key files: `config.go`, `loader.go`, `template.go`, `loader_test.go`
- Subdirectories: None

**`internal/contacts/`: Address book**
- Purpose: Contact persistence for CardDAV
- Contains: `contacts.Store` interface and `PGStore` implementation
- Key files: `contacts.go`
- Subdirectories: None

**`internal/dav/`: DAV protocols**
- Purpose: CardDAV (contacts), CalDAV stub, WebDAV stub
- Contains: `dav.Handler` with `go-webdav`/`go-ical`/`go-vcard` backends
- Key files: `dav.go`
- Subdirectories: None

**`internal/db/`: Database connectivity**
- Purpose: pgxpool wrapper with lifecycle helpers
- Contains: `db.Pool`
- Key files: `db.go`
- Subdirectories: None

**`internal/imap/`: IMAP server**
- Purpose: IMAP4rev1 protocol server using `go-imap`
- Contains: `imap.Server` wrapper and `imapBackend` for mailstore integration
- Key files: `imap.go`, `backend.go`
- Subdirectories: None

**`internal/logger/`: Logging**
- Purpose: JSON structured logger factory
- Contains: `logger.New()` returning `*slog.Logger`
- Key files: `logger.go`
- Subdirectories: None

**`internal/mailstore/`: Mail persistence**
- Purpose: Canonical abstraction for all mail storage
- Contains: `mailstore.Store` interface, `PGStore` implementation, message/label/thread/attachment CRUD
- Key files: `mailstore.go`, `pgstore.go`
- Subdirectories: None

**`internal/migrate/`: Schema migrations**
- Purpose: Embedded SQL migrations via `golang-migrate`
- Contains: `migrate.go` (Up/Down/Version/Force), `migrations/*.sql`
- Key files: `migrate.go`, `migrations/*.sql`
- Subdirectories: `migrations/` (6 migration files)

**`internal/models/`: Domain types**
- Purpose: Shared structs used across all packages
- Contains: `User`, `Domain`, `Message`, `Label`, `Attachment`, `Thread`, `Contact`, `DeliveryLog`, `AuthSession`
- Key files: `models.go`
- Subdirectories: None

**`internal/postmark/`: External API client**
- Purpose: Outbound email relay and inbound payload parsing
- Contains: `postmark.Client`, `OutboundMessage`, `InboundPayload`
- Key files: `postmark.go`
- Subdirectories: None

**`internal/redis/`: Cache and queue**
- Purpose: Redis wrapper for job queues, delayed jobs, dead-letter queue, pub/sub
- Contains: `redis.Client` with app-specific methods (`Enqueue`, `Dequeue`, `PromoteReadyDelayed`)
- Key files: `redis.go`, `redis_test.go`
- Subdirectories: None

**`internal/reputation/`: Spam filtering**
- Purpose: Whitelist/blacklist/greylist evaluation
- Contains: `reputation.Engine`
- Key files: `reputation.go`
- Subdirectories: None

**`internal/search/`: Full-text search**
- Purpose: Async PostgreSQL `tsvector` index maintenance
- Contains: `search.Indexer`
- Key files: `search.go`
- Subdirectories: None

**`internal/smtp/`: SMTP server**
- Purpose: SMTP submission proxy using `go-smtp`
- Contains: `smtp.Server` wrapper, `smtpBackend`, `smtpSession` with MIME parsing
- Key files: `smtp.go`, `smtp_test.go`
- Subdirectories: None

**`internal/webmail/`: REST API**
- Purpose: Webmail JSON API for inbox, compose, labels, threads, drafts
- Contains: `webmail.Handler` with chi route handlers
- Key files: `webmail.go`, `webmail_test.go`
- Subdirectories: None

**`internal/webhook/`: Webhook receiver**
- Purpose: Receive and validate Postmark webhooks, enqueue jobs
- Contains: `webhook.Handler`
- Key files: `webhook.go`, `webhook_test.go`
- Subdirectories: None

**`internal/workers/`: Background jobs**
- Purpose: Worker pool orchestration and job processors
- Contains: `workers.Pool`, `Job` struct, `Processor` interface, inbound/bounce/delivery/spam/send processors
- Key files: `workers.go`, `inbound.go`, `bounce.go`, `delivery.go`, `spam.go`, `send.go`, `workers_test.go`
- Subdirectories: None

**`design/`: Design documents**
- Purpose: Requirements, architecture, protocol, component, database, and API specifications
- Key files: `ARCHITECTURE.md`, `COMPONENT-DESIGN.md`, `DATABASE-SCHEMA.md`, `API-SPEC.md`, etc.
- Subdirectories: None

**`scripts/`: Deployment scripts**
- Purpose: Docker and systemd installation helpers
- Key files: `install-docker.sh`, `install-systemd.sh`
- Subdirectories: None

## Key File Locations

**Entry Points:**
- `cmd/server/main.go` - Main server process (HTTP + IMAP + SMTP + DAV)
- `cmd/worker/main.go` - Background worker process
- `cmd/migrate/main.go` - Database migration CLI

**Configuration:**
- `internal/config/config.go` - Config struct with env var tags and defaults
- `internal/config/loader.go` - TOML + env override loader
- `.env.example` - Example environment variables
- `docker-compose.yml` - Local development services

**Core Logic:**
- `internal/mailstore/pgstore.go` - Primary mail persistence (19.5KB, most complex file)
- `internal/auth/auth.go` - Authentication and session management
- `internal/api/middleware.go` - HTTP middleware stack
- `internal/smtp/smtp.go` - SMTP submission handling

**Testing:**
- `*_test.go` files alongside source in each package
- `internal/workers/workers_test.go` - Worker pool tests
- `internal/webmail/webmail_test.go` - API handler tests

**Documentation:**
- `README.md` - Project overview
- `design/*.md` - Detailed design specifications
- `docs/design/tls-letsencrypt.md` - TLS operational guide
- `INTEGRATION.md` - Integration guide

## Naming Conventions

**Files:**
- `*.go` - Go source files (kebab-case not used; all lowercase)
- `*_test.go` - Test files adjacent to source
- `*.md` - Markdown documentation
- `*.sql` - Database migrations
- `Dockerfile.*` - Container image definitions

**Directories:**
- `cmd/` - Singular for entry points
- `internal/` - Standard Go private package root
- Package names are lowercase, single-word when possible (`mailstore`, `webmail`, `webhook`)

**Go Identifiers:**
- Exported constructors: `New`, `NewService`, `NewHandler`, `NewPool`
- Interface names: `Store`, `DomainLister`, `Processor`
- Concrete implementations: `PGStore`, `Handler`, `Pool`
- Context keys: unexported `ctxKey` type with string constants

**Special Patterns:**
- `main.go` in each `cmd/` subdirectory for binary entry points
- `embed` tag on `migrationsFS` in `internal/migrate/migrate.go` for embedded SQL
- `RegisterRoutes(r chi.Router)` convention for HTTP handlers

## Where to Add New Code

**New Protocol Server:**
- Implementation: `internal/<protocol>/<protocol>.go`
- Wire into: `cmd/server/main.go`
- Tests: `internal/<protocol>/<protocol>_test.go`

**New HTTP Handler/Domain:**
- Implementation: `internal/<domain>/<domain>.go`
- Route registration: Call `RegisterRoutes` in `cmd/server/main.go`
- Tests: `internal/<domain>/<domain>_test.go`

**New Background Worker:**
- Processor: `internal/workers/<jobtype>.go`
- Registration: `cmd/worker/main.go` (`pool.Register`)
- Tests: `internal/workers/workers_test.go` or new test file

**New Database Migration:**
- SQL file: `internal/migrate/migrations/NNN_<name>.up.sql`
- Down file: `internal/migrate/migrations/NNN_<name>.down.sql`
- Run: `cmd/migrate/main.go up`

**New Model/Domain Type:**
- Add to: `internal/models/models.go`

**Utilities:**
- Shared helpers: Add to relevant `internal/` package or create new package
- HTTP-specific: `internal/api/`

## Special Directories

**`internal/migrate/migrations/`: Embedded SQL migrations**
- Purpose: Database schema version control
- Source: Manually authored SQL, consumed by `golang-migrate` via `embed.FS`
- Committed: Yes

**`.gograph/`: Code analysis artifacts**
- Purpose: Generated dependency graphs, reports, and analysis files
- Source: Generated by external tooling
- Committed: No (in `.gitignore`)

**`.serena/`, `.pi/`, `.agents/`, `.remember/`: Agent tooling**
- Purpose: Claude Code / agent session metadata
- Source: Auto-generated by agent tooling
- Committed: Partially (`.serena/project.yml`); caches ignored

---

*Structure analysis: 2025-06-10*
*Update when directory structure changes*
