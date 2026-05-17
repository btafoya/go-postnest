# Technology Stack

**Analysis Date:** 2025-07-28

## Languages

**Primary:**
- Go 1.25.0 — All application code (`go 1.25.0` in `go.mod`)

**Secondary:**
- Shell (Bash) — Installation and deployment scripts (`scripts/install-systemd.sh`, `scripts/install-docker.sh`)
- Nix — Declarative packaging and NixOS module (`flake.nix`, `nix/module.nix`)
- TOML — Application configuration format (`postnest.conf`)
- SQL — PostgreSQL schema and migrations (`migrations/`)

## Runtime

**Environment:**
- Go 1.25+ (no browser runtime; server-side only)
- CGO disabled for container builds (`CGO_ENABLED=0`)

**Package Manager:**
- Go Modules (`go.mod`, `go.sum` present)
- Multi-stage Docker builds using `golang:1.25-alpine` → `gcr.io/distroless/static-debian12:nonroot`

## Frameworks

**Core:**
- `github.com/go-chi/chi/v5` v5.2.5 — HTTP router and middleware
- `github.com/emersion/go-imap` v1.2.1 — IMAP4rev1 server implementation
- `github.com/emersion/go-smtp` v0.24.0 — SMTP submission server
- `github.com/emersion/go-webdav` v0.7.0 — WebDAV/CardDAV/CalDAV handlers
- `github.com/emersion/go-message` v0.18.2 — MIME message parsing (RFC822)
- `github.com/emersion/go-vcard` — vCard 4.0 contact serialization
- `github.com/emersion/go-ical` — iCalendar parsing (used in CalDAV stub)
- `github.com/emersion/go-sasl` — SASL authentication for SMTP

**Testing:**
- Go standard `testing` package
- `github.com/alicebob/miniredis/v2` v2.38.0 — In-memory Redis for integration tests

**Build/Dev:**
- Go toolchain (`go build`, `go test`)
- `github.com/golang-migrate/migrate/v4` v4.19.1 — Database migration runner (embedded in `cmd/migrate`)
- Docker Compose — Local orchestration of PostgreSQL, Redis, server, worker, and migration services

## Key Dependencies

**Critical:**
- `github.com/jackc/pgx/v5` v5.9.2 — PostgreSQL driver and connection pool
- `github.com/redis/go-redis/v9` v9.19.0 — Redis client (job queues, IMAP IDLE pub/sub)
- `github.com/mrz1836/postmark` v1.9.2 — Postmark email API client (inbound/outbound)
- `github.com/go-acme/lego/v4` v4.35.2 — ACME/Let's Encrypt certificate automation
- `github.com/golang-migrate/migrate/v4` v4.19.1 — Database migration framework

**Infrastructure:**
- `github.com/go-chi/chi/v5` v5.2.5 — HTTP routing and middleware
- `github.com/google/uuid` v1.6.0 — UUID v7 generation
- `github.com/BurntSushi/toml` v1.6.0 — TOML configuration parsing
- `golang.org/x/crypto` v0.51.0 — Argon2id password hashing
- `github.com/microcosm-cc/bluemonday` v1.0.27 — HTML sanitization for inbound mail

## Configuration

**Environment:**
- Primary: TOML file at `/etc/postnest/postnest.conf` (or path via `POSTNEST_CONFIG_PATH`)
- Override: All TOML values overridable via `POSTNEST_<SECTION>_<KEY>` environment variables
- Legacy support: Pre-TOML env var names (e.g., `POSTGRES_DSN`, `SESSION_KEY`) still honored
- Docker Compose: `.env` file sourced from `.env.example`

**Key Required Configs:**
- `POSTNEST_DATABASE_DSN` — PostgreSQL connection string
- `POSTNEST_SECURITY_SESSION_KEY` — Secret key for session signing
- `POSTNEST_POSTMARK_WEBHOOK_SECRET` — Postmark webhook signature verification

**Build:**
- `go.mod` / `go.sum` — Dependency management
- `docker-compose.yml` — Local service orchestration
- `Dockerfile.server` / `Dockerfile.worker` / `Dockerfile.migrate` — Multi-stage container builds
- `flake.nix` + `nix/module.nix` — Nix package and NixOS module

## Platform Requirements

**Development:**
- macOS/Linux/Windows with Go 1.25+
- PostgreSQL 16+ (local or Docker)
- Redis 7+ (local or Docker)
- Postmark account for email transport

**Production:**
- Docker Compose stack (PostgreSQL, Redis, server, worker sidecars)
- Native systemd Linux service (see `scripts/install-systemd.sh`)
- NixOS module (declarative, see `nix/module.nix`)
- Distroless container images (`gcr.io/distroless/static-debian12:nonroot`)
- Exposed ports: HTTP 8080, IMAP 143/993, SMTP 587/465

---

*Stack analysis: 2025-07-28*
*Update after major dependency changes*
