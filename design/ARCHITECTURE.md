# System Architecture — Go-PostNest Postmark Mail Platform

## 1. Overview

Go-PostNest is a multi-tenant mail platform that uses Postmark for inbound and outbound email transport. It exposes standard mail protocols (IMAP4, SMTP) alongside modern webmail and DAV services, storing all mail, contacts, and metadata in PostgreSQL.

## 2. High-Level Topology

```mermaid
flowchart TB
    subgraph Clients
        A[Email Clients<br/>Thunderbird, Apple Mail]
        B[Webmail UI]
        C[DAV Clients<br/>macOS, iOS, Android]
    end

    subgraph "Go-PostNest Platform"
        D[IMAP Server<br/>internal/imap]
        E[SMTP Proxy<br/>internal/smtp]
        F[Webmail API<br/>internal/webmail]
        G[DAV Server<br/>internal/dav]
        H[Webhook Receiver<br/>internal/webhook]
        I[Background Workers<br/>internal/workers]

        subgraph "Shared Services"
            J[Auth Service<br/>internal/auth]
            K[Mail Store<br/>internal/mailstore]
            L[Search Engine<br/>internal/search]
            M[Reputation Engine<br/>internal/reputation]
            N[Postmark Client<br/>internal/postmark]
        end
    end

    subgraph "External"
        O[Postmark API]
        P[PostgreSQL]
        Q[Redis]
    end

    A -->|IMAP4rev1:143/993| D
    B -->|HTTP/REST:8080| F
    C -->|HTTP/DAV:8080| G
    A -->|SMTP:587/25| E

    D --> K
    E --> K
    E --> N
    F --> K
    F --> J
    G --> K
    H --> I
    I --> K
    I --> M
    I --> L
    I --> N

    K --> P
    J --> P
    L --> P
    M --> P
    N --> O
    I --> Q
```

## 3. Service Boundaries

### 3.1 IMAP Server (`internal/imap`)
- **Responsibility**: IMAP4rev1 protocol implementation for mail client access.
- **Capabilities**: IDLE, MOVE, ACL, THREAD, SORT, QUOTA, QRESYNC, CONDSTORE, UIDPLUS.
- **Dependencies**: `mailstore`, `auth`, `search`.
- **State**: Connection-per-client; no shared session state. Uses `auth` for credential validation, `mailstore` for mailbox operations.
- **Concurrency**: Goroutine per connection; mailbox updates broadcast to IDLE clients via Redis pub/sub.

### 3.2 SMTP Proxy (`internal/smtp`)
- **Responsibility**: Accept SMTP submissions from clients, authenticate, relay to Postmark.
- **Modes**:
  - **Inbound**: Receives mail from Postmark inbound webhook (handled by `webhook` receiver).
  - **Outbound**: Accepts SMTP AUTH + MAIL FROM / RCPT TO / DATA, validates sender, injects into `mailstore` as outgoing draft, immediately relays via Postmark API.
- **Dependencies**: `auth`, `mailstore`, `postmark`.
- **Security**: STARTTLS on 587, TLS on 465. Argon2id password verification via `auth`.

### 3.3 Webmail API (`internal/webmail`)
- **Responsibility**: REST API powering the Gmail-style web UI.
- **Dependencies**: `mailstore`, `auth`, `search`, `contacts`.
- **Features**: Inbox listing, compose/send, thread view, label management, contact CRUD, calendar events, full-text search.
- **Authentication**: Session cookies (CSRF protected) or Bearer tokens for API access.

### 3.4 DAV Server (`internal/dav`)
- **Responsibility**: HTTP-based DAV protocol endpoints.
- **Protocols**: CardDAV (contacts), CalDAV (calendar), WebDAV (file/mail attachments).
- **Dependencies**: `mailstore`, `contacts`, `auth`.
- **Routing**: `/.well-known/carddav`, `/.well-known/caldav`, `/dav/...`.

### 3.5 Webhook Receiver (`internal/webhook`)
- **Responsibility**: HTTP endpoints receiving Postmark webhooks.
- **Events**: Bounce, delivery, open, click, spam complaint, inbound email.
- **Processing**: Validates Postmark signature, enqueues to Redis for background workers.
- **Dependencies**: Redis.

### 3.6 Background Workers (`internal/workers`)
- **Responsibility**: Async job processing via Redis-backed queue.
- **Workers**:
  1. **Webhook Processor**: Parses inbound emails, writes to `mailstore`, triggers spam evaluation.
  2. **Bounce Processor**: Updates delivery logs, increments reputation scores.
  3. **Delivery Processor**: Polls Postmark delivery status, updates `delivery_logs`.
  4. **Reputation Updater**: Recalculates contact reputation from event history.
  5. **Spam Evaluator**: Runs greylist/blacklist/whitelist rules on inbound messages.
  6. **Search Updater**: Maintains PostgreSQL `tsvector` indexes asynchronously.
  7. **Mailbox Synchronizer**: Reconciles Postmark inbound stream with local mailstore.

