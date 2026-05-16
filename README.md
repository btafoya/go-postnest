# PostNest

![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)
![Go Report Card](https://goreportcard.com/badge/github.com/go-postnest/postnest)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-336791?logo=postgresql)
![Redis](https://img.shields.io/badge/Redis-7+-DC382D?logo=redis)

> A Go-based, multi-tenant mail platform built around Postmark for inbound/outbound transport. PostNest exposes IMAP4, SMTP, a Gmail-style REST webmail API, CardDAV/CalDAV, and a reputation-aware contact system — all backed by PostgreSQL and Redis.

## Overview

PostNest is a self-hostable mail server and webmail platform that uses [Postmark](https://postmarkapp.com) as its email transport layer. It stores mail, contacts, and metadata in PostgreSQL, uses Redis for background job queues and IMAP IDLE pub/sub, and provides standard email protocols alongside a modern REST API.

### Architecture

```
Client
   ↓
IMAP / SMTP / Webmail REST / CardDAV / CalDAV
   ↓
Go Services (cmd/server + cmd/worker)
   ↓
Postmark API  +  PostgreSQL  +  Redis
```

- **Inbound flow**: Postmark → Webhook → MIME parse → PostgreSQL → Mailbox (IMAP/Webmail)
- **Outbound flow**: SMTP/Webmail → RFC822 generation → Persist Sent item → Postmark Send API → Delivery events

## Features

- **IMAP4rev1 Server** — Full mailbox management with IDLE, SEARCH, FETCH, APPEND, EXPUNGE, COPY, and flag updates.
- **SMTP Proxy** — AUTH PLAIN submission with immediate foreground relay to Postmark, plus TLS support.
- **Gmail-Style Webmail API** — Labels, threads, drafts (autosave), batch operations, and full-text search via PostgreSQL `tsvector`.
- **CardDAV / CalDAV / WebDAV** — Contact sync via vCard 4.0; CalDAV stub included.
- **Multi-Tenant Domains** — Users can belong to multiple domains with role-based access.
- **Reputation System** — Whitelist, blacklist, and greylist evaluation with per-contact reputation scoring.
- **Background Workers** — Redis-backed worker pool for inbound processing, bounces, and delivery tracking.
- **Webhook Processing** — Native Postmark inbound, bounce, delivery, and spam complaint handlers.
- **Secure by Default** — Argon2id password hashing, session & API-key auth, CORS, rate limiting, and recovery middleware.

## Quick Start

### Prerequisites

- Go 1.25+
- PostgreSQL 16+
- Redis 7+
- Postmark account (for email transport)

### Database Setup

Start PostgreSQL and Redis, then run migrations:

```bash
# Using golang-migrate
migrate -path migrations -database "$POSTGRES_DSN" up

# Or run the SQL files manually
psql "$POSTGRES_DSN" -f migrations/V1__init.sql
psql "$POSTGRES_DSN" -f migrations/V2__fts.sql
psql "$POSTGRES_DSN" -f migrations/V3__seed_labels.sql
```

### Running the Server

The server exposes HTTP (REST + webhooks), IMAP, SMTP, and DAV:

```bash
export POSTGRES_DSN="postgres://user:pass@localhost:5432/postnest?sslmode=disable"
export REDIS_URL="redis://localhost:6379/0"
export SESSION_KEY="change-me-in-production"
export POSTMARK_WEBHOOK_SECRET="your-postmark-secret"

go run ./cmd/server
```

### Running the Worker

Workers process background jobs from Redis:

```bash
export POSTGRES_DSN="postgres://user:pass@localhost:5432/postnest?sslmode=disable"
export REDIS_URL="redis://localhost:6379/0"
export SESSION_KEY="change-me-in-production"

go run ./cmd/worker
```

### TLS (Optional)

Provide certificate paths to enable TLS on IMAP and SMTP:

```bash
export TLS_CERT_PATH="/path/to/cert.pem"
export TLS_KEY_PATH="/path/to/key.pem"
go run ./cmd/server
```

## Configuration

All configuration is environment-driven. Key variables:

| Variable | Default | Description |
|---|---|---|
| `HTTP_ADDR` | `:8080` | HTTP REST API address |
| `IMAP_ADDR` | `:143` | IMAP server address |
| `SMTP_ADDR` | `:587` | SMTP submission address |
| `POSTGRES_DSN` | — | PostgreSQL connection string |
| `POSTGRES_READ_DSN` | — | Optional read-replica DSN |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection URL |
| `SESSION_KEY` | — | Secret key for session signing |
| `SESSION_EXPIRY` | `168h` | Session duration |
| `POSTMARK_WEBHOOK_SECRET` | — | Postmark webhook signature secret |
| `WORKER_CONCURRENCY` | `10` | Number of concurrent worker goroutines |
| `WORKER_POLL_INTERVAL` | `5s` | Redis job polling interval |
| `MAX_MESSAGE_SIZE` | `52428800` (50MB) | Maximum incoming message size |
| `MAX_ATTACHMENT_SIZE` | `26214400` (25MB) | Maximum attachment size |
| `TLS_CERT_PATH` | — | Path to TLS certificate |
| `TLS_KEY_PATH` | — | Path to TLS private key |

## API

PostNest provides a Gmail-style REST API for webmail clients:

- **Messages** — List by label, thread view, send, draft autosave, batch archive/spam/delete.
- **Threads** — Grouped conversation view with participant lists.
- **Search** — Full-text search across subjects, bodies, from/to addresses, with date and attachment filters.
- **Contacts** — CRUD with vCard import/export.
- **Admin** — Domain management, user provisioning, delivery logs, webhook events, and reputation dashboards.

See [`design/API-SPEC.md`](design/API-SPEC.md) for the complete endpoint specification.

## Protocol Support

| Protocol | Status | Notes |
|---|---|---|
| IMAP4rev1 | ✅ Supported | LOGIN, LIST, STATUS, FETCH, SEARCH, APPEND, EXPUNGE, COPY, IDLE |
| SMTP | ✅ Supported | AUTH PLAIN, immediate Postmark relay, TLS |
| CardDAV | ✅ Supported | vCard 4.0 list/get/put/delete |
| CalDAV | 🚧 Partial | Read-only stub; full calendar events TBD |
| WebDAV | 🚧 Partial | File storage not yet implemented |

## Workers

Registered background processors:

1. **Inbound** — Parses Postmark inbound webhooks and stores messages/attachments.
2. **Bounce** — Processes bounce events and updates delivery logs.
3. **Delivery** — Tracks delivery confirmations and updates message status.

Additional workers (reputation updater, spam evaluator, search indexer, mailbox sync) are planned. See [`design/COMPONENT-DESIGN.md`](design/COMPONENT-DESIGN.md) for the worker architecture.

## Deployment

PostNest supports containerized and native deployments:

- **Docker / Docker Compose** — See [`design/ARCHITECTURE.md`](design/ARCHITECTURE.md) §8 for service topology.
- **Nix** — Flake and `nixosModules` are planned.
- **Systemd** — Service integration via `kardianos/service` is planned.

Services required:

- `app` — HTTP + IMAP + SMTP + DAV
- `worker` — Background job processors
- `postgres` — Primary datastore
- `redis` — Job queue + IMAP IDLE pub/sub

## Project Structure

```
cmd/
  server/          # HTTP, IMAP, SMTP, DAV server entrypoint
  worker/          # Background worker pool entrypoint

internal/
  api/             # Shared middleware (auth, CORS, JSON, rate limit, errors)
  auth/            # Argon2id hashing, session & API-key management
  config/          # Environment-based configuration
  contacts/        # Contact store (PostgreSQL) with upsert
  dav/             # WebDAV/CalDAV/CardDAV handlers
  db/              # PostgreSQL connection pool wrapper
  imap/            # IMAP4rev1 server implementation
  logger/          # Structured JSON logging (slog)
  mailstore/       # Canonical mail storage abstraction + PostgreSQL implementation
  models/          # Pure data structures (Message, User, Label, Thread, etc.)
  postmark/        # Postmark HTTP client + webhook parser
  redis/           # Redis client with enqueue/dequeue helpers
  reputation/      # Whitelist/blacklist/greylist + scoring engine
  search/          # PostgreSQL tsvector update helpers
  smtp/            # SMTP submission server + immediate Postmark relay
  webmail/         # REST handlers for labels, messages, threads, drafts
  webhook/         # Postmark webhook receiver (inbound, bounce, delivery, spam)
  workers/         # Redis-backed worker pool with retry logic

migrations/        # Flyway-style SQL migrations (V1, V2, V3)
design/            # Architecture & design documents
```

## Design Documentation

PostNest was designed with comprehensive architecture documents:

- [`design/ARCHITECTURE.md`](design/ARCHITECTURE.md) — System topology, data flow, deployment models
- [`design/API-SPEC.md`](design/API-SPEC.md) — REST endpoints, webhook contracts, auth modes
- [`design/DATABASE-SCHEMA.md`](design/DATABASE-SCHEMA.md) — PostgreSQL schema, indexes, FTS design
- [`design/PROTOCOL-DESIGN.md`](design/PROTOCOL-DESIGN.md) — IMAP, SMTP, DAV protocol details
- [`design/COMPONENT-DESIGN.md`](design/COMPONENT-DESIGN.md) — Package layout, interfaces, implementation order
- [`INTEGRATION.md`](INTEGRATION.md) — Current implementation status and known limitations

## Contributing

Contributions are welcome. Please review the design documents before proposing structural changes, and ensure your code compiles with `go build ./...`.

## License

This project is open source. A `LICENSE` file will be added shortly.
