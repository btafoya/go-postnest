# Requirements Specification: Multi-Mode Deployment & Installation

**Project**: PostNest (`github.com/go-postnest/postnest`)  
**Scope**: Deployment packaging, configuration, and installation for three execution modes.  
**Output of**: `/sc:brainstorm` — Requirements Discovery Only.  
**Next Step**: `/sc:design` for architecture, `/sc:workflow` for implementation planning.

---

## 1. Clarified User Goals

1. **System Service (Linux host)**: Run PostNest as native systemd services (server + worker) on a modern Linux host, using pre-installed host PostgreSQL and Redis, configured via `/etc/postnest/postnest.conf`.
2. **Docker Stack**: Run PostNest as a containerized Docker Compose stack (app + worker + postgres + redis), configured via `.env` with an entrypoint bridge to the config file.
3. **Nix Flake**: Run PostNest reproducibly via Nix — both as a NixOS module with native PostgreSQL/Redis services, and as a generic flake for non-NixOS users (devShell / `nix run`).
4. **Unified Configuration**: The application must load configuration from a TOML file, overridden by environment variables, so all three modes share the same config schema.
5. **Installer Per Mode**: Each mode must have a dedicated installer that validates dependencies, creates the database/user, installs the config template, and prints next steps (including manual migration).

---

## 2. Functional Requirements

### FR-1: Configuration System (TOML + Env Override)

| ID | Requirement | Priority |
|---|---|---|
| FR-1.1 | The application SHALL read configuration from `/etc/postnest/postnest.conf` (path configurable via `CONFIG_PATH` env var). | Must |
| FR-1.2 | The configuration file SHALL be TOML format with nested sections: `[server]`, `[database]`, `[redis]`, `[postmark]`, `[tls]`, `[worker]`, `[security]`. | Must |
| FR-1.3 | Environment variables SHALL override TOML values using a documented mapping (e.g., `POSTNEST_DATABASE_DSN` overrides `[database] dsn`). | Must |
| FR-1.4 | If the config file is missing and all required values are provided via env vars, the app SHALL start without error. | Must |
| FR-1.5 | The app SHALL print a clear fatal error listing missing required keys if neither file nor env provides them. | Must |
| FR-1.6 | A command-line flag `--print-config-template` SHALL output a documented TOML file with all defaults and comments. | Should |

### FR-2: Linux System Service Mode

| ID | Requirement | Priority |
|---|---|---|
| FR-2.1 | An install script (`install-systemd.sh`) SHALL be provided, runnable as root, which: installs the server and worker binaries to `/usr/local/bin/`, creates `/etc/postnest/`, writes a default TOML config, creates a systemd unit for each binary, reloads systemd, and enables + starts the services. | Must |
| FR-2.2 | The installer SHALL validate PostgreSQL and Redis connectivity before proceeding and abort with a diagnostic message if unreachable. | Must |
| FR-2.3 | The installer SHALL create the `postnest` database and a dedicated `postnest` PostgreSQL user if they do not exist (requires superuser or prompt for superuser credentials). | Must |
| FR-2.4 | The installer SHALL NOT install or manage PostgreSQL/Redis themselves; it assumes they are pre-installed by the host administrator. | Must |
| FR-2.5 | The systemd units SHALL run as a dedicated `postnest` system user (created by the installer if absent). | Must |
| FR-2.6 | The installer SHALL print a post-install message instructing the admin to run `postnest migrate` before starting services. | Must |
| FR-2.7 | An uninstall script (`uninstall-systemd.sh`) SHALL stop, disable, and remove units, binaries, and the `postnest` user (optional, with confirmation). | Should |

### FR-3: Docker Compose Stack Mode

| ID | Requirement | Priority |
|---|---|---|
| FR-3.1 | A `docker-compose.yml` SHALL define four services: `postgres` (`postgres:16-alpine`), `redis` (`redis:7-alpine`), `server` (built from `cmd/server`), `worker` (built from `cmd/worker`). | Must |
| FR-3.2 | A `.env.example` and `.env` file SHALL provide all required environment variables; the compose file SHALL reference these via `${VAR}`. | Must |
| FR-3.3 | A `Dockerfile` (or `Dockerfile.server` / `Dockerfile.worker`) SHALL produce minimal images based on `scratch` or `distroless` using multi-stage Go builds. | Must |
| FR-3.4 | The server and worker container entrypoint SHALL write `.env` values to `/etc/postnest/postnest.conf` inside the container before launching the binary, OR the app SHALL natively read `.env` values in Docker mode. **Decision**: App natively reads `.env` mapped env vars; config file template is optional inside the container. | Must |
| FR-3.5 | An install script (`install-docker.sh`) SHALL validate that `docker` and `docker compose` are installed, create the `.env` from `.env.example` if missing, run `docker compose up --build -d`, and wait for postgres to be healthy before printing next steps. | Must |
| FR-3.6 | The docker installer SHALL run migrations automatically via a one-off `docker compose run --rm worker migrate` step OR print the exact command for the admin to run. **Decision**: Print the exact command; do not auto-run to keep the admin in control. | Must |
| FR-3.7 | Persistent volumes SHALL be defined for `postgres_data` and optionally for mail/attachment storage if future S3 migration is deferred. | Should |

