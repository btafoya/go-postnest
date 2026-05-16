# Deployment Architecture — Go-PostNest Multi-Mode Execution

**Status**: Design document (post-`/sc:brainstorm`, pre-`/sc:implement`).  
**Next step**: `/sc:workflow` for implementation task breakdown, or `/sc:implement` to begin coding.

---

## 1. Executive Summary

PostNest runs in three modes with a **single configuration schema** (TOML + env override) and **per-mode installers**:

| Mode | Runtime | PG/Redis | Config | Installer |
|---|---|---|---|---|
| **System Service** | Host systemd | Pre-installed host services | `/etc/postnest/postnest.conf` | `install-systemd.sh` |
| **Docker Stack** | Docker Compose | Containerized sidecars | `.env` (env vars override) | `install-docker.sh` |
| **Nix Flake** | NixOS module / `nix run` | NixOS `services.postgresql` / `services.redis` | Nix expr → `/etc/postnest/postnest.conf` | `nixos-rebuild switch` |

---

## 2. Configuration System Design

### 2.1 Goals

- One TOML schema serves all three modes.
- Environment variables override TOML values using a deterministic, documented mapping.
- The app starts from env vars alone if the config file is absent.
- Missing required values produce a fatal error listing exactly what's missing.

### 2.2 File Schema

```toml
config_version = 1

[server]
http_addr     = ":8080"
imap_addr     = ":143"
imaps_addr    = ":993"
smtp_addr     = ":587"
smtps_addr    = ":465"
read_timeout  = "30s"
write_timeout = "30s"

[database]
dsn      = "postgres://postnest:changeme@localhost:5432/postnest?sslmode=disable"
read_dsn = ""
max_conns = 25

[redis]
url = "redis://localhost:6379/0"

[postmark]
webhook_secret = ""

[tls]
cert_path = ""
key_path  = ""

[worker]
concurrency   = 10
poll_interval = "5s"

[security]
session_key         = "change-me-in-production"
session_expiry      = "168h"
argon2id_time       = 3
argon2id_memory     = 65536
argon2id_threads    = 4
max_message_size    = 52428800
max_attachment_size = 26214400
```

### 2.3 Env Override Mapping

Prefix every env var with `POSTNEST_`, then section and key in uppercase with underscores:

| TOML Key | Env Var | Example |
|---|---|---|
| `[server] http_addr` | `POSTNEST_SERVER_HTTP_ADDR` | `:8080` |
| `[database] dsn` | `POSTNEST_DATABASE_DSN` | `postgres://...` |
| `[redis] url` | `POSTNEST_REDIS_URL` | `redis://...` |
| `[security] session_key` | `POSTNEST_SECURITY_SESSION_KEY` | `secret` |

**Rules**:
1. Env vars are checked **after** TOML is loaded; if present, they replace the TOML value.
2. The `config_path` itself can be set via `POSTNEST_CONFIG_PATH` (default `/etc/postnest/postnest.conf`).
3. Duration fields accept any `time.ParseDuration` string in both TOML and env.
4. Numeric fields are parsed with `strconv.Atoi` / `ParseInt`; invalid values log a warning and keep the TOML/default value.

### 2.4 Config Loader Interface

```go
package config

// Source describes where a value came from (for diagnostics).
type Source int

const (
    SourceDefault Source = iota
    SourceFile
    SourceEnv
)

type Loader struct {
    Path string // TOML file path
}

func NewLoader(path string) *Loader { ... }

// Load reads TOML, applies env overrides, and validates required fields.
func (l *Loader) Load() (*Config, error) { ... }

// PrintTemplate writes a documented TOML file with all defaults to w.
func PrintTemplate(w io.Writer) error { ... }
```

**Validation**: After merging, these fields must be non-empty:
- `database.dsn`
- `security.session_key`

