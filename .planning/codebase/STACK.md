# Technology Stack

**Analysis Date:** 2026-05-18

## Languages

**Primary:**
- Go 1.25.0 — Backend services, protocol implementations, workers, migration CLI
- JavaScript (JSX/ES modules) — Frontend SPA (`web/src/**/*.jsx`)
- SQL — PostgreSQL schema migrations (`internal/migrate/migrations/*.sql`)

**Secondary:**
- TOML — Server configuration file format (`internal/config/loader.go`)
- Nix — Development shell and build definitions (`flake.nix`, `nix/module.nix`)
- HTML/CSS — Embedded web UI shell (`web/index.html`, Tailwind utility classes)

## Runtime

**Environment:**
- Go 1.25 (compiled with `CGO_ENABLED=0`, static Linux binary)
- Node.js 22 — Used only at build time for the frontend SPA (multi-stage Docker)

**Package Manager:**
- Go modules — `go.mod` / `go.sum` present, lockfile committed
- npm — `web/package-lock.json` present, lockfile committed

**Deployment Images:**
- `gcr.io/distroless/static-debian12:nonroot` for all Go binaries
- `postgres:16-alpine` for local datastore
- `redis:7-alpine` for local queue/cache

## Frameworks

**Core (Backend):**
- `github.com/go-chi/chi/v5` v5.2.5 — HTTP router/middleware for REST API and webhook handlers
- `github.com/emersion/go-imap` v1.2.1 — IMAP4rev1 server implementation
- `github.com/emersion/go-smtp` v0.24.0 — SMTP submission server with AUTH PLAIN
- `github.com/emersion/go-message` v0.18.2 — RFC822 / MIME parsing and generation
- `github.com/emersion/go-webdav` v0.7.0 — WebDAV / CardDAV / CalDAV backend
- `github.com/emersion/go-ical` v0.0.0-20250609112844-439c63cef608 — iCalendar parsing/generation
- `github.com/emersion/go-vcard` v0.0.0-20241024213814-c9703dde27ff — vCard parsing/generation
- `github.com/mrz1836/postmark` v1.9.2 — Postmark email API client
- `github.com/jackc/pgx/v5` v5.9.2 — PostgreSQL driver with connection pool (`pgxpool`)
- `github.com/redis/go-redis/v9` v9.19.0 — Redis client
- `github.com/golang-migrate/migrate/v4` v4.19.1 — Database migrations
- `github.com/go-acme/lego/v4` v4.35.2 — ACME/Let's Encrypt certificate automation
- `github.com/BurntSushi/toml` v1.6.0 — Configuration file parsing

**Core (Frontend):**
- React 19.0.0 — UI framework (`web/package.json`)
- react-router-dom 7.0.0 — Client-side routing (`web/src/main.jsx`)
- Vite 6.0.0 — Build tool and dev server (`web/vite.config.js`)
- TailwindCSS 3.4.15 — Utility-first CSS (`web/tailwind.config.js`)
- PostCSS 8.4.49 + Autoprefixer 10.4.20 — CSS pipeline (`web/postcss.config.js`)
- TipTap 3.23.4 (`@tiptap/react`, `@tiptap/starter-kit`, `@tiptap/pm`) — Rich text compose editor
- axios 1.7.0 — HTTP client with CSRF double-submit interceptor (`web/src/api.js`)
- date-fns 4.1.0 — Date formatting
- DOMPurify 3.4.4 — HTML sanitization for inbound message rendering
- lucide-react 0.460.0 — Icon set

**Testing:**
- Go standard `testing` package — All backend unit tests (no third-party assertion library detected)
- Vitest 4.1.6 — Frontend test runner (`web/vite.config.js` test block)
- @testing-library/react 16.3.2 — React component tests
- @testing-library/jest-dom 6.9.1 — Custom DOM matchers
- jsdom 29.1.1 — Browser environment for Vitest
- msw 2.14.6 — Mock Service Worker for API mocking in frontend tests
- miniredis/v2 2.38.0 — In-memory Redis for Go tests (`internal/redis/redis_test.go`)

**Build/Dev:**
- `make` — Build orchestration (`Makefile`)
- Docker + Docker Compose — Local orchestration (`docker-compose.yml`, `Dockerfile.server`, `Dockerfile.webui`, `Dockerfile.worker`, `Dockerfile.migrate`)
- Nix flake — Reproducible dev shell and NixOS module (`flake.nix`)

## Key Dependencies

**Critical:**
- `github.com/mrz1836/postmark` v1.9.2 — Sole email transport provider for outbound sending and inbound webhook parsing
- `github.com/jackc/pgx/v5` v5.9.2 — Primary datastore driver; all SQL is explicit (no ORM)
- `github.com/redis/go-redis/v9` v9.19.0 — Worker queue backend, delayed job scheduling, webhook deduplication
- `github.com/emersion/go-imap` / `go-smtp` / `go-message` / `go-webdav` — Protocol layer; do not replace with custom implementations
- `golang.org/x/crypto` v0.51.0 — Argon2id password hashing and TLS support

**Infrastructure:**
- `github.com/golang-migrate/migrate/v4` v4.19.1 — Schema versioning (`internal/migrate/migrations/`)
- `github.com/google/uuid` v1.6.0 — UUID generation (v7 used for job IDs)
- `github.com/go-acme/lego/v4` v4.35.2 — Automatic TLS certificate provisioning via DNS-01
- `github.com/microcosm-cc/bluemonday` v1.0.27 / `github.com/aymerick/douceur` v0.2.0 — HTML sanitization pipelines (indirect, via gin-contrib)

## Configuration

**Environment:**
- Primary source: TOML file at `/etc/postnest/postnest.conf` (default), overridable via `POSTNEST_CONFIG_PATH`
- Override layer: Environment variables using `POSTNEST_<SECTION>_<KEY>` naming (`internal/config/loader.go`)
- Legacy fallback: Bare env vars (e.g., `POSTGRES_DSN`, `SESSION_KEY`) still honored for backward compatibility
- `.env` file present at repo root but **not** read by the Go application; it is consumed by Docker Compose only

**Build:**
- `web/vite.config.js` — SPA build, outputs to `internal/webui/dist` with `emptyOutDir: true`
- `web/tailwind.config.js` — Custom color tokens (`primary`, `surface` scales)
- `go:embed` — Embeds `internal/webui/dist` into the `webui` binary
- `Makefile` — Defines `build-server`, `build-webui`, `build-worker`, `build-migrate`, `build-admin`

## Platform Requirements

**Development:**
- Go 1.25+
- Node.js 22+ (for frontend builds)
- PostgreSQL 16+
- Redis 7+
- `make` (optional)
- Docker + Docker Compose (optional)

**Production:**
- Deployment target: Linux containers (distroless static images)
- Optional: NixOS via `nixosModules.postnest`
- Optional: Systemd service wrapper (architecture approved per `CLAUDE.md`, not yet wired in current binary builds)

---

*Stack analysis: 2026-05-18*