### FR-4: Nix Flake Mode

| ID | Requirement | Priority |
|---|---|---|
| FR-4.1 | A `flake.nix` SHALL expose: `packages.<system>.postnest-server`, `packages.<system>.postnest-worker`, `devShells.default`, and `nixosModules.postnest`. | Must |
| FR-4.2 | The `devShell` SHALL provide `go`, `postgres`, `redis`, `golangci-lint`, `migrate`, and `air`. | Must |
| FR-4.3 | The NixOS module SHALL declare systemd services (`services.postnest.server.enable`, `services.postnest.worker.enable`) that use the Nix-built binaries. | Must |
| FR-4.4 | The NixOS module SHALL integrate with `services.postgresql` and `services.redis` (or `services.redis.servers`) to ensure they are available; it SHALL create the `postnest` database and user declaratively via `services.postgresql.ensureDatabases` / `ensureUsers`. | Must |
| FR-4.5 | The NixOS module SHALL generate `/etc/postnest/postnest.conf` from Nix expression options (mapping NixOS module options to TOML). | Must |
| FR-4.6 | For non-NixOS hosts, a `nix run` or `nix develop` workflow SHALL be documented; `nix run` shall NOT require `/etc/postnest/postnest.conf` if all required env vars are exported in the shell. | Should |
| FR-4.7 | A helper script or nix app (`nix run .#install-nixos`) is NOT required; documentation + the NixOS module option reference is sufficient. | Should |

### FR-5: Migration & Database Bootstrap

| ID | Requirement | Priority |
|---|---|---|
| FR-5.1 | A CLI subcommand `postnest migrate` (or `migrate-up`, `migrate-down`) SHALL apply SQL migrations using `golang-migrate` or an embedded migration runner. | Must |
| FR-5.2 | Migrations SHALL be embedded into the binary (e.g., via `embed` + `golang-migrate` source driver) so the CLI works without a local `migrations/` directory. | Must |
| FR-5.3 | The installer for each mode SHALL print the exact migration command to run before first use. | Must |
| FR-5.4 | The app SHALL refuse to start if it detects an uninitialized database (missing `schema_migrations` table and no required tables). | Should |

### FR-6: Installer Common Behavior

| ID | Requirement | Priority |
|---|---|---|
| FR-6.1 | Each installer SHALL be idempotent where safe: re-running should not duplicate databases/users, but should overwrite config files only with explicit `--force` or confirmation. | Should |
| FR-6.2 | Each installer SHALL produce a clear summary of what was created, what the admin must do next, and how to view logs. | Must |
| FR-6.3 | Installer scripts SHALL be POSIX-compatible where feasible (bash acceptable for systemd/Docker; Nix uses Nix expressions). | Should |
| FR-6.4 | Installers SHALL accept non-interactive mode (`--yes` or env var `POSTNEST_INSTALL_NONINTERACTIVE=1`) for CI/automation. | Should |

---

## 3. Non-Functional Requirements

| ID | Requirement | Priority |
|---|---|---|
| NFR-1 | The TOML config parser + env override SHALL add <5ms to cold startup time. | Must |
| NFR-2 | Docker images SHALL be <50MB compressed (use `scratch` / `distroless` base). | Should |
| NFR-3 | No secrets (passwords, API tokens) SHALL be written to logs or printed by installers. | Must |
| NFR-4 | The systemd units SHALL use `Restart=on-failure` and `Type=simple` with proper `After=` dependencies on `network-online.target`. | Must |
| NFR-5 | Nix flake evaluation SHALL be pure (no IFD where avoidable) and support `x86_64-linux` and `aarch64-linux` at minimum. | Should |
| NFR-6 | The config schema SHALL be versioned (e.g., `config_version = 1` in TOML) so future breaking changes are detectable. | Should |
| NFR-7 | All installer output SHALL be redirectable to a log file and return a non-zero exit code on any fatal error. | Must |

---

## 4. User Stories & Acceptance Criteria

### US-1: System Administrator Installing on Ubuntu Server
> As a sysadmin, I want to install PostNest on an existing Ubuntu 24.04 server that already runs PostgreSQL 16 and Redis 7, so that I can start the mail platform as systemd services.

**Acceptance Criteria:**
- [ ] I download `install-systemd.sh` and run `sudo ./install-systemd.sh`.
- [ ] The script checks that `psql` and `redis-cli` can connect using the DSN/URL I provide (or localhost defaults).
- [ ] The script creates the `postnest` DB and user if missing.
- [ ] The script writes `/etc/postnest/postnest.conf` with my DSN and Redis URL.
- [ ] The script installs `postnest-server` and `postnest-worker` to `/usr/local/bin/`.
- [ ] The script creates and starts `postnest-server.service` and `postnest-worker.service` running as `postnest` user.
- [ ] The script prints: `Run 'postnest migrate' to initialize the database, then 'systemctl start postnest-server postnest-worker'`.
- [ ] After running `postnest migrate`, the services start cleanly and bind to the configured ports.

