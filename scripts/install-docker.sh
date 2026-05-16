#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "$PROJECT_ROOT"

log() { echo "[postnest-docker] $*"; }
die() { echo "[postnest-docker] ERROR: $*" >&2; exit 1; }

# 1. Check docker and docker compose
command -v docker >/dev/null || die "docker not found"
docker compose version >/dev/null 2>&1 || die "docker compose not found"

# 2. Create .env if missing
if [[ ! -f .env ]]; then
    if [[ -f .env.example ]]; then
        cp .env.example .env
        log "Created .env from .env.example."
        log "Please edit .env and set at least POSTNEST_SECURITY_SESSION_KEY and POSTNEST_POSTMARK_WEBHOOK_SECRET."
        log "Then re-run this script."
        exit 0
    else
        die ".env.example not found. Cannot create .env."
    fi
fi

# 3. Build and start infrastructure
log "Starting PostgreSQL and Redis..."
docker compose up -d postgres redis

# 4. Wait for healthy
log "Waiting for services to be healthy..."
sleep 5

# 5. Run migrations
log "Running migrations..."
docker compose run --rm migrate up

# 6. Start app
log "Starting PostNest server and worker..."
docker compose up -d server worker

log ""
log "==================================================="
log "PostNest Docker stack is running."
log ""
log "  Health check:  curl http://localhost:8080/healthz"
log "  Logs:          docker compose logs -f server"
log "  Stop:          docker compose down"
log "  Migrations:    docker compose run --rm migrate up"
log "==================================================="
