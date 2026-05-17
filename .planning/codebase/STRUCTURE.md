# Postnest Directory Structure

## Root Layout

```
.
├── cmd/                    # Application entry points (one per binary)
├── internal/               # Private application code
├── web/                    # React 19 frontend source
├── scripts/                # Build and deployment scripts
├── design/                 # Design documents and assets
├── docs/                   # Project documentation
├── .planning/              # Planning artifacts (this directory)
├── .gograph/               # Dependency graph artifacts
├── .gstack/                # gstack harness configuration
├── .serena/                # Serena agent configuration
├── nix/                    # Nix flake support files
├── go.mod                  # Go module definition (github.com/go-postnest/postnest)
├── go.sum                  # Go dependency checksums
├── Makefile                # Build automation
├── docker-compose.yml      # Local development stack
├── Dockerfile.server         # Server binary container
├── Dockerfile.webui        # Web UI binary container
├── Dockerfile.worker       # Worker binary container
├── Dockerfile.migrate      # Migration binary container
├── flake.nix               # Nix flake for reproducible builds
├── README.md               # Project overview
├── AGENTS.md               # Agent guidelines
├── INTEGRATION.md          # Integration documentation
├── PLAN.md                 # Development plan
├── .env.example            # Example environment variables
└── .env                    # Local environment (gitignored)
```

---

## `cmd/` — Entry Points

Each subdirectory is a standalone binary:

```
cmd/
├── admin/main.go     # CLI administration tool (create-user, create-domain, add-member, setup)
├── migrate/main.go   # Database migration runner (up, down, version, force)
├── server/main.go   # Primary server: HTTP API + IMAP + SMTP + DAV
├── webui/main.go    # Static SPA server + API reverse proxy + SSE
└── worker/main.go   # Background job processor pool
```

**Naming convention**: Binary names match directory names exactly (`server`, `worker`, `webui`, `admin`, `migrate`). Build outputs are prefixed with `postnest-` in the Makefile.

---

## `internal/` — Application Code

### Core Infrastructure

| Path | Purpose |
|------|---------|
| `internal/config/` | Configuration loading: TOML file + env var overrides with legacy fallback support |
| `internal/db/` | PostgreSQL connection pool wrapper (`pgxpool.Pool`) |
| `internal/redis/` | Redis client wrapper with app-specific queue helpers (`Enqueue`, `Dequeue`, `PromoteReadyDelayed`) |
| `internal/logger/` | Structured JSON logger (`slog.NewJSONHandler`) |

### Domain Models

| Path | Purpose |
|------|---------|
| `internal/models/models.go` | Canonical entity structs: `User`, `Domain`, `Message`, `Thread`, `Label`, `Attachment`, `Contact`, `DeliveryLog`, `AuthSession` |

### Authentication & Authorization

| Path | Purpose |
|------|---------|
| `internal/auth/auth.go` | `auth.Service` — Argon2id hashing, session/API-key validation, domain membership queries |

### Persistence Stores

| Path | Purpose |
|------|---------|
| `internal/mailstore/mailstore.go` | `Store` interface — the canonical contract for mail persistence |
| `internal/mailstore/pgstore.go` | `PGStore` — PostgreSQL implementation of `Store` |
| `internal/contacts/contacts.go` | `Store` interface + `PGStore` for contacts |

### Protocol Servers

| Path | Purpose |
|------|---------|
| `internal/api/` | Shared HTTP middleware: request ID, logging, recovery, CORS, rate limiting, session extraction, error types |
| `internal/webmail/webmail.go` | REST API handler for labels, messages, threads, drafts, search (`chi/v5`) |
| `internal/webhook/webhook.go` | Postmark webhook receiver with HMAC verification and deduplication |
| `internal/smtp/smtp.go` | SMTP server (`go-smtp`) with PLAIN/LOGIN auth, MIME parsing, Postmark relay |
| `internal/imap/imap.go` | IMAP server wrapper (`go-imap`) |
| `internal/imap/backend.go` | IMAP backend: labels-as-mailboxes, flag mapping, message fetch/search/expunge |
| `internal/dav/dav.go` | CardDAV (contacts) and CalDAV (stub) via `go-webdav` |

