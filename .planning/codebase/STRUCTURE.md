# Codebase Structure

**Analysis Date:** 2026-05-18

## Directory Layout

```
[project-root]/
├── cmd/                     # Compiled entry-point binaries
│   ├── admin/               # CLI admin tool
│   ├── migrate/             # Database migration runner
│   ├── server/              # Main API + IMAP + SMTP + DAV server
│   ├── webui/               # SPA static server + API proxy
│   └── worker/              # Background job worker pool
├── internal/                # Application code (Go standard project layout)
│   ├── admin/               # Admin REST API + PG store
│   ├── api/                 # Shared HTTP middleware, auth handler, errors, CSRF
│   ├── auth/                # Authentication service (passwords, sessions, domains)
│   ├── calendar/            # Calendar REST API + PG store + iCal helpers
│   ├── certmanager/         # ACME TLS certificate lifecycle
│   ├── config/              # TOML/env configuration loading
│   ├── contacts/            # Contacts REST API + PG store
│   ├── dav/                 # CardDAV/CalDAV handler
│   ├── db/                  # pgx pool wrapper
│   ├── imap/                # IMAP4rev1 backend
│   ├── logger/              # Structured JSON logger
│   ├── mailstore/           # Mail persistence interface + PG store
│   ├── migrate/             # Migration runner wrapper + SQL files
│   │   └── migrations/      # golang-migrate up SQL
│   ├── models/              # Canonical domain structs
│   ├── postmark/            # Postmark client wrapper + inbound parsing
│   ├── redis/               # Redis wrapper + queue helpers
│   ├── reputation/          # Whitelist/blacklist/greylist engine
│   ├── search/              # PostgreSQL tsvector indexer
│   ├── smtp/                # SMTP proxy server
│   ├── webhook/             # Postmark webhook receiver
│   ├── webmail/             # Webmail REST API (messages, drafts, labels, threads)
│   ├── webui/               # Web UI binary helpers (proxy, SSE, router, config)
│   │   └── dist/            # Embedded React build output
│   └── workers/             # Worker pool + job processors
├── web/                     # React frontend source
│   └── src/
│       ├── components/      # React components (SPA pages)
│       ├── styles/          # TailwindCSS entry
│       ├── test/            # Vitest tests
│       ├── api.js           # Axios HTTP client + API functions
│       ├── sse.js           # SSE reconnect client
│       ├── App.jsx          # Router and auth gate
│       └── main.jsx         # React root entry
├── docs/                    # Project documentation
├── design/                  # Design artifacts
├── scripts/                 # Utility scripts
├── docker-compose.yml       # Local dev stack (Postgres, Redis)
├── Makefile               # Build shortcuts
├── go.mod                  # Go module definition
├── flake.nix               # Nix development shell
└── .planning/              # GSD planning documents
    └── codebase/            # Codebase analysis outputs
```

## Directory Purposes

**`cmd/`: Entry Point Binaries**
- Purpose: Each subdirectory compiles to a standalone binary.
- Contains: Only `main.go` files.
- Key files:
  - `cmd/server/main.go`: Orchestrates all services and protocol servers.
  - `cmd/webui/main.go`: Serves the embedded React SPA.
  - `cmd/worker/main.go`: Starts background job consumers.
  - `cmd/admin/main.go`: CLI for user/domain management.
  - `cmd/migrate/main.go`: Runs database migrations.

**`internal/models/`: Domain Models**
- Purpose: Single source of truth for data structures.
- Contains: `models.go` with all domain structs (`User`, `Message`, `Label`, `Contact`, `CalendarEvent`, etc.).
- Key files: `internal/models/models.go`

**`internal/config/`: Configuration**
- Purpose: Load TOML config and overlay environment variables with legacy fallback names.
- Contains: `config.go` (Config struct), `loader.go` (TOML + reflect-based env override), `template.go`.
- Key files: `internal/config/loader.go`

**`internal/api/`: Shared HTTP Concerns**
- Purpose: Reusable middleware, error types, and the public auth HTTP handler.
- Contains: `middleware.go`, `auth.go`, `errors.go`, `csrf.go`, plus test files.
- Key files:
  - `internal/api/middleware.go`: RequestID, logger, recovery, CORS, rate limiter, session/auth middleware.
  - `internal/api/errors.go`: Structured error types and JSON writer.
  - `internal/api/auth.go`: Login, logout, me endpoints.

**`internal/mailstore/`: Mail Persistence**
- Purpose: Interface and PostgreSQL implementation for messages, labels, threads, attachments.
- Contains: `mailstore.go` (interface + options/patch types), `pgstore.go` (SQL implementation).
- Key files:
  - `internal/mailstore/mailstore.go`: `Store` interface.
  - `internal/mailstore/pgstore.go`: `PGStore` with explicit SQL.

**`internal/webmail/`: Webmail API**
- Purpose: REST endpoints for the Gmail-like web UI.
- Contains: `webmail.go` (handler with all routes), `dto.go`, `dto_test.go`, `webmail_test.go`.
- Key files: `internal/webmail/webmail.go`

**`internal/workers/`: Background Jobs**
- Purpose: Redis-backed queue system and job processors.
- Contains: `workers.go` (pool), `inbound.go`, `send.go`, `bounce.go`, `delivery.go`, `spam.go`, plus tests.
- Key files:
  - `internal/workers/workers.go`: `Pool`, `Job`, `Processor` interface.
  - `internal/workers/inbound.go`: `InboundProcessor`.
  - `internal/workers/send.go`: `SendProcessor`.

