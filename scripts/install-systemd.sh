#!/usr/bin/env bash
set -euo pipefail

# PostNest Systemd Installer
# Requires root privileges.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
CONFIG_PATH="${POSTNEST_CONFIG_PATH:-/etc/postnest/postnest.conf}"
BIN_DIR="${POSTNEST_BIN_DIR:-/usr/local/bin}"
SYSTEMD_DIR="/etc/systemd/system"
DB_NAME="postnest"
DB_USER="postnest"
RUN_USER="postnest"
RUN_GROUP="postnest"

INTERACTIVE=1
if [[ "${POSTNEST_INSTALL_NONINTERACTIVE:-}" == "1" || "${1:-}" == "--yes" ]]; then
    INTERACTIVE=0
fi

log() { echo "[postnest-install] $*"; }
die() { echo "[postnest-install] ERROR: $*" >&2; exit 1; }

require_root() {
    if [[ $EUID -ne 0 ]]; then
        die "This script must be run as root. Use sudo."
    fi
}

check_deps() {
    command -v systemctl >/dev/null || die "systemctl not found. Is systemd installed?"
    command -v psql >/dev/null || die "psql not found. Install postgresql-client."
    command -v redis-cli >/dev/null || die "redis-cli not found. Install redis-tools."
}

prompt_or_env() {
    local var_name="$1"
    local prompt_text="$2"
    local default_val="${3:-}"

    if [[ -n "${!var_name:-}" ]]; then
        log "Using ${var_name} from environment."
        return
    fi

    if [[ $INTERACTIVE -eq 0 ]]; then
        if [[ -n "$default_val" ]]; then
            printf '%s' "$prompt_text"
            read -r -t 0.1 < /dev/null 2>/dev/null || true
            if [[ -z "${!var_name:-}" ]]; then
                printf '%s\n' "$default_val"
                export "$var_name=$default_val"
            fi
        else
            die "${var_name} is required but not set and --yes was passed."
        fi
        return
    fi

    local input
    if [[ -n "$default_val" ]]; then
        read -rp "${prompt_text} [${default_val}]: " input
        input="${input:-$default_val}"
    else
        read -rp "${prompt_text}: " input
    fi
    export "$var_name=$input"
}

validate_postgres() {
    local dsn="$1"
    log "Checking PostgreSQL connectivity..."
    if ! psql "$dsn" -c "SELECT 1" > /dev/null 2>&1; then
        die "Cannot connect to PostgreSQL with provided DSN."
    fi
}

validate_redis() {
    local url="$1"
    log "Checking Redis connectivity..."
    if ! redis-cli -u "$url" ping > /dev/null 2>&1; then
        die "Cannot connect to Redis with provided URL."
    fi
}

create_database() {
    local dsn="$1"
    log "Ensuring database '${DB_NAME}' and user '${DB_USER}' exist..."

    # Create database if not exists
    psql "$dsn" -c "SELECT 1 FROM pg_database WHERE datname = '${DB_NAME}'" \
        | grep -q 1 && log "Database already exists." || {
            log "Creating database ${DB_NAME}..."
            psql "$dsn" -c "CREATE DATABASE ${DB_NAME};" > /dev/null || die "Failed to create database."
        }

    # Create user if not exists
    psql "$dsn" -c "SELECT 1 FROM pg_roles WHERE rolname = '${DB_USER}'" \
        | grep -q 1 && log "User already exists." || {
            log "Creating user ${DB_USER}..."
            psql "$dsn" -c "CREATE USER ${DB_USER} WITH PASSWORD 'changeme';" > /dev/null || true
        }

    # Grant privileges
    psql "$dsn" -c "GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};" > /dev/null || true
}

create_system_user() {
    if id -u "$RUN_USER" &>/dev/null; then
        log "System user ${RUN_USER} already exists."
    else
        log "Creating system user ${RUN_USER}..."
        useradd --system --no-create-home --shell /usr/sbin/nologin "$RUN_USER"
    fi

    if getent group "$RUN_GROUP" &>/dev/null; then
        log "Group ${RUN_GROUP} already exists."
    else
        groupadd --system "$RUN_GROUP"
    fi
}

install_binaries() {
    log "Installing binaries to ${BIN_DIR}..."
    cp "${PROJECT_ROOT}/postnest-server" "${BIN_DIR}/postnest-server" || die "postnest-server binary not found. Build first: go build ./cmd/server"
    cp "${PROJECT_ROOT}/postnest-worker" "${BIN_DIR}/postnest-worker" || die "postnest-worker binary not found. Build first: go build ./cmd/worker"
    cp "${PROJECT_ROOT}/postnest-migrate" "${BIN_DIR}/postnest-migrate" || die "postnest-migrate binary not found. Build first: go build ./cmd/migrate"
    chmod +x "${BIN_DIR}/postnest-"*
}