### Workers

| Path | Purpose |
|------|---------|
| `internal/workers/workers.go` | `Pool`, `Job`, and `Processor` interface; Redis queue orchestration |
| `internal/workers/inbound.go` | Postmark inbound webhook processor |
| `internal/workers/send.go` | Draft send processor ( relays to Postmark) |
| `internal/workers/bounce.go` | Bounce event processor |
| `internal/workers/delivery.go` | Delivery confirmation processor |
| `internal/workers/spam.go` | Spam complaint processor |

### Supporting Services

| Path | Purpose |
|------|---------|
| `internal/postmark/postmark.go` | Thin wrapper around `mrz1836/postmark` library; inbound payload parsing |
| `internal/search/search.go` | PostgreSQL full-text search indexer (`tsvector` updates) |
| `internal/reputation/reputation.go` | Whitelist/blacklist/greylist engine |
| `internal/certmanager/manager.go` | ACME certificate lifecycle (LEGO + Cloudflare DNS-01) |

### Web UI Server

| Path | Purpose |
|------|---------|
| `internal/webui/router.go` | Gin router: SSE hub, API proxy, embedded SPA static file serving |
| `internal/webui/proxy.go` | `httputil.ReverseProxy` to backend API server |
| `internal/webui/sse.go` | SSE hub with Redis pub/sub subscription and client broadcast |
| `internal/webui/config.go` | `Config` struct for webui server |
| `internal/webui/dist/` | **Generated** — Vite build output embedded into Go binary |

### Migrations

| Path | Purpose |
|------|---------|
| `internal/migrate/migrate.go` | Wrapper around `golang-migrate/migrate/v4` using embedded `iofs` source |
| `internal/migrate/migrations/` | SQL migration files (up-only in current set) |
| `internal/migrate/migrations/000001_init.up.sql` | Initial schema: domains, users, messages, labels, threads, contacts, reputation, delivery logs |
| `internal/migrate/migrations/000002_fts.up.sql` | Full-text search vector setup |
| `internal/migrate/migrations/000003_seed_labels.up.sql` | System label seeding |
| `internal/migrate/migrations/000004_fts_trigger.up.sql` | Search vector trigger |
| `internal/migrate/migrations/000005_search_composite.up.sql` | Search performance indexes |
| `internal/migrate/migrations/000006_thread_unique.up.sql` | Thread uniqueness constraint |

---

## `web/` — Frontend Source

```
web/
├── index.html              # HTML entry point
├── package.json            # npm dependencies (React 19, Vite, TailwindCSS, Axios, React Router)
├── vite.config.js          # Vite config: React plugin, output to ../internal/webui/dist
├── tailwind.config.js      # Tailwind theme with custom primary/surface color scales
├── postcss.config.js       # PostCSS with Tailwind + Autoprefixer
├── src/
│   ├── main.jsx            # React DOM root mount
│   ├── App.jsx             # Root component: auth gate, routing, SSE lifecycle
│   ├── api.js              # Axios client + API wrappers (labels, messages, drafts, contacts, calendar)
│   ├── sse.js              # SSE client with auto-reconnect and event emitter
│   ├── components/
│   │   ├── Layout.jsx      # Shell layout with navigation
│   │   ├── Login.jsx       # Authentication form
│   │   ├── Inbox.jsx       # Message list view
│   │   ├── MessageView.jsx # Single message reading pane
│   │   ├── Compose.jsx     # Draft composition
│   │   ├── Contacts.jsx    # Address book
│   │   ├── Calendar.jsx    # Calendar view
│   │   └── Admin.jsx       # Domain administration
│   └── styles/
│       └── index.css       # Tailwind directives + global styles
```

**Build integration**: `npm run build` in `web/` outputs to `internal/webui/dist`, which is embedded into the `webui` binary.

---

## Key File Locations

### Configuration
- `internal/config/loader.go` — TOML parsing and `POSTNEST_<SECTION>_<KEY>` env override logic
- `internal/config/config.go` — Flat `Config` struct with legacy env fallback mapping