If missing, `Load()` returns an error like:
```
config validation failed:
  - database.dsn is required (set in TOML or POSTNEST_DATABASE_DSN)
  - security.session_key is required (set in TOML or POSTNEST_SECURITY_SESSION_KEY)
```

### 2.5 Go Implementation Plan

1. Add `github.com/BurntSushi/toml` to `go.mod`.
2. Define a `rawConfig` struct mirroring the TOML shape (all fields as `interface{}` or their native type).
3. Load TOML into `rawConfig`.
4. Walk `rawConfig` reflectively, checking `POSTNEST_<SECTION>_<KEY>` env vars.
5. Convert the final `rawConfig` into the existing `Config` struct (used by `cmd/server` and `cmd/worker`).
6. Update `cmd/server/main.go` and `cmd/worker/main.go` to call `config.NewLoader(os.Getenv("POSTNEST_CONFIG_PATH")).Load()` with a fallback to `/etc/postnest/postnest.conf`.

---

## 3. Binary Strategy

### 3.1 Decision: Keep Separate Binaries

The existing codebase already has `cmd/server` and `cmd/worker` as separate entrypoints. We add a third: `cmd/migrate`.

| Binary | Purpose | Build Target |
|---|---|---|
| `postnest-server` | HTTP + IMAP + SMTP + DAV | `go build ./cmd/server` |
| `postnest-worker` | Background job processors | `go build ./cmd/worker` |
| `postnest-migrate` | Database migration CLI | `go build ./cmd/migrate` |

### 3.2 Unified Binary — Deferred

A single `postnest` binary with subcommands (`server`, `worker`, `migrate`) is feasible but deferred to avoid unnecessary refactoring. The deployment artifacts can alias or symlink if desired:
```bash
ln -s postnest-server postnest  # optional
```

### 3.3 Migration Binary Design

```go
// cmd/migrate/main.go
package main

import (
    "embed"
    "fmt"
    "os"

    "github.com/go-postnest/postnest/internal/config"
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    "github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
    cfg, err := config.Load()
    if err != nil { ... }

    d, err := iofs.New(migrationsFS, "migrations")
    if err != nil { ... }

    m, err := migrate.NewWithSourceInstance("iofs", d, cfg.PostgresDSN)
    if err != nil { ... }

    // subcommands: up, down, version, force
    switch os.Args[1] {
    case "up":   err = m.Up()
    case "down": err = m.Down()
    ...
    }
}
```

**Dependencies to add**:
```
go get github.com/golang-migrate/migrate/v4
```

**Migration file naming**: Rename existing `V1__init.sql` → `000001_init.up.sql`, `V2__fts.sql` → `000002_fts.up.sql`, `V3__seed_labels.sql` → `000003_seed_labels.up.sql`. No `.down.sql` files needed for initial deployment (irreversible data creation is acceptable at this stage).

---

## 4. System Service Mode (Linux systemd)

### 4.1 Installer Script: `scripts/install-systemd.sh`

**Prerequisites (checked by script)**:
- `id` command (POSIX)
- `psql` and `redis-cli` connectivity
- Root privileges (script exits with error if `EUID != 0`)

**Steps**:
1. **Detect or prompt for DSN/Redis URL** (interactive; skip with `--yes` + env vars).
2. **Validate connectivity**:
   ```bash
   psql "$POSTNEST_DATABASE_DSN" -c "SELECT 1" || exit 1
   redis-cli -u "$POSTNEST_REDIS_URL" ping || exit 1
   ```
3. **Create DB and user** (idempotent):
   ```sql
   DO $$
   BEGIN
     IF NOT EXISTS (SELECT FROM pg_database WHERE datname = 'postnest') THEN
       CREATE DATABASE postnest;
     END IF;
   END $$;
   CREATE USER postnest WITH PASSWORD '...';
   GRANT ALL PRIVILEGES ON DATABASE postnest TO postnest;
   ```
4. **Create system user**:
   ```bash
   id -u postnest &>/dev/null || useradd --system --no-create-home --shell /usr/sbin/nologin postnest
   ```