write_config() {
    log "Writing config to ${CONFIG_PATH}..."
    mkdir -p "$(dirname "$CONFIG_PATH")"
    cat > "$CONFIG_PATH" <<EOF
config_version = 1

[server]
http_addr     = ":8080"
imap_addr     = ":143"
smtp_addr     = ":587"

[database]
dsn      = "${POSTNEST_DATABASE_DSN}"
read_dsn = "${POSTNEST_DATABASE_READ_DSN:-}"
max_conns = ${POSTNEST_DATABASE_MAX_CONNS:-25}

[redis]
url = "${POSTNEST_REDIS_URL}"

[postmark]
webhook_secret = "${POSTNEST_POSTMARK_WEBHOOK_SECRET:-}"

[tls]
cert_path = "${POSTNEST_TLS_CERT_PATH:-}"
key_path  = "${POSTNEST_TLS_KEY_PATH:-}"

[worker]
concurrency   = ${POSTNEST_WORKER_CONCURRENCY:-10}
poll_interval = "${POSTNEST_WORKER_POLL_INTERVAL:-5s}"

[security]
session_key         = "${POSTNEST_SECURITY_SESSION_KEY}"
session_expiry      = "${POSTNEST_SECURITY_SESSION_EXPIRY:-168h}"
argon2id_time       = ${POSTNEST_SECURITY_ARGON2ID_TIME:-3}
argon2id_memory     = ${POSTNEST_SECURITY_ARGON2ID_MEMORY:-65536}
argon2id_threads    = ${POSTNEST_SECURITY_ARGON2ID_THREADS:-4}
max_message_size    = ${POSTNEST_SECURITY_MAX_MESSAGE_SIZE:-52428800}
max_attachment_size = ${POSTNEST_SECURITY_MAX_ATTACHMENT_SIZE:-26214400}
EOF
    chmod 640 "$CONFIG_PATH"
    chown root:"$RUN_GROUP" "$CONFIG_PATH"
}

write_systemd_units() {
    log "Writing systemd units to ${SYSTEMD_DIR}..."

    cat > "${SYSTEMD_DIR}/postnest-server.service" <<EOF
[Unit]
Description=PostNest Server (IMAP + SMTP + HTTP + DAV)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_DIR}/postnest-server
Restart=on-failure
RestartSec=5
User=${RUN_USER}
Group=${RUN_GROUP}
Environment=POSTNEST_CONFIG_PATH=${CONFIG_PATH}

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/postnest

[Install]
WantedBy=multi-user.target
EOF

    cat > "${SYSTEMD_DIR}/postnest-worker.service" <<EOF
[Unit]
Description=PostNest Background Worker Pool
After=network-online.target postnest-server.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_DIR}/postnest-worker
Restart=on-failure
RestartSec=5
User=${RUN_USER}
Group=${RUN_GROUP}
Environment=POSTNEST_CONFIG_PATH=${CONFIG_PATH}

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
}

main() {
    require_root
    check_deps

    log "PostNest Systemd Installer"
    log "=========================="

    prompt_or_env POSTNEST_DATABASE_DSN "PostgreSQL DSN" "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
    prompt_or_env POSTNEST_REDIS_URL "Redis URL" "redis://localhost:6379/0"
    prompt_or_env POSTNEST_SECURITY_SESSION_KEY "Session signing key (32+ chars random)"
    prompt_or_env POSTNEST_POSTMARK_WEBHOOK_SECRET "Postmark webhook secret (optional)" ""

    validate_postgres "$POSTNEST_DATABASE_DSN"
    validate_redis "$POSTNEST_REDIS_URL"
    create_database "$POSTNEST_DATABASE_DSN"
    create_system_user

    mkdir -p /var/lib/postnest
    chown "${RUN_USER}:${RUN_GROUP}" /var/lib/postnest

    install_binaries
    write_config
    write_systemd_units

    systemctl daemon-reload
    systemctl enable postnest-server.service postnest-worker.service

    log ""
    log "==================================================="
    log "Installation complete."
    log ""
    log "NEXT STEPS:"
    log "  1. Run migrations:  postnest-migrate up"
    log "  2. Start services:   systemctl start postnest-server postnest-worker"
    log "  3. View logs:        journalctl -u postnest-server -f"
    log "  4. Health check:     curl http://localhost:8080/healthz"
    log "==================================================="
}

main "$@"
