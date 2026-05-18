# Codebase Structure

**Analysis Date:** 2026-05-18

## Directory Layout

```
[project-root]/
├── cmd/                    # Entry point binaries
│   ├── admin/              # CLI admin tool
│   ├── migrate/            # Database migration CLI
│   ├── server/             # HTTP + IMAP + SMTP server
│   ├── webui/              # React SPA proxy server
│   └── worker/             # Background job worker
├── internal/               # Private application packages
│   ├── api/                # HTTP middleware, errors, auth handler
│   ├── auth/               # Authentication & session service
│   ├── calendar/           # Calendar REST API + PGStore + iCal
│   ├── certmanager/        # ACME TLS certificate manager
│   ├── config/             # Environment configuration loading
│   ├── contacts/           # Contacts store & REST handler
│   ├── dav/                # CardDAV/CalDAV protocol adapters
│   ├── db/                 # PostgreSQL connection pool wrapper
│   ├── imap/               # IMAP server backend
│   ├── logger/             # Structured JSON logger
│   ├── mailstore/          # Mail persistence interface + PGStore
│   ├── migrate/            # Migration runner + SQL files
│   ├── models/             # Shared domain models
│   ├── postmark/           # Postmark API client wrapper
│   ├── redis/              # Redis client + queue helpers
│   ├── reputation/         # Email reputation engine
│   ├── search/             # Search helpers
│   ├── smtp/               # SMTP server backend
│   ├── webhook/            # Postmark webhook receiver
│   ├── webmail/            # Webmail REST API + DTOs
│   ├── webui/              # SPA proxy, SSE hub, embedded dist
│   └── workers/            # Worker pool + job processors
├── web/                    # React SPA frontend (Vite)
│   ├── src/
│   │   ├── components/     # React components (one per major view)
│   │   ├── styles/         # Tailwind + global CSS
│   │   └── test/           # Vitest tests
│   ├── package.json
│   ├── vite.config.js
│   └── tailwind.config.js
├── scripts/                # Build/deployment scripts
├── docs/                   # Design documentation
├── design/                 # Design assets
├── nix/                    # Nix flake support
├── docker-compose.yml     # Local dev stack
├── Makefile               # Build automation
├── go.mod                  # Go module definition
└── .env.example            # Environment variable template
```

## Directory Purposes

**cmd:**
- Purpose: Standalone executable entry points
- Contains: `main.go` files, each producing a single binary
- Key files: `cmd/server/main.go`, `cmd/worker/main.go`, `cmd/webui/main.go`

**internal:**
- Purpose: All application code (Go convention: non-importable by external modules)
- Contains: ~20 domain packages, each self-contained
- Key files: `internal/models/models.go` (shared entities), `internal/api/errors.go` (error contract)

**internal/api:**
- Purpose: Cross-cutting HTTP concerns
- Contains: Middleware (request ID, logging, recovery, CORS, rate limiting, auth, CSRF), auth handler, error types
- Key files: `internal/api/middleware.go`, `internal/api/errors.go`, `internal/api/auth.go`, `internal/api/csrf.go`

**internal/mailstore:**
- Purpose: Mail persistence abstraction and implementation
- Contains: `mailstore.go` (interface ~25 methods), `pgstore.go` (PostgreSQL implementation)
- Key files: `internal/mailstore/mailstore.go`, `internal/mailstore/pgstore.go`

**internal/workers:**
- Purpose: Background job processing
- Contains: Pool implementation, processor interface, 5 concrete processors
- Key files: `internal/workers/workers.go`, `internal/workers/inbound.go`, `internal/workers/send.go`

**internal/webui:**
- Purpose: Serve built React SPA and proxy API requests
- Contains: Gin router, reverse proxy, SSE hub, embedded `dist/` filesystem
- Key files: `internal/webui/router.go`, `internal/webui/proxy.go`, `internal/webui/sse.go`

**web:**
- Purpose: Frontend React application
- Contains: JSX components, API client, styles, tests
- Key files: `web/src/App.jsx`, `web/src/api.js`, `web/src/main.jsx`

**internal/migrate/migrations:**
- Purpose: Database schema evolution
- Contains: Up-migration SQL files with sequential numbering
- Key files: `000001_init.up.sql`, `000007_calendar.up.sql`

## Key File Locations

**Entry Points:**
- `cmd/server/main.go`: Main application server
- `cmd/worker/main.go`: Background worker
- `cmd/webui/main.go`: Frontend proxy
- `cmd/admin/main.go`: Admin CLI
- `cmd/migrate/main.go`: Migration CLI

**Configuration:**
- `internal/config/config.go`: `Config` struct with env var tags
- `internal/config/loader.go`: `Load()` implementation
- `.env.example`: Template for required environment variables

