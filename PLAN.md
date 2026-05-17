# Go-PostNest Postmark Mail Platform

## Overview

Build a Go-based mail platform using Postmark for inbound/outbound
transport while exposing:

-   IMAP4 server
-   SMTP proxy
-   Gmail-style webmail
-   CardDAV / CalDAV / WebDAV
-   Multi-tenant domain support
-   PostgreSQL mail storage
-   Contacts + reputation system

## Core Decisions

-   Local accounts stored in PostgreSQL
-   Argon2id password hashing
-   Gmail-style labels + folders
-   Multi-domain users supported
-   Rich HTML editor
-   Gmail-style autosave drafts
-   PostgreSQL full text search (tsvector)
-   Full DAV support
-   Store exact RFC822 source permanently
-   Attachments stored as PostgreSQL bytea
-   Immediate SMTP relay to Postmark
-   Webhook processing for bounce, delivery, retries, complaints,
    failures

## Architecture

```text
Client
   ↓
IMAP / SMTP / Webmail / DAV
   ↓
Go Services
   ↓
Postmark + PostgreSQL + Redis
```

### Inbound Flow

```text
Postmark
→ Webhook
→ MIME Parse
→ PostgreSQL
→ Mailbox
→ IMAP/Webmail
```

### Outbound Flow

```text
SMTP/Webmail
→ RFC822 generation
→ Persist Sent item
→ Postmark Send API
→ Delivery events
```

## Core Packages

```text
cmd/
  server/
  worker/

internal/
  api/
  auth/
  contacts/
  dav/
  db/
  imap/
  logger/
  mailstore/
  models/
  postmark/
  redis/
  reputation/
  search/
  smtp/
  webmail/
  webhook/
  workers/
```

## PostgreSQL Entities

### Core
- `domains`
- `users`
- `domain_members`
- `auth_sessions`

### Mail
- `messages`
- `labels`
- `message_labels`
- `attachments`
- `message_flags`
- `threads`

### Contacts
- `contacts`
- `contact_reputation`

### Spam
- `whitelist`
- `greylist`
- `blacklist`

### Events
- `delivery_logs`
- `webhook_events`
- `bounce_events`

## IMAP Features

-   IMAP4rev1
-   IDLE
-   MOVE
-   ACL
-   THREAD
-   SORT
-   QUOTA
-   QRESYNC
-   CONDSTORE
-   UIDPLUS

## Web UI

### Admin
- Domains
- Users
- Spam rules
- Postmark config
- Logs

### User
- Inbox
- Compose
- Contacts
- Calendar
- Search

## Worker Processes

1.  webhook processor
2.  bounce processor
3.  delivery processor
4.  reputation updater
5.  spam evaluator
6.  search updater
7.  mailbox synchronizer

## Deployment

Supported platforms:
- Docker
- Docker Compose
- Nix

Services:
- app
- postgres
- redis
- workers

## API References

- [Postmark Inbound Webhook](https://postmarkapp.com/developer/webhooks/inbound-webhook)
- [Postmark Webhooks Overview](https://postmarkapp.com/developer/webhooks/webhooks-overview)
- [Postmark Email API](https://postmarkapp.com/developer/api/email-api)

## Required Libraries

| Library | Purpose |
|---|---|
| [mrz1836/postmark](https://github.com/mrz1836/postmark) | Outbound sending, inbound parsing, bounce processing |
| [emersion/go-imap](https://github.com/emersion/go-imap) | IMAP4rev1 server implementation |
| [emersion/go-smtp](https://github.com/emersion/go-smtp) | SMTP AUTH and submission server |
| [emersion/go-message](https://github.com/emersion/go-message) | RFC822 and MIME parsing |
| [emersion/go-webdav](https://github.com/emersion/go-webdav) | WebDAV, CardDAV, CalDAV |
| [kardianos/service](https://github.com/kardianos/service) | System service management |
https://gin-gonic.com/
https://github.com/go-playground/validator