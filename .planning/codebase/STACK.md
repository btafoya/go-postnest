# Technology Stack

## Overview

PostNest (`github.com/go-postnest/postnest`) is a Go 1.25 backend + React 19 frontend self-hosted mail platform. It compiles into five binaries (`server`, `worker`, `webui`, `admin`, `migrate`) and uses PostgreSQL as the primary store and Redis for queues / pub-sub.

## Languages & Runtimes

| Layer | Language / Runtime | Version | Files |
|---|---|---|---|
| Backend | Go | 1.25.0 | `go.mod` |
| Frontend | JavaScript (JSX) | ES2022+ | `web/src/**/*.jsx` |
| Styles | CSS (Tailwind) | — | `web/src/styles/index.css` |
| Migrations | SQL | PostgreSQL 16 | `internal/migrate/migrations/*.sql` |
| Config | TOML | — | `internal/config/loader.go`, `internal/config/template.go` |

The backend runs directly on the Go runtime. Node.js is only used at build time for the frontend.

## Backend Dependencies

### Direct (`go.mod`)

```go
require (
    github.com/go-chi/chi/v5 v5.2.5
    github.com/jackc/pgx/v5 v5.9.2
    github.com/redis/go-redis/v9 v9.19.0
    github.com/golang-migrate/migrate/v4 v4.19.1
    github.com/go-acme/lego/v4 v4.35.2
    github.com/mrz1836/postmark v1.9.2
    github.com/google/uuid v1.6.0
    github.com/emersion/go-imap v1.2.1
    github.com/emersion/go-smtp v0.24.0
    github.com/emersion/go-webdav v0.7.0
    github.com/emersion/go-ical v0.0.0-20250609112844-439c63cef608
    github.com/emersion/go-vcard v0.0.0-20241024213814-c9703dde27ff
    github.com/emersion/go-message v0.18.2
    github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
    github.com/microcosm-cc/bluemonday v1.0.27
    github.com/BurntSushi/toml v1.6.0
    golang.org/x/crypto v0.51.0
)
```

### Notable indirect

- `github.com/gin-gonic/gin v1.12.0` — used only by the `webui` binary for static-file serving, reverse proxy, and SSE (`internal/webui/router.go`).
- `github.com/go-playground/validator/v10` — Gin validation (email validator registered in `internal/webui/router.go`).
- `github.com/alicebob/miniredis/v2` — Redis mock for unit tests (`internal/redis/redis_test.go`, `internal/workers/workers_test.go`).
- `github.com/quic-go/quic-go`, `github.com/miekg/dns` — pulled in by lego.
- `go.mongodb.org/mongo-driver/v2` — indirect via lego.

## Frontend Stack

| Technology | Version | Role | File |
|---|---|---|---|
| React | ^19.0.0 | UI framework | `web/package.json` |
| React DOM | ^19.0.0 | Renderer | `web/package.json` |
| React Router DOM | ^7.0.0 | SPA routing | `web/package.json` |
| Vite | ^6.0.0 | Build tool / dev server | `web/package.json`, `web/vite.config.js` |
| Tailwind CSS | ^3.4.15 | Utility-first CSS | `web/package.json`, `web/tailwind.config.js` |
| PostCSS | ^8.4.49 | CSS processing | `web/postcss.config.js` |
| Autoprefixer | ^10.4.20 | Vendor prefixes | `web/postcss.config.js` |
| axios | ^1.7.0 | HTTP client | `web/src/api.js` |
| lucide-react | ^0.460.0 | Icon set | `web/package.json` |
| date-fns | ^4.1.0 | Date formatting | `web/package.json` |

### Vite configuration (`web/vite.config.js`)

```js
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
})
```

### Tailwind configuration (`web/tailwind.config.js`)

```js
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: { extend: {} },
  plugins: [],
}
```

## Configuration

PostNest uses a layered configuration system:

1. **TOML file** — default path `/etc/postnest/postnest.conf`, overridable via `POSTNEST_CONFIG_PATH`.
2. **Environment variables** — `POSTNEST_<SECTION>_<KEY>` overrides any TOML value.
3. **Legacy env names** — backward-compatible aliases such as `POSTGRES_DSN`, `SESSION_KEY`, etc.

### Config struct (`internal/config/config.go`)

```go
type Config struct {
    HTTPAddr     string        `env:"HTTP_ADDR" envDefault:":8080"`
    IMAPAddr     string        `env:"IMAP_ADDR" envDefault:":143"`
    IMAPSAddr    string        `env:"IMAPS_ADDR" envDefault:":993"`
    SMTPAddr     string        `env:"SMTP_ADDR" envDefault:":587"`
    SMTPSAddr    string        `env:"SMTPS_ADDR" envDefault:":465"`
    ReadTimeout  time.Duration `env:"READ_TIMEOUT" envDefault:"30s"`
    WriteTimeout time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
    AllowedOrigins []string

    TLSCertPath string
    TLSKeyPath  string

    ACMEEnabled       bool
    ACMEEmail         string
    ACMEDomain        string
    ACMEDirectory     string
    ACMECertDir       string
    ACMEDNSProvider   string
    ACMERenewInterval time.Duration
    ACMERenewBefore   time.Duration

    PostgresDSN     string
    PostgresReadDSN string
    MaxDBConns      int

    RedisURL string

    Argon2idTime    uint32
    Argon2idMemory  uint32
    Argon2idThreads uint8
    SessionKey      string
    SessionExpiry   time.Duration

    PostmarkWebhookSecret string

    WorkerConcurrency  int
    WorkerPollInterval time.Duration

    AllowInsecureAuth bool

    MaxMessageSize    int64
    MaxAttachmentSize int64
}
```

### Template generator (`internal/config/template.go`)

The binary can emit a documented TOML template via `--print-config-template`.

## Build & Packaging

### Makefile (`Makefile`)

```makefile
.PHONY: build build-server build-webui build-admin build-worker build-migrate

build: build-server build-webui build-admin build-worker build-migrate

build-server:
	go build -o postnest-server ./cmd/server

build-webui:
	cd web && npm ci && npm run build
	go build -o postnest-webui ./cmd/webui

build-admin:
	go build -o postnest-admin ./cmd/admin

build-worker:
	go build -o postnest-worker ./cmd/worker

build-migrate:
	go build -o postnest-migrate ./cmd/migrate
```

### Docker images

All images are multi-stage: `golang:1.25-alpine` builder → `gcr.io/distroless/static-debian12:nonroot` runtime.

| Binary | Dockerfile | Exposed ports |
|---|---|---|
| server | `Dockerfile.server` | 8080, 143, 587, 993, 465 |
| worker | `Dockerfile.worker` | none |
| webui | `Dockerfile.webui` | 3000 |
| migrate | `Dockerfile.migrate` | none |

### Docker Compose (`docker-compose.yml`)

Services defined:
- `postgres` (postgres:16-alpine)
- `redis` (redis:7-alpine)
- `server`
- `webui`
- `worker`
- `migrate`

### Nix (`flake.nix`, `nix/module.nix`)

A NixOS module and flake are included for declarative deployment.

## Database & Caching

### PostgreSQL (`internal/db/db.go`, `internal/migrate/migrations/000001_init.up.sql`)

- Version required: 16+
- Extension: `pgcrypto`
- Driver: `pgx/v5` with connection pooling (`pgxpool.Pool`)
- Tables: `domains`, `users`, `domain_members`, `auth_sessions`, `threads`, `messages`, `labels`, `message_labels`, `attachments`, `message_flags`, `imap_uids`, `contacts`, `contact_reputation`, `whitelist`, `greylist`, `blacklist`, `delivery_logs`, `webhook_events`, `bounce_events`.
- Full-text search: PostgreSQL `tsvector` with `GIN` index on `messages.search_vector`.
- Migrations: embedded SQL files using `golang-migrate/migrate/v4` (`internal/migrate/migrate.go`).

### Redis (`internal/redis/redis.go`)

- Library: `go-redis/v9`
- Uses: job queues (`LPUSH` / `BRPOP`), delayed jobs (`ZADD` / `ZREVRANGEBYSCORE`), dead-letter queue, deduplication (`SETNX`), SSE pub/sub (`Subscribe`/`Publish`).

## Protocols & Servers

| Protocol | Go Package | Status | Entrypoint |
|---|---|---|---|
| HTTP REST / Webhooks | `go-chi/chi/v5` | ✅ | `cmd/server/main.go` |
| IMAP4rev1 | `emersion/go-imap` | ✅ | `internal/imap/imap.go` |
| SMTP submission | `emersion/go-smtp` | ✅ | `internal/smtp/smtp.go` |
| CardDAV | `emersion/go-webdav/carddav` | ✅ | `internal/dav/dav.go` |
| CalDAV | `emersion/go-webdav/caldav` | 🚧 stub | `internal/dav/dav.go` |
| WebDAV | `emersion/go-webdav` | 🚧 partial | `internal/dav/dav.go` |

## Authentication & Security

- **Password hashing**: Argon2id via `golang.org/x/crypto/argon2` (`internal/auth/auth.go`).
- **Sessions**: 16-byte random tokens stored hashed in `auth_sessions` table; cookies with `HttpOnly`, `Secure`, `SameSite=Lax` (`internal/api/middleware.go`).
- **API keys**: validated as fallback after session cookie / Bearer token (`internal/api/middleware.go`).
- **Rate limiting**: per-IP token-bucket middleware (`internal/api/middleware.go`).
- **CORS**: restricted to configured origins (`internal/api/middleware.go` for Chi; `internal/webui/router.go` for Gin).
- **Recovery**: panic recovery middleware logging stack traces (`internal/api/middleware.go`).

## Logging & Observability

- Structured JSON logging via Go standard `log/slog` (`internal/logger/logger.go`).
- Every HTTP request logs method, path, duration, and request ID (`internal/api/middleware.go`).
- Gin webui logs status and latency (`internal/webui/router.go`).

## Testing

- Unit tests: `go test ./...`.
- Redis mocking: `miniredis/v2` in `internal/redis/redis_test.go`, `internal/workers/workers_test.go`, `internal/webhook/webhook_test.go`.

## Project Structure

```
cmd/
  server/   # HTTP + IMAP + SMTP + DAV server
  worker/   # Background job pool
  webui/    # Gin SPA server + SSE
  admin/    # CLI user/domain setup
  migrate/  # Embedded migration runner
internal/
  api/          # Shared middleware
  auth/         # Argon2id, sessions, API keys
  certmanager/  # ACME / static TLS
  config/       # TOML + env configuration
  contacts/     # PostgreSQL contact store
  dav/          # CardDAV/CalDAV handlers
  db/           # pgxpool wrapper
  imap/         # go-imap server
  logger/       # slog JSON logger
  mailstore/    # Mail persistence interface + PGStore
  migrate/      # golang-migrate wrapper
  models/       # Data structures
  postmark/     # Postmark HTTP client
  redis/        # Redis helpers
  reputation/   # Greylist / blacklist engine
  search/       # tsvector indexer
  smtp/         # go-smtp server
  webhook/      # Postmark webhook receiver
  webmail/      # REST handlers
  webui/        # Gin router, SSE, proxy
  workers/      # Redis-backed worker pool
web/
  src/          # React SPA
  package.json
  vite.config.js
  tailwind.config.js
```

## Deployment Artifacts

- `.env.example` — required environment variables for local / Docker Compose runs.
- `scripts/install-docker.sh` — Docker Compose setup script.
- `scripts/install-systemd.sh` — systemd service installation.
- `nix/module.nix` — NixOS module definition.
