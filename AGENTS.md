# Go-PostNest Agent Instructions

This file defines mandatory development rules and architectural constraints for all AI agents and contributors working on Go-PostNest.

Follow these instructions exactly.

Violation of these rules creates architecture drift and inconsistent implementations.

---

## Project Goal

Build a self-hosted Gmail-style mail platform using:

- Postmark for mail transport
- PostgreSQL as primary datastore
- IMAP4-compatible persistence
- SMTP proxy
- Webmail
- CardDAV
- CalDAV
- WebDAV

This project intentionally avoids operating a traditional mail server stack.

Postmark handles mail transport.

Go-PostNest handles storage, mailbox logic, protocols, UI, and synchronization.

---

## Architecture Rules

The system architecture is:

```text
Client
   ↓
IMAP / SMTP / Webmail / DAV
   ↓
Go Services
   ↓
Postmark + PostgreSQL + Redis
```

Inbound:

```text
Postmark
→ Webhook
→ MIME Parse
→ PostgreSQL
→ Mailbox
→ IMAP/Webmail
```

Outbound:

```text
SMTP/Webmail
→ RFC822 generation
→ Persist Sent item
→ Postmark Send API
→ Delivery events
```

Do not redesign this architecture.

---

## Approved Libraries

Agents MUST use the following libraries.

Do not replace them unless explicitly instructed.

### Postmark

Required:

```go
github.com/mrz1836/postmark
```

Use for:

- outbound sending
- inbound parsing
- bounce processing
- event handling
- templates
- streams

Do NOT create custom Postmark HTTP clients.

Do NOT replace with:

- net/http wrappers
- alternative SDKs
- handwritten REST clients

---

### IMAP

Required:

```go
github.com/emersion/go-imap
```

Use for:

- IMAP4rev1 implementation
- mailbox interfaces
- message fetch
- IDLE
- QRESYNC
- UID handling

Do NOT implement IMAP manually.

Do NOT create a custom IMAP parser.

---

### SMTP

Required:

```go
github.com/emersion/go-smtp
```

Use for:

- SMTP AUTH
- submission server
- STARTTLS
- DATA handling

SMTP only proxies mail to Postmark.

Do not implement SMTP manually.

---

### MIME Parsing

Required:

```go
github.com/emersion/go-message
```

Use for:

- RFC822 parsing
- MIME
- multipart
- attachments
- inline images

Do not manually parse MIME structures.

Preserve exact RFC822 source.

Store:

```go
messages.raw_message BYTEA
```

without modification.

---

### DAV

Required:

```go
github.com/emersion/go-webdav
```

Use for:

- WebDAV
- CardDAV
- CalDAV

Do not build DAV XML handling manually.

---

### Database

Required:

```go
github.com/jackc/pgx/v5
```

Use:

```go
pgxpool
```

Do not use:

- GORM
- sqlx
- ent
- xorm

This project uses explicit SQL.

---

### Redis

Required:

```go
github.com/redis/go-redis/v9
```

Use only for:

- worker queues
- background jobs
- retry workflows

Redis is not primary storage.

---

### Passwords

Required:

```go
golang.org/x/crypto/argon2
```

Use:

Argon2id

Never use:

- bcrypt
- md5
- sha1
- plain hashes

---

### Service Management

Required:

```go
github.com/kardianos/service
```

Use for:

- running the server as a system service
- cross-platform service lifecycle (install, start, stop, uninstall)

Do not write custom service wrappers.

---

## Required APIs

Postmark Inbound:

https://postmarkapp.com/developer/webhooks/inbound-webhook

Postmark Webhooks:
https://postmarkapp.com/developer/webhooks/webhooks-overview

Postmark Email API:

https://postmarkapp.com/developer/api/email-api

Agents must follow official request/response payloads.

Do not invent fields.

---

## Mail Storage Rules

Messages are immutable.

Always preserve:

```go
raw RFC822
```

Store:

- headers
- attachments
- MIME structure
- threading metadata

Never rewrite original message bodies.

---

## Mailbox Rules

This is NOT folder ownership architecture.

Required model:

```text
messages
labels
message_labels
```

A message may belong to multiple labels.

Examples:

Inbox
Archive
Starred
Important

This is required for Gmail-like behavior.

Do not redesign to:

```text
messages.mailbox_id
```

---

## IMAP Requirements

Support:

- IMAP4rev1
- IDLE
- MOVE
- ACL
- THREAD
- SORT
- QUOTA
- QRESYNC
- CONDSTORE
- UIDPLUS
- NAMESPACE

Persist:

- UID
- UIDVALIDITY
- MODSEQ

Maintain protocol correctness.

---

## SMTP Rules

SMTP authentication is separate from Postmark credentials.

Flow:

```text
authenticate
→ create Sent message
→ persist RFC822
→ send via Postmark
→ log delivery
```

Sent items must be stored before outbound delivery.

---

## Spam Rules

Use Postmark headers:

```text
X-Spam-Status
X-Spam-Score
```

Support:

Whitelist

Greylist

Blacklist

Do not integrate external spam engines unless requested.

---

## Search Rules

Use PostgreSQL:

```sql
tsvector
GIN indexes
```

Do not introduce:

- Elasticsearch
- Meilisearch
- Solr

Searches:

- subject
- sender
- body
- recipients
- attachments

---

## Authentication Rules

Accounts:

Local PostgreSQL users

Password hashing:

Argon2id

Roles:

Admin

User

Users may belong to multiple domains.

---

## Web UI Rules

Framework:

Gin

UI style:

Gmail-inspired

Features:

- threaded conversations
- rich compose editor
- autosave drafts
- labels
- drag/drop attachments
- keyboard shortcuts

Do not introduce React unless requested.

Prefer:

- Gin templates
- HTMX
- Tailwind

---

## Worker Rules

Workers process:

1. inbound mail
2. bounce events
3. delivery events
4. reputation updates
5. search indexing
6. mailbox synchronization
7. attachment dedupe

Workers must be idempotent.

---

## Testing Requirements

All new code should include:

- unit tests
- integration tests when applicable

Protocol changes require:

- IMAP tests
- SMTP tests

---

## Code Rules

Prefer:

- small packages
- interfaces where useful
- explicit SQL
- context.Context
- structured logging

Avoid:

- global state
- hidden magic
- reflection-heavy frameworks
- ORM abstractions

---

## Before Creating New Dependencies

Ask:

1. Does existing approved library already solve this?

2. Can existing architecture support this?

3. Does this introduce stack drift?

If yes:

Do not add dependency.

---

## Source of Truth

Architecture documents:

- PLAN.md
- INTEGRATION.md
- AGENTS.md

When conflicts exist:

AGENTS.md overrides implementation assumptions.
PLAN.md overrides feature intent.
INTEGRATION.md overrides package wiring.

Follow them in that order.
