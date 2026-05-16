# Integration Notes — Go-PostNest

## Status

All packages compile with `go build ./...`. The project is ready for iterative development.

## Implemented Packages

| Package | Status | Notes |
|---|---|---|
| `internal/config` | Complete | Env-based configuration with defaults and validation |
| `internal/models` | Complete | Pure data structs using `github.com/google/uuid` |
| `internal/logger` | Complete | JSON `slog` wrapper |
| `internal/db` | Complete | `pgxpool` wrapper with ping check |
| `internal/redis` | Complete | `go-redis/v9` wrapper with enqueue/dequeue helpers |
| `internal/mailstore` | Complete | PostgreSQL implementation of full `Store` interface |
| `internal/auth` | Complete | Argon2id hashing, session/API-key management |
| `internal/api` | Complete | RequestID, logger, recovery, CORS, session auth, domain admin middleware, unified errors |
| `internal/postmark` | Complete | HTTP client for outbound send, inbound webhook parsing |
| `internal/webhook` | Complete | Postmark webhook receiver (inbound, bounce, delivery, spam) enqueues proper Job structs to `queue:jobs` |
| `internal/webmail` | Complete | REST handlers for labels, messages, threads, drafts, batch ops, search |
| `internal/search` | Complete | `tsvector` update helper (synchronous for now) |
| `internal/contacts` | Complete | PostgreSQL contact store with upsert |
| `internal/reputation` | Complete | Whitelist/blacklist/greylist evaluation + reputation scoring |
| `internal/workers` | Complete | Redis-backed worker pool with retry logic; registered processors: inbound, bounce, delivery |
| `internal/imap` | Complete | go-imap server with LOGIN, LIST, STATUS, FETCH, SEARCH, APPEND, EXPUNGE, COPY, flag updates |
| `internal/smtp` | Complete | go-smtp server with AUTH PLAIN, DATA relay to Postmark via go-message parsing, Sent copy persistence, TLS support |
| `internal/dav` | Partial | go-webdav CardDAV backend (list/get/put/delete contacts as vCards); CalDAV stub |
| `cmd/server` | Wired | HTTP + IMAP + SMTP + DAV routes; health check with DB/Redis; graceful shutdown for all services; TLS config |
| `cmd/worker` | Wired | Worker pool startup with registered processors; graceful shutdown |

## Database Setup

1. Start PostgreSQL 16+ and Redis.
2. Run migrations:
   ```bash
   # Using golang-migrate
   migrate -path migrations -database "$POSTGRES_DSN" up
   ```
   Or execute `V1__init.sql`, `V2__fts.sql`, `V3__seed_labels.sql` manually.

## Running

```bash
# Server
POSTGRES_DSN=postgres://user:pass@localhost:5432/postnest?sslmode=disable \
REDIS_URL=redis://localhost:6379/0 \
SESSION_KEY=changeme \
go run ./cmd/server

# Worker
POSTGRES_DSN=postgres://user:pass@localhost:5432/postnest?sslmode=disable \
REDIS_URL=redis://localhost:6379/0 \
SESSION_KEY=changeme \
go run ./cmd/worker
```

## TLS Configuration

Set environment variables to enable TLS on IMAP and SMTP:

```bash
TLS_CERT_PATH=/path/to/cert.pem \
TLS_KEY_PATH=/path/to/key.pem \
go run ./cmd/server
```

## Production Readiness Checklist

- [x] Worker processors (inbound, bounce, delivery)
- [x] TLS configuration for IMAP/SMTP
- [x] Health check endpoint (`/healthz`) with DB/Redis verification
- [x] Graceful shutdown for HTTP, IMAP, SMTP
- [x] IMAP APPEND, EXPUNGE, COPY, flag updates
- [x] Webhook handler enqueues properly formatted jobs
	- [x] Systemd service integration
- [ ] Admin REST handlers
- [ ] Frontend UI
- [ ] Comprehensive tests
- [ ] CalDAV implementation
- [ ] STARTTLS on IMAP/SMTP

## Next Implementation Priorities

1. **Admin REST handlers** (`internal/webmail` admin group)
2. **Frontend** — Gmail-style web UI consuming the REST API
3. **Tests** — unit + integration tests for all store layers
4. **CalDAV** — implement calendar backend and `calendar_events` table
5. **Systemd service** — complete `kardianos/service` integration
6. **STARTTLS** — automatic TLS negotiation on IMAP/SMTP
7. **SMTP LOGIN auth** — support LOGIN mechanism in addition to PLAIN

## Known Limitations

- Domain scoping in webmail handlers currently uses `user.ID` as a placeholder for `domainID`; replace with actual domain context from `X-Domain-ID` header or `domain_members` lookup
- `imap_uids` table is defined but not populated by `mailstore`; UIDs are derived deterministically from message UUIDs
- `messages_update_search_vector()` trigger is created but workers use direct SQL updates instead of the trigger
- Greylist deferral / retry logic is simplified (no background delay enforced)
- CalDAV events table (`calendar_events`) is not defined; full calendar UI scope TBD
- SMTP only supports PLAIN auth; LOGIN not yet implemented
- CardDAV only supports a single default address book
- DAV auth uses Basic Auth only (no Bearer token support yet)