**`internal/imap/`: IMAP Server**
- Purpose: IMAP4rev1 protocol gateway.
- Contains: `backend.go` (go-imap backend implementation), `imap.go`.
- Key files: `internal/imap/backend.go`

**`internal/smtp/`: SMTP Server**
- Purpose: SMTP submission proxy with AUTH.
- Contains: `smtp.go` (server + backend + session), `smtp_test.go`.
- Key files: `internal/smtp/smtp.go`

**`internal/dav/`: DAV Protocols**
- Purpose: CardDAV and CalDAV endpoints.
- Contains: `dav.go` (handler + both backends).
- Key files: `internal/dav/dav.go`

**`internal/webui/`: Web UI Server Helpers**
- Purpose: Proxy, SSE hub, router, embedded SPA serving.
- Contains: `router.go` (Gin router), `proxy.go` (reverse proxy), `sse.go` (SSE hub), `config.go`.
- Key files:
  - `internal/webui/router.go`: Static file serving + proxy rules.
  - `internal/webui/proxy.go`: `httputil.ReverseProxy` to backend.
  - `internal/webui/sse.go`: Redis-backed SSE hub.

**`web/src/`: React Frontend**
- Purpose: Gmail-inspired SPA.
- Contains: Components, API client, SSE client, styles, tests.
- Key files:
  - `web/src/App.jsx`: Routes and auth state.
  - `web/src/api.js`: Axios instance with CSRF interceptor.
  - `web/src/sse.js`: Reconnecting SSE client.
  - `web/src/components/Layout.jsx`: App shell.
  - `web/src/components/Inbox.jsx`: Message list.
  - `web/src/components/Compose.jsx`: Draft editor.
  - `web/src/components/Admin.jsx`: Admin dashboard.

## Key File Locations

**Entry Points:**
- `cmd/server/main.go`: Main server (HTTP, IMAP, SMTP, DAV)
- `cmd/webui/main.go`: Web UI server
- `cmd/worker/main.go`: Worker pool
- `cmd/admin/main.go`: Admin CLI
- `cmd/migrate/main.go`: Migration runner

**Configuration:**
- `internal/config/config.go`: Config struct definition
- `internal/config/loader.go`: TOML + env loading logic
- `.env.example`: Example environment variables

**Core Logic:**
- `internal/models/models.go`: All domain structs
- `internal/auth/auth.go`: Auth service (sessions, passwords, domains)
- `internal/mailstore/mailstore.go`: Mail store interface
- `internal/mailstore/pgstore.go`: Mail store PostgreSQL implementation
- `internal/api/middleware.go`: HTTP middleware stack
- `internal/webmail/webmail.go`: Webmail REST handler
- `internal/workers/workers.go`: Worker pool and queue logic

**Protocol Implementations:**
- `internal/imap/backend.go`: IMAP backend
- `internal/smtp/smtp.go`: SMTP server
- `internal/dav/dav.go`: DAV handler

**Frontend:**
- `web/src/main.jsx`: React bootstrap
- `web/src/App.jsx`: Routing
- `web/src/api.js`: Backend API client
- `web/vite.config.js`: Build config (outputs to `internal/webui/dist`)

**Testing:**
- `web/src/test/`: Frontend Vitest tests
- `internal/api/*_test.go`: Go middleware tests
- `internal/webmail/webmail_test.go`: Webmail handler tests
- `internal/smtp/smtp_test.go`: SMTP tests
- `internal/workers/workers_test.go`: Worker tests

## Naming Conventions

**Files:**
- Go source: `snake_case.go` for implementation, `*_test.go` for tests.
- Frontend: `PascalCase.jsx` for React components, `camelCase.js` for utilities.

**Directories:**
- Go packages: lowercase, singular noun (`mailstore`, `calendar`, `auth`).
- Frontend: lowercase plural for groups (`components/`, `styles/`, `test/`).

**Types:**
- Interfaces: noun describing the capability (`Store`, `DomainLister`, `Processor`).
- Implementations: `PGStore` for PostgreSQL-backed stores.
- Handlers: `{Domain}Handler` (e.g., `AuthHandler`, `webmail.Handler`).

## Where to Add New Code

**New Feature (backend):**
- Domain model additions: `internal/models/models.go`
- New persistence layer: create package under `internal/` with `Store` interface + `PGStore`.
- New REST endpoints: create `handler.go` in the same package, register in `cmd/server/main.go`.
- New background job: add `Processor` in `internal/workers/`, register in `cmd/worker/main.go`.

**New Feature (frontend):**
- New page: `web/src/components/{Name}.jsx`
- New API calls: `web/src/api.js`
- Route registration: `web/src/App.jsx`

**Utilities:**
- Shared helpers: place in the most specific existing package (e.g., `internal/api/` for HTTP helpers, `internal/models/` for shared types).

## Special Directories

**`internal/webui/dist/`: Embedded SPA**
- Purpose: Build output from `web/` Vite build, embedded via `//go:embed all:dist`.
- Generated: Yes (by `vite build` or `make build-web`).
- Committed: Yes (tracked in git so the Go binary builds without Node).

**`internal/migrate/migrations/`: Database Migrations**
- Purpose: golang-migrate SQL up migrations.
- Generated: No.
- Committed: Yes.

**`.planning/codebase/`: GSD Analysis Outputs**
- Purpose: Consumed by `/gsd:plan-phase` and `/gsd:execute-phase`.
- Generated: Yes (by codebase mapper).
- Committed: Yes.

---

*Structure analysis: 2026-05-18*