### US-2: Developer Running Locally with Docker
> As a developer, I want to spin up the entire stack with Docker Compose so I can test the webmail API without installing Postgres/Redis on my laptop.

**Acceptance Criteria:**
- [ ] I clone the repo and run `./install-docker.sh` (or `docker compose up --build`).
- [ ] The script ensures `.env` exists (copies from `.env.example` if missing) and prompts for `POSTMARK_TOKEN` and `SESSION_KEY`.
- [ ] `docker compose up` builds images and starts postgres, redis, server, and worker.
- [ ] The server container boots and connects to the `postgres` and `redis` services on the internal Docker network.
- [ ] The script prints the exact command to run migrations: `docker compose run --rm worker migrate`.
- [ ] After migration, `curl http://localhost:8080/api/v1/health` returns `200 OK`.

### US-3: NixOS User Enabling the Module
> As a NixOS user, I want to add PostNest to my `configuration.nix` and have all dependencies managed declaratively.

**Acceptance Criteria:**
- [ ] I add the flake input and set `services.postnest.server.enable = true; services.postnest.worker.enable = true;`.
- [ ] I set `services.postnest.database.host = "localhost";` and `services.postnest.redis.enable = true;`.
- [ ] Rebuilding NixOS creates the `postnest` PostgreSQL database and user, starts Redis, generates `/etc/postnest/postnest.conf`, and starts systemd units.
- [ ] Running `sudo -u postnest postnest migrate` initializes the schema.
- [ ] `systemctl status postnest-server` shows active (running).

### US-4: Generic Nix User (non-NixOS)
> As a developer on macOS or non-NixOS Linux, I want to enter a Nix dev shell and run the app locally.

**Acceptance Criteria:**
- [ ] `nix develop` drops me into a shell with Go, Postgres, Redis, and migrate available.
- [ ] `nix run .#postnest-server` starts the server using env vars exported in my shell (no `/etc/postnest/postnest.conf` required).
- [ ] `nix build .#postnest-server` produces a static binary in `./result/bin/`.

---

## 5. Configuration Schema (TOML)

```toml
config_version = 1

[server]
http_addr = ":8080"
imap_addr = ":143"
smtp_addr = ":587"

[database]
dsn = "postgres://postnest:changeme@localhost:5432/postnest?sslmode=disable"
read_dsn = ""

[redis]
url = "redis://localhost:6379/0"

[postmark]
webhook_secret = ""

[tls]
cert_path = ""
key_path = ""

[worker]
concurrency = 10
poll_interval = "5s"

[security]
session_key = "change-me-in-production"
session_expiry = "168h"
argon2id_time = 3
argon2id_memory = 65536
max_message_size = 52428800
max_attachment_size = 26214400
```

**Environment Variable Mapping:**
- `POSTNEST_SERVER_HTTP_ADDR` → `[server] http_addr`
- `POSTNEST_DATABASE_DSN` → `[database] dsn`
- `POSTNEST_REDIS_URL` → `[redis] url`
- ... (full mapping table to be produced in design phase)

---

## 6. Open Questions for User

1. **Binary naming**: Should the unified binary be named `postnest` with subcommands (`postnest server`, `postnest worker`, `postnest migrate`) or keep separate binaries (`postnest-server`, `postnest-worker`, `postnest-migrate`)?  
   - _Current codebase has `cmd/server` and `cmd/worker` as separate entrypoints._

2. **Config file path**: The user prompt said `/etc/go-postnext.conf`, but the project is `go-postnest`. Should the path be `/etc/postnest/postnest.conf` (namespaced directory) or `/etc/postnest.conf` (flat file)?

3. **Migration tool**: Should we stick with `golang-migrate` as an external dependency, or switch to an embedded migration runner (e.g., `pressly/goose`, `rubenv/sql-migrate`) so `postnest migrate` has zero external tool dependencies?

4. **Docker base image**: Is `scratch` acceptable (no shell for debugging), or do we prefer `alpine` or `distroless` for minimal debuggability?

5. **TLS in Docker**: Should the Docker mode auto-generate self-signed TLS certs for IMAP/SMTPS if no cert paths are provided, or leave TLS strictly opt-in with user-provided certs?

---

## 7. Out of Scope (Explicitly Excluded)

- Kubernetes / Helm charts.
- Windows service mode.
- macOS `.plist` service installer.
- Auto-update / watchtower mechanisms.
- Multi-node clustering or HA setup.
- S3 / external object storage migration (retain PostgreSQL `bytea` for now).

---

## 8. Next Steps

1. **Answer open questions** above.
2. **Run `/sc:design`** to produce: config loader interface, TOML parser selection, NixOS module structure, Dockerfile strategy.
3. **Run `/sc:workflow`** to sequence implementation: config refactor → systemd installer → Docker packaging → Nix flake → migration CLI → validation.
