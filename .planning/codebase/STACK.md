# Technology Stack

**Analysis Date:** 2026-05-18

## Languages

**Primary:**
- Go 1.25.0 - Backend services, IMAP/SMTP servers, worker pool, API handlers, migrations (`cmd/`, `internal/`)
- JavaScript (JSX) - Frontend web UI (`web/src/`)
- SQL - Embedded PostgreSQL migrations (`internal/migrate/migrations/*.sql`)

**Secondary:**
- Nix - Development shell and package definitions (`flake.nix`, `nix/`)
- Dockerfile - Multi-stage container builds (`Dockerfile.server`, `Dockerfile.webui`, `Dockerfile.worker`, `Dockerfile.migrate`)

## Runtime

**Environment:**
- Go 1.25 (compiled binaries, statically linked with `CGO_ENABLED=0`)
- Node.js 22 (frontend build only, via `node:22-alpine` in `Dockerfile.webui`)

**Package Manager:**
- Go Modules (`go.mod`, `go.sum` present)
- npm (`web/package.json`, `web/package-lock.json` present)

**Lockfiles:**
- `go.sum` - Go dependency checksums
- `web/package-lock.json` - npm dependency checksums

## Frameworks

**Core Backend:**
- `github.com/go-chi/chi/v5` v5.2.5 - HTTP router and middleware for the main API server (`cmd/server/main.go`)
- `github.com/gin-gonic/gin` v1.12.0 - HTTP router for the web UI reverse-proxy server (`cmd/webui/main.go`, `internal/webui/router.go`)

**Frontend:**
- React 19.0.0 - UI framework (`web/src/main.jsx`)
- React Router DOM 7.0.0 - Client-side routing (`web/src/App.jsx`)
- Vite 6.0.0 - Build tool and dev server (`web/vite.config.js`)
- Tailwind CSS 3.4.15 - Utility-first CSS (`web/tailwind.config.js`)
- PostCSS 8.4.49 + Autoprefixer 10.4.20 - CSS processing (`web/postcss.config.js`)

**Testing:**
- Vitest 4.1.6 - Frontend test runner (`web/vite.config.js`)
- `@vitest/coverage-v8` 4.1.6 - Coverage reporting
- `@testing-library/react` 16.3.2 + `@testing-library/jest-dom` 6.9.1 - React component testing
- `jsdom` 29.1.1 - Browser environment for tests
- `msw` 2.14.6 - Mock Service Worker for API mocking in tests
- Go `testing` standard library - Backend tests (`internal/calendar/ical_test.go`)

**Build/Dev:**
- Vite (frontend build)
- Make (`Makefile`) - Orchestrates Go binary builds and admin CLI tasks
- golangci-lint (Nix devShell) - Go linting
- `air` (Nix devShell) - Go live-reload

## Key Dependencies

**Critical Infrastructure:**
- `github.com/jackc/pgx/v5` v5.9.2 - PostgreSQL driver and connection pool (`internal/db/db.go`)
- `github.com/redis/go-redis/v9` v9.19.0 - Redis client, used for job queue, pub/sub, and caching (`internal/redis/redis.go`)
- `github.com/golang-migrate/migrate/v4` v4.19.1 - Database migrations with embedded SQL files (`internal/migrate/migrate.go`)

**Mail & Protocols:**
- `github.com/emersion/go-imap` v1.2.1 - IMAP server implementation (`internal/imap/imap.go`)
- `github.com/emersion/go-smtp` v0.24.0 - SMTP server implementation (`internal/smtp/smtp.go`)
- `github.com/emersion/go-message` v0.18.2 - MIME message parsing (`internal/smtp/smtp.go`)
- `github.com/emersion/go-sasl` v0.0.0-20241020182733-b788ff22d5a6 - SASL authentication mechanisms (`internal/smtp/smtp.go`)
- `github.com/emersion/go-ical` v0.0.0-20250609112844-439c63cef608 - iCalendar parsing and generation (`internal/calendar/ical.go`)
- `github.com/emersion/go-vcard` v0.0.0-20241024213814-c9703dde27ff - vCard parsing and generation (`internal/dav/dav.go`)
- `github.com/emersion/go-webdav` v0.7.0 - WebDAV/CardDAV/CalDAV server backend (`internal/dav/dav.go`)