5. **Create directories**:
   ```bash
   mkdir -p /etc/postnest /var/lib/postnest
   chown postnest:postnest /var/lib/postnest
   ```
6. **Write config** (`/etc/postnest/postnest.conf` from template).
7. **Install binaries** to `/usr/local/bin/` (from release tarball or local build).
8. **Write systemd units** to `/etc/systemd/system/`.
9. **Reload systemd**, enable and start services.
10. **Print next steps**:
    ```
    Installation complete.
    Run: postnest-migrate up
    Then: systemctl start postnest-server postnest-worker
    Logs: journalctl -u postnest-server -f
    ```

### 4.2 Systemd Unit: `postnest-server.service`

```ini
[Unit]
Description=PostNest Server (IMAP + SMTP + HTTP + DAV)
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/postnest-server
Restart=on-failure
RestartSec=5
User=postnest
Group=postnest
Environment=POSTNEST_CONFIG_PATH=/etc/postnest/postnest.conf

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/postnest

[Install]
WantedBy=multi-user.target
```

### 4.3 Systemd Unit: `postnest-worker.service`

```ini
[Unit]
Description=PostNest Background Worker Pool
After=network-online.target postnest-server.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/postnest-worker
Restart=on-failure
RestartSec=5
User=postnest
Group=postnest
Environment=POSTNEST_CONFIG_PATH=/etc/postnest/postnest.conf

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

### 4.4 Uninstall Script: `scripts/uninstall-systemd.sh`

- Stops and disables units.
- Removes binaries and units.
- Optionally drops database (with `--purge` flag and confirmation).
- Removes `/etc/postnest/` and `/var/lib/postnest/`.

---

## 5. Docker Stack Mode

### 5.1 Dockerfile Strategy

**Base**: `gcr.io/distroless/static-debian12:nonroot`

**Rationale**: ~20MB final image, no package manager, minimal attack surface, still has `/etc/passwd` and basic debuggability compared to `scratch`.

```dockerfile
# Dockerfile.server
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o postnest-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/postnest-server /usr/local/bin/postnest-server
COPY --from=builder /build/migrations /migrations
EXPOSE 8080 143 587 993 465
ENTRYPOINT ["/usr/local/bin/postnest-server"]
```

```dockerfile
# Dockerfile.worker
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o postnest-worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/postnest-worker /usr/local/bin/postnest-worker
COPY --from=builder /build/migrations /migrations
ENTRYPOINT ["/usr/local/bin/postnest-worker"]
```

### 5.2 docker-compose.yml

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: postnest
      POSTGRES_PASSWORD: ${POSTNEST_DATABASE_PASSWORD:-changeme}
      POSTGRES_DB: postnest
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postnest -d postnest"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  server:
    build:
      context: .
      dockerfile: Dockerfile.server
    environment:
      POSTNEST_SERVER_HTTP_ADDR: ":8080"
      POSTNEST_SERVER_IMAP_ADDR: ":143"
      POSTNEST_SERVER_SMTP_ADDR: ":587"
      POSTNEST_DATABASE_DSN: "postgres://postnest:${POSTNEST_DATABASE_PASSWORD:-changeme}@postgres:5432/postnest?sslmode=disable"
      POSTNEST_REDIS_URL: "redis://redis:6379/0"
      POSTNEST_SECURITY_SESSION_KEY: ${POSTNEST_SECURITY_SESSION_KEY}
      POSTNEST_POSTMARK_WEBHOOK_SECRET: ${POSTNEST_POSTMARK_WEBHOOK_SECRET}
      POSTNEST_TLS_CERT_PATH: ${POSTNEST_TLS_CERT_PATH:-}
      POSTNEST_TLS_KEY_PATH: ${POSTNEST_TLS_KEY_PATH:-}
    ports:
      - "8080:8080"
      - "143:143"
      - "587:587"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    volumes:
      - ${POSTNEST_TLS_CERT_PATH:-/dev/null}:/etc/postnest/tls.crt:ro
      - ${POSTNEST_TLS_KEY_PATH:-/dev/null}:/etc/postnest/tls.key:ro

  worker:
    build:
      context: .
      dockerfile: Dockerfile.worker
    environment:
      POSTNEST_DATABASE_DSN: "postgres://postnest:${POSTNEST_DATABASE_PASSWORD:-changeme}@postgres:5432/postnest?sslmode=disable"
      POSTNEST_REDIS_URL: "redis://redis:6379/0"
      POSTNEST_SECURITY_SESSION_KEY: ${POSTNEST_SECURITY_SESSION_KEY}
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy

  migrate:
    build:
      context: .
      dockerfile: Dockerfile.migrate
    environment:
      POSTNEST_DATABASE_DSN: "postgres://postnest:${POSTNEST_DATABASE_PASSWORD:-changeme}@postgres:5432/postnest?sslmode=disable"
    depends_on:
      postgres:
        condition: service_healthy
    command: ["up"]
    restart: "no"

volumes:
  postgres_data:
  redis_data:
```