### 3.7 Auth Service (`internal/auth`)
- **Responsibility**: Identity verification, session management, password hashing.
- **Storage**: PostgreSQL (`users`, `auth_sessions`, `domain_members`).
- **Algorithms**: Argon2id for passwords, bcrypt for API keys, secure random tokens for sessions.
- **Multi-tenancy**: Users belong to one or more domains via `domain_members`.

### 3.8 Mail Store (`internal/mailstore`)
- **Responsibility**: Canonical abstraction for all mail persistence.
- **Storage**: PostgreSQL (`messages`, `labels`, `message_labels`, `attachments`, `message_flags`, `threads`).
- **Features**:
  - Gmail-style labels (many-to-many with messages).
  - RFC822 source stored as exact `bytea` in `messages.source`.
  - Attachments stored as PostgreSQL `bytea` in `attachments.data`.
  - Threading via `threads` table with `message_id` / `in_reply_to` / `references` correlation.
  - Flags: `\Seen`, `\Answered`, `\Flagged`, `\Deleted`, `\Draft`, custom flags.

### 3.9 Search Engine (`internal/search`)
- **Responsibility**: Full-text query execution.
- **Backend**: PostgreSQL `tsvector` on `messages.subject`, `messages.plain_text`, `messages.from_address`, etc.
- **Features**: Webmail search bar, IMAP SEARCH command delegation.
- **Async**: `search updater` worker refreshes `tsvector` after message ingestion.

### 3.10 Reputation Engine (`internal/reputation`)
- **Responsibility**: Contact scoring and spam rule evaluation.
- **Storage**: PostgreSQL (`contact_reputation`, `whitelist`, `greylist`, `blacklist`).
- **Rules**:
  - Whitelist: immediate inbox delivery.
  - Blacklist: immediate discard / junk folder.
  - Greylist: temporary deferral for unknown senders (triple-check pattern).

### 3.11 Postmark Client (`internal/postmark`)
- **Responsibility**: Outbound API calls to Postmark.
- **Features**:
  - Send email with template or raw body.
  - Parse inbound webhook payloads.
  - Poll message streams for delivery status.
- **Configuration**: Per-domain API token stored in `domains.postmark_token`.

## 4. Data Flow

### 4.1 Outbound Mail (Client → Postmark)

```mermaid
sequenceDiagram
    participant C as Email Client
    participant S as SMTP Proxy
    participant A as Auth
    participant M as Mailstore
    participant P as Postmark Client
    participant O as Postmark API

    C->>S: EHLO, AUTH LOGIN
    S->>A: Validate credentials
    A-->>S: OK
    C->>S: MAIL FROM, RCPT TO, DATA
    S->>M: Create message (draft, outbox label)
    S->>P: Relay via Postmark API
    P->>O: POST /email
    O-->>P: MessageID + Accepted
    P->>M: Update delivery_log (pending)
    S-->>C: 250 OK
```

### 4.2 Inbound Mail (Postmark → Client)

```mermaid
sequenceDiagram
    participant O as Postmark
    participant H as Webhook Receiver
    participant R as Redis Queue
    participant W as Webhook Worker
    participant M as Mailstore
    participant L as Search Updater
    participant C as IMAP Client

    O->>H: POST /webhook/inbound
    H->>R: Enqueue inbound job
    H-->>O: 200 OK
    W->>R: Dequeue job
    W->>M: Parse RFC822, create message + attachments + thread
    W->>L: Enqueue search index job
    W->>M: Apply spam rules (greylist/blacklist/whitelist)
    M-->>C: NOTIFY via IDLE / Redis pub/sub
```

### 4.3 Webmail Read

```mermaid
sequenceDiagram
    participant B as Browser
    participant F as Webmail API
    participant A as Auth
    participant M as Mailstore
    participant S as Search

    B->>F: GET /api/v1/mailbox/INBOX?limit=50
    F->>A: Validate session
    A-->>F: User + Domain context
    F->>M: List messages by label (INBOX)
    F->>M: Count unread per label
    F-->>B: JSON: threads, labels, counts
```

## 5. Communication Patterns

| Layer | Protocol | Transport | State |
|---|---|---|---|
| IMAP | IMAP4rev1 | TCP (143/993) | Per-connection |
| SMTP | SMTP + STARTTLS | TCP (587/25) | Per-connection |
| Webmail | HTTP/REST + JSON | TCP (8080/443) | Session cookie / Bearer |
| DAV | HTTP + WebDAV/CalDAV/CardDAV | TCP (8080/443) | Basic / Bearer |
| Webhooks | HTTP + JSON | TCP (8080/443) | Stateless |
| Internal | Go interfaces | In-process | Stateless |
| Workers | Redis List / Stream | TCP | Job state in Redis |
| IDLE Broadcast | Redis Pub/Sub | TCP | Event-driven |

## 6. Multi-Tenancy Model