**Email Delivery:**
- `github.com/mrz1836/postmark` v1.9.2 - Postmark API client for outbound email and inbound webhook parsing (`internal/postmark/postmark.go`)

**Security & Auth:**
- `golang.org/x/crypto` v0.51.0 - Argon2id password hashing (`internal/auth/auth.go`)
- `github.com/go-acme/lego/v4` v4.35.2 - ACME/Let's Encrypt certificate automation (`cmd/server/main.go`)

**Data & Serialization:**
- `github.com/google/uuid` v1.6.0 - UUID v7 generation throughout the codebase
- `github.com/BurntSushi/toml` v1.6.0 - TOML parsing
- `github.com/goccy/go-yaml` v1.19.2 - YAML parsing (indirect)

**Frontend UI:**
- `@tiptap/react` 3.23.4 + `@tiptap/starter-kit` 3.23.4 + `@tiptap/pm` 3.23.4 - Rich text editor (`web/src/components/RichEditor.jsx`)
- `axios` 1.7.0 - HTTP client for backend API (`web/src/api.js`)
- `date-fns` 4.1.0 - Date formatting and manipulation
- `dompurify` 3.4.4 - HTML sanitization
- `lucide-react` 0.460.0 - Icon library

## Configuration

**Environment Variables (Backend):**
All backend services load configuration from environment variables with `POSTNEST_` prefix. See `internal/config/config.go` for the full schema.

Key variables:
- `POSTNEST_DATABASE_DSN` / `POSTNEST_POSTGRES_DSN` - PostgreSQL connection string
- `POSTNEST_REDIS_URL` - Redis connection URL (default `redis://localhost:6379/0`)
- `POSTNEST_SECURITY_SESSION_KEY` - Session signing key (required)
- `POSTNEST_POSTMARK_WEBHOOK_SECRET` - Postmark webhook HMAC secret
- `POSTNEST_TLS_CERT_PATH` / `POSTNEST_TLS_KEY_PATH` - Static TLS certificate paths
- `POSTNEST_ACME_ENABLED` / `POSTNEST_ACME_EMAIL` / `POSTNEST_ACME_DOMAIN` - ACME auto-TLS
- `POSTNEST_WORKER_CONCURRENCY` / `POSTNEST_WORKER_POLL_INTERVAL` - Worker pool tuning

**Environment Variables (WebUI):**
- `WEBUI_ADDR` - Bind address (default `:3000`)
- `WEBUI_API_BASE_URL` - Backend API proxy target (default `http://localhost:8080`)
- `WEBUI_REDIS_URL` - Redis for SSE pub/sub
- `WEBUI_ALLOWED_ORIGINS` - CORS origins

**Build Configuration:**
- `web/vite.config.js` - Vite build config, outputs to `../internal/webui/dist`
- `web/tailwind.config.js` - Tailwind theme with custom `primary` and `surface` color palettes
- `Makefile` - Build targets for `postnest-server`, `postnest-webui`, `postnest-admin`, `postnest-worker`, `postnest-migrate`
- `flake.nix` - Nix development shell with Go 1.25, PostgreSQL 16, Redis, golangci-lint, air, go-migrate

**Docker:**
- `docker-compose.yml` - Orchestrates postgres:16-alpine, redis:7-alpine, server, webui, worker, migrate services
- `Dockerfile.server` - Multi-stage Go build, distroless nonroot runtime, exposes 8080/143/587/993/465
- `Dockerfile.webui` - Multi-stage Node+Go build, embeds Vite dist, distroless nonroot runtime, exposes 3000
- `Dockerfile.worker` - Similar to server for worker binary
- `Dockerfile.migrate` - Migration runner binary

## Platform Requirements

**Development:**
- Go 1.25+
- Node.js 22+ (for frontend builds)
- PostgreSQL 16+
- Redis 7+
- Make (optional)
- Docker + Docker Compose (optional)

**Production:**
- Container runtime (Docker or Kubernetes) recommended
- PostgreSQL 16+ with `pgcrypto` extension (for `gen_random_uuid()`)
- Redis 7+ for job queue and pub/sub
- TLS certificate (static files or ACME auto-provisioning)
- Inbound internet access on ports 143/587 (or 993/465 with TLS) for IMAP/SMTP
- Outbound internet access for Postmark API (`api.postmarkapp.com`)

---

*Stack analysis: 2026-05-18*