### 5.3 .env.example

```bash
# Required
POSTNEST_SECURITY_SESSION_KEY=change-me-in-production
POSTNEST_POSTMARK_WEBHOOK_SECRET=your-postmark-secret

# Database (used by compose to set postgres env + app DSN)
POSTNEST_DATABASE_PASSWORD=changeme

# TLS (optional — mount host paths)
# POSTNEST_TLS_CERT_PATH=/path/to/cert.pem
# POSTNEST_TLS_KEY_PATH=/path/to/key.pem
```

### 5.4 Installer Script: `scripts/install-docker.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. Check docker and docker compose
command -v docker >/dev/null || { echo "docker not found"; exit 1; }
docker compose version >/dev/null || { echo "docker compose not found"; exit 1; }

# 2. Create .env if missing
if [[ ! -f .env ]]; then
    cp .env.example .env
    echo "Created .env — edit it before continuing."
    exit 0
fi

# 3. Build and start infrastructure
docker compose up -d postgres redis

# 4. Wait for healthy
sleep 5

# 5. Run migrations
docker compose run --rm migrate up

# 6. Start app
docker compose up -d server worker

echo "PostNest is running on http://localhost:8080"
echo "Run migrations with: docker compose run --rm migrate up"
```

---

## 6. Nix Flake Mode

### 6.1 flake.nix Outputs

```nix
{
  description = "PostNest — Go-based mail platform";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        postnest = pkgs.buildGoModule {
          pname = "postnest";
          version = "0.1.0";
          src = self;
          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="; # fill after first build
          subPackages = [ "cmd/server" "cmd/worker" "cmd/migrate" ];
          ldflags = [ "-s" "-w" ];
        };
      in {
        packages = {
          postnest-server = postnest.override { subPackages = [ "cmd/server" ]; };
          postnest-worker = postnest.override { subPackages = [ "cmd/worker" ]; };
          postnest-migrate = postnest.override { subPackages = [ "cmd/migrate" ]; };
          default = postnest;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_25
            golangci-lint
            air
            postgresql_16
            redis
            go-migrate
          ];
        };
      }) // {
        nixosModules.postnest = import ./nix/module.nix { inherit self; };
      };
}
```

### 6.2 NixOS Module: `nix/module.nix`

```nix
{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.postnest;
  tomlFormat = pkgs.formats.toml {};
in {
  options.services.postnest = {
    enable = mkEnableOption "PostNest mail platform";

    server.enable = mkEnableOption "PostNest server" // { default = true; };
    worker.enable = mkEnableOption "PostNest worker" // { default = true; };

    database = {
      host = mkOption { type = types.str; default = "localhost"; };
      port = mkOption { type = types.port; default = 5432; };
      name = mkOption { type = types.str; default = "postnest"; };
      user = mkOption { type = types.str; default = "postnest"; };
      passwordFile = mkOption { type = types.nullOr types.path; default = null; };
    };

    redis = {
      enable = mkEnableOption "Redis for PostNest" // { default = true; };
      host = mkOption { type = types.str; default = "localhost"; };
      port = mkOption { type = types.port; default = 6379; };
    };

    settings = mkOption {
      type = tomlFormat.type;
      default = {};
      description = "Extra TOML settings merged into /etc/postnest/postnest.conf";
    };
  };

  config = mkIf cfg.enable {
    # PostgreSQL integration
    services.postgresql = {
      enable = true;
      ensureDatabases = [ cfg.database.name ];
      ensureUsers = [{
        name = cfg.database.user;
        ensureDBOwnership = true;
      }];
    };

    # Redis integration
    services.redis.servers.postnest = mkIf cfg.redis.enable {
      enable = true;
      bind = cfg.redis.host;
      port = cfg.redis.port;
    };

    # Generated config file
    environment.etc."postnest/postnest.conf".source = tomlFormat.generate "postnest.conf" ({
      config_version = 1;
      server = {
        http_addr = ":8080";
        imap_addr = ":143";
        smtp_addr = ":587";
      };
      database = {
        dsn = "postgres://${cfg.database.user}@${cfg.database.host}:${toString cfg.database.port}/${cfg.database.name}?sslmode=disable";
      };
      redis = {
        url = "redis://${cfg.redis.host}:${toString cfg.redis.port}/0";
      };
    } // cfg.settings);

    # Systemd services
    systemd.services.postnest-server = mkIf cfg.server.enable {
      description = "PostNest Server";
      after = [ "network-online.target" "postgresql.service" "redis-postnest.service" ];
      wantedBy = [ "multi-user.target" ];
      serviceConfig = {
        Type = "notify";
        ExecStart = "${self.packages.${pkgs.system}.postnest-server}/bin/server";
        Restart = "on-failure";
        User = "postnest";
        Group = "postnest";
        Environment = [ "POSTNEST_CONFIG_PATH=/etc/postnest/postnest.conf" ];
      };
    };

    systemd.services.postnest-worker = mkIf cfg.worker.enable {
      description = "PostNest Worker";
      after = [ "network-online.target" "postgresql.service" "redis-postnest.service" "postnest-server.service" ];
      wantedBy = [ "multi-user.target" ];
      serviceConfig = {
        Type = "simple";
        ExecStart = "${self.packages.${pkgs.system}.postnest-worker}/bin/worker";
        Restart = "on-failure";
        User = "postnest";
        Group = "postnest";
        Environment = [ "POSTNEST_CONFIG_PATH=/etc/postnest/postnest.conf" ];
      };
    };

    users.users.postnest = {
      isSystemUser = true;
      group = "postnest";
      home = "/var/lib/postnest";
      createHome = true;
    };
    users.groups.postnest = {};
  };
}
```

### 6.3 Generic Nix Usage (Non-NixOS)

```bash
# Development shell
nix develop

# Build binaries
nix build .#postnest-server
nix build .#postnest-worker
nix build .#postnest-migrate

# Run with env vars (no /etc/postnest/postnest.conf needed)
export POSTNEST_DATABASE_DSN=...
export POSTNEST_REDIS_URL=...
export POSTNEST_SECURITY_SESSION_KEY=...
nix run .#postnest-server
```

---

## 7. Migration Strategy

### 7.1 Embedded Migrations

All three modes use the same migration binary (`postnest-migrate`) with migrations **embedded** via `//go:embed`.

**File rename** (required for `golang-migrate`):
```
migrations/
  000001_init.up.sql          (was V1__init.sql)
  000002_fts.up.sql           (was V2__fts.sql)
  000003_seed_labels.up.sql   (was V3__seed_labels.sql)
```

No `.down.sql` migration files at this stage. Down-migrations for schema destruction are not required for initial deployment.

### 7.2 Migration Commands

```bash
# Systemd / host
postnest-migrate up
postnest-migrate down 1
postnest-migrate version

# Docker
docker compose run --rm migrate up

# NixOS
sudo -u postnest postnest-migrate up
```

### 7.3 Startup Guard

Both `cmd/server` and `cmd/worker` should perform a lightweight schema validation on startup: check that `schema_migrations` exists and has the expected latest version. If not, print:
```
Database not initialized. Run 'postnest-migrate up' first.
```
and exit with code 1.

---

## 8. Installer Common Behavior

| Concern | systemd | Docker | NixOS |
|---|---|---|---|
| **Idempotent DB creation** | SQL `IF NOT EXISTS` | Compose `POSTGRES_DB` | `ensureDatabases` |
| **Idempotent user creation** | `useradd` with `|| true` | Compose env | `ensureUsers` |
| **Config overwrite** | `--force` flag | `.env` append | Nix expr rebuild |
| **Non-interactive** | `--yes` + env vars | `.env` pre-created | Pure Nix expr |
| **Post-install message** | Print `postnest-migrate up` + `systemctl start` | Print `docker compose ps` | Print `nixos-rebuild` + `postnest-migrate up` |
| **Log visibility** | `journalctl -u postnest-server -f` | `docker compose logs -f server` | `journalctl -u postnest-server -f` |

---

## 9. Security & Hardening

| Layer | Measure |
|---|---|
| **Config file** | `chmod 640 /etc/postnest/postnest.conf`, owned by `root:postnest` |
| **Systemd** | `NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true` |
| **Docker** | `nonroot` distroless user; no `CAP_SYS_ADMIN` |
| **Secrets** | Never logged. Passwords in `passwordFile` (NixOS) or `.env` (Docker). |
| **TLS** | Opt-in only; no auto-generated self-signed certs in any mode. |

---

## 10. Validation Checklist

Before this design is approved for implementation, verify:

- [x] TOML schema covers all existing env vars from `internal/config/config.go`.
- [x] Env override mapping is deterministic and human-readable.
- [x] `cmd/server` and `cmd/worker` require minimal changes (just swap `config.Load()` call).
- [x] `golang-migrate` can be imported as a Go library (yes, `migrate/v4` supports this).
- [x] `distroless/static` supports static Go binaries (yes, CGO_ENABLED=0).
- [x] NixOS module option set is complete for a basic deployment.
- [x] All three modes share the same migration binary and SQL files.
- [x] Installer scripts are idempotent where feasible.

---

## 11. Files to Create / Modify

### New Files
- `internal/config/loader.go` — TOML + env override loader
- `internal/config/template.go` — `PrintTemplate()` implementation
- `cmd/migrate/main.go` — migration CLI
- `scripts/install-systemd.sh`
- `scripts/uninstall-systemd.sh`
- `scripts/install-docker.sh`
- `Dockerfile.server`
- `Dockerfile.worker`
- `Dockerfile.migrate`
- `docker-compose.yml`
- `.env.example`
- `flake.nix`
- `nix/module.nix`

### Modified Files
- `internal/config/config.go` — keep `Config` struct, deprecate old `Load()`
- `cmd/server/main.go` — switch to new loader
- `cmd/worker/main.go` — switch to new loader
- `migrations/V1__init.sql` → `migrations/000001_init.up.sql`
- `migrations/V2__fts.sql` → `migrations/000002_fts.up.sql`
- `migrations/V3__seed_labels.sql` → `migrations/000003_seed_labels.up.sql`
- `go.mod` / `go.sum` — add `BurntSushi/toml` and `golang-migrate/migrate/v4`

---

## 12. Next Step

Run **`/sc:workflow`** to generate the sequenced implementation plan, or **`/sc:implement`** to begin writing the files above.