- **Domain**: Top-level entity. Each domain has a Postmark token, DNS settings, and spam rule overrides.
- **User**: Belongs to one or more domains via `domain_members`.
- **Mailbox**: Per-user. Messages are scoped by `user_id`; IMAP login selects the user's mailbox.
- **Cross-domain restrictions**: Users can only see messages, contacts, and settings for domains they are members of. Admin users have cross-domain visibility for the admin panel.

## 7. Scalability Considerations

- **Horizontal scaling**: IMAP/SMTP/Webmail services are stateless except for TCP connections. Multiple instances behind a TCP/HTTP load balancer.
- **Database**: PostgreSQL with read replicas for IMAP SEARCH and Webmail listing. Write load is moderate (one write per inbound/outbound message).
- **Workers**: Horizontally scalable by adding worker processes. Redis-backed queue ensures at-least-once delivery.
- **Attachments**: Stored as `bytea` in PostgreSQL. For >10MB average size, consider migrating to S3 with `attachments.storage_url` reference.
- **Search**: `tsvector` is updated asynchronously to avoid write latency on inbound delivery.

## 8. Deployment Architecture

```mermaid
flowchart TB
    subgraph "Docker Compose Stack"
        LB[NGINX / Traefik<br/>TLS termination]
        APP1[App Instance 1<br/>IMAP+SMTP+Web+DAV]
        APP2[App Instance 2<br/>IMAP+SMTP+Web+DAV]
        WORKER[Worker Pool<br/>7 worker types]
        DB[(PostgreSQL<br/>Primary)]
        DBR[(PostgreSQL<br/>Read Replica)]
        REDIS[(Redis<br/>Queue + Pub/Sub)]
    end

    LB --> APP1
    LB --> APP2
    APP1 --> DB
    APP2 --> DB
    APP1 --> REDIS
    APP2 --> REDIS
    WORKER --> DB
    WORKER --> REDIS
    DB --> DBR
```

### 8.1 Service Configuration

| Service | Image | Ports | Notes |
|---|---|---|---|
| app | Built from `cmd/server` | 8080, 143, 587, 465 | Can run all protocols or be split |
| worker | Built from `cmd/worker` | — | Reads Redis, writes PostgreSQL |
| postgres | `postgres:16-alpine` | 5432 | Primary + replica |
| redis | `redis:7-alpine` | 6379 | Queue + cache + pub/sub |

### 8.2 Environment Variables

```
POSTGRES_DSN=postgres://user:pass@postgres:5432/postnest?sslmode=disable
POSTGRES_READ_DSN=postgres://user:pass@postgres-replica:5432/postnest?sslmode=disable
REDIS_URL=redis://redis:6379/0
POSTMARK_WEBHOOK_SECRET=...
IMAP_TLS_CERT=/etc/ssl/certs/imap.crt
IMAP_TLS_KEY=/etc/ssl/certs/imap.key
SMTP_TLS_CERT=/etc/ssl/certs/smtp.crt
SMTP_TLS_KEY=/etc/ssl/certs/smtp.key
ARGON2ID_TIME=3
ARGON2ID_MEMORY=65536
SESSION_KEY=...
```

### 8.3 Nix Support

A `flake.nix` provides reproducible builds and development shells:

- **Packages**: `go-postnest-server` and `go-postnest-worker` built with `buildGoModule`.
- **Dev shell**: Includes `go`, `postgres`, `redis`, `golangci-lint`, and `air` for live reload.
- **Module**: `nixosModules.go-postnest` exposes systemd services for server + worker, plus `services.postgresql` and `services.redis` integration.
- **Container image**: `pkgs.dockerTools.buildLayeredImage` produces OCI images without Docker daemon dependency.

This enables deployment on NixOS hosts, traditional Linux with Nix package manager, or as a hermetic build step in CI.

## 9. Security Model

- **Transport**: TLS 1.3 for all external-facing TCP (IMAPS, SMTPS, HTTPS). STARTTLS supported for IMAP/SMTP on plain ports.
- **Authentication**: Argon2id for passwords. Session tokens rotated on privilege change. API keys scoped per-domain.
- **Authorization**: Domain-scoped RBAC. Roles: `admin`, `user`, `readonly`.
- **Webhook Validation**: HMAC-SHA256 of Postmark payload against shared secret.
- **Storage**: RFC822 source and attachments stored as `bytea`. At-rest encryption delegated to PostgreSQL/file system.

## 10. Error Handling & Observability

- **Logging**: Structured JSON logging (slog). Per-request trace IDs propagated via context.
- **Metrics**: Prometheus counters for IMAP commands, SMTP transactions, API requests, worker job durations.
- **Health Checks**: `/healthz` (HTTP), `NOOP` (SMTP), `NOOP` (IMAP) for load balancer probes.
- **Retries**: Postmark API calls use exponential backoff (max 5 retries). Worker jobs retry 3 times before dead-letter queue.