### Database Schema
- `internal/migrate/migrations/000001_init.up.sql` — Complete initial schema

### Error Handling
- `internal/api/errors.go` — Unified `AppError` and `WriteError`

### Middleware
- `internal/api/middleware.go` — `RequestID`, `StructuredLogger`, `Recovery`, `CORS`, `RateLimiter`, `RequireSession`, `RequireDomainAdmin`

### IMAP Backend Detail
- `internal/imap/backend.go` — All IMAP command implementations (LOGIN, LIST, SELECT, FETCH, SEARCH, STORE, EXPUNGE, COPY)

---

## Naming Conventions

### Go

| Pattern | Example |
|---------|---------|
| **Packages** | Single word, lowercase: `mailstore`, `certmanager`, `webmail` |
| **Interfaces** | Noun describing capability: `Store`, `Processor`, `DomainLister` |
| **Implementations** | Prefix with storage/driver: `PGStore`, `InboundProcessor` |
| **Constructors** | `New` + type name: `NewPool`, `NewHandler`, `NewServer` |
| **Handler methods** | HTTP verb or action prefix: `listLabels`, `createDraft`, `patchMessage` |
| **Context keys** | Unexported typed string constants: `ctxKeyUser`, `ctxKeyRequestID` |
| **Error variables** | `Err` prefix: `ErrNotFound`, `ErrUnauthorized`, `ErrRateLimited` |
| **Binary names** | Match `cmd/` directory: `server`, `worker`, `webui`, `admin`, `migrate` |

### React

| Pattern | Example |
|---------|---------|
| **Components** | PascalCase, noun: `Inbox.jsx`, `MessageView.jsx`, `Layout.jsx` |
| **API functions** | camelCase, verb prefix: `getLabels`, `createDraft`, `sendDraft` |
| **Client modules** | lowercase: `api.js`, `sse.js` |

### SQL / Database

| Pattern | Example |
|---------|---------|
| **Tables** | Plural, snake_case: `messages`, `domain_members`, `delivery_logs` |
| **Indexes** | Descriptive suffix: `messages_domain_user_mailbox_idx`, `auth_sessions_token_hash_idx` |
| **Constraints** | Inline or via migration; `ON DELETE CASCADE` for domain-scoped data |
| **UUID generation** | `gen_random_uuid()` (pgcrypto) or `uuid.NewV7()` in code |

---

## Module Organization

### Import Graph (Simplified)

```
cmd/server
  ├─ internal/api
  ├─ internal/auth
  ├─ internal/certmanager
  ├─ internal/config
  ├─ internal/contacts
  ├─ internal/dav
  ├─ internal/db
  ├─ internal/imap
  ├─ internal/logger
  ├─ internal/mailstore
  ├─ internal/postmark
  ├─ internal/redis
  ├─ internal/smtp
  ├─ internal/webhook
  └─ internal/webmail

cmd/worker
  ├─ internal/auth
  ├─ internal/config
  ├─ internal/db
  ├─ internal/logger
  ├─ internal/mailstore
  ├─ internal/postmark
  ├─ internal/redis
  ├─ internal/reputation
  └─ internal/workers

cmd/webui
  └─ internal/webui
      ├─ internal/logger
      └─ internal/redis (via SSE)

cmd/admin
  ├─ internal/auth
  ├─ internal/config
  ├─ internal/logger
  └─ internal/models

cmd/migrate
  ├─ internal/config
  └─ internal/migrate
```

### Dependency Rules

1. **`models`** has zero internal dependencies. It is imported by almost every other package.
2. **`api`** is a leaf package imported by handlers; it does not import domain packages except `auth` and `models`.
3. **`mailstore`** and **`contacts`** are the only packages that import `db` (indirectly via `pgxpool.Pool`).
4. **`webui`** is a leaf server package; it only proxies to the backend and serves static assets.
5. **`workers`** processors import `mailstore`, `auth`, `postmark`, `reputation`, but not each other.
6. **`config`** is imported by all `cmd/*` packages. It has no internal dependencies.