**Core Logic:**
- `internal/models/models.go`: All domain entities
- `internal/auth/auth.go`: Password hashing, sessions, domain authorization
- `internal/mailstore/mailstore.go`: Store interface contract
- `internal/mailstore/pgstore.go`: PostgreSQL implementation

**API Handlers:**
- `internal/webmail/webmail.go`: Webmail REST routes (labels, messages, drafts, attachments, search)
- `internal/calendar/handler.go`: Calendar REST routes
- `internal/contacts/handler.go`: Contacts REST routes
- `internal/webhook/webhook.go`: Postmark inbound/bounce/delivery/spam webhooks
- `internal/dav/dav.go`: CardDAV/CalDAV protocol handlers

**Protocol Servers:**
- `internal/imap/imap.go`: IMAP backend + server wrapper
- `internal/smtp/smtp.go`: SMTP backend + server wrapper

**Testing:**
- `internal/api/*_test.go`: Middleware/error tests
- `internal/auth/auth_test.go`: Auth service tests
- `internal/calendar/ical_test.go`: iCal conversion tests
- `internal/redis/redis_test.go`: Redis client tests
- `internal/smtp/smtp_test.go`: SMTP tests
- `internal/webhook/webhook_test.go`: Webhook tests
- `internal/webmail/dto_test.go`, `webmail_test.go`: DTO + handler tests
- `internal/workers/workers_test.go`: Worker pool tests
- `internal/config/loader_test.go`: Config loader tests
- `web/src/test/*.test.js`, `*.test.jsx`: Frontend Vitest tests

## Naming Conventions

**Files:**
- Go files: lowercase with underscore for multi-word (`pgstore.go`, `middleware_test.go`)
- Frontend components: PascalCase matching component name (`Inbox.jsx`, `Compose.jsx`)
- API client: lowercase (`api.js`, `sse.js`)
- Styles: lowercase (`index.css`)

**Directories:**
- Go packages: lowercase single word (`mailstore`, `webmail`, `certmanager`)
- Frontend components directory: lowercase plural (`components/`)

**Types:**
- Interfaces: noun describing capability (`Store`, `DomainLister`, `Processor`)
- Implementations: package prefix + interface name (`PGStore`, `AuthHandler`)
- DTOs: lowercase with `DTO` suffix in Go (`messageDTO`, `calendarDTO`); frontend uses plain objects
- Context keys: unexported typed string constants (`ctxKeyUser`, `ctxKeyDomainID`)

**Functions:**
- Constructors: `New` + type name (`NewHandler`, `NewPool`, `NewPGStore`)
- Route registration: `RegisterRoutes` on handler types
- Context extraction: `XFromContext` pattern (`UserFromContext`, `DomainIDFromContext`)

## Where to Add New Code

**New REST API Feature:**
- Handler: `internal/<feature>/handler.go`
- Store interface: `internal/<feature>/<feature>.go`
- PGStore implementation: `internal/<feature>/pgstore.go`
- DTOs: `internal/<feature>/dto.go`
- Register routes in `cmd/server/main.go` inside authenticated group
- Wire store instantiation in `cmd/server/main.go`

**New Worker Job Type:**
- Processor: `internal/workers/<type>.go` implementing `Processor` interface
- Register in `cmd/worker/main.go`: `pool.Register("<type>", workers.NewXProcessor(...))`
- Enqueue from handlers via `h.redis.Enqueue(ctx, "queue:jobs", jobBytes)` or `pool.Enqueue`

**New Frontend Page:**
- Component: `web/src/components/<Name>.jsx`
- Route: add to `web/src/App.jsx` inside `<Routes>`
- API client functions: add to `web/src/api.js`

**New Database Table:**
- Migration: `internal/migrate/migrations/000XXX_<name>.up.sql`
- Model: add struct to `internal/models/models.go`
- Store methods: add to relevant store interface + PGStore

**New Middleware:**
- Add to `internal/api/middleware.go`
- Register in `cmd/server/main.go` on the chi router
- Add tests in `internal/api/middleware_test.go`

**Utilities:**
- Shared helpers: add to the package where used, or `internal/api/` if HTTP-specific
- Cross-package helpers: consider `internal/utils/` (does not exist currently; add if justified)

## Special Directories

**internal/webui/dist:**
- Purpose: Embedded build output of the React SPA
- Generated: Yes (via `vite build` in `web/`)
- Committed: Yes (currently tracked; embed directive requires files at build time)

**internal/migrate/migrations:**
- Purpose: SQL schema migrations
- Generated: No (handwritten)
- Committed: Yes

**.planning:**
- Purpose: GSD planning documents, codebase analysis, phase plans
- Generated: Yes (by agent workflows)
- Committed: Yes

**web/node_modules:**
- Purpose: Frontend dependencies
- Generated: Yes (by npm)
- Committed: No (in `.gitignore`)

---

*Structure analysis: 2026-05-18*
