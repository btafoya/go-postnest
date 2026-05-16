# Design Summary — Go-PostNest Postmark Mail Platform

## Documents

| Document | Scope | Key Decisions |
|---|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | System topology, service boundaries, data flow, deployment | Multi-tenant domain model, Redis pub/sub for IMAP IDLE, Docker Compose + Nix deployment |
| [DATABASE-SCHEMA.md](DATABASE-SCHEMA.md) | PostgreSQL tables, indexes, constraints, FTS design | `BYTEA` for RFC822 + attachments, Gmail-style labels via `message_labels`, `imap_uids` for IMAP correctness |
| [API-SPEC.md](API-SPEC.md) | REST endpoints, webhook contracts, internal Go interfaces | Session + Bearer auth, Gmail-style batch operations, unified error format, 8 worker types |
| [PROTOCOL-DESIGN.md](PROTOCOL-DESIGN.md) | IMAP4rev1, SMTP proxy, CardDAV/CalDAV/WebDAV | IDLE via Redis pub/sub, immediate foreground SMTP relay to Postmark, vCard 4.0 / iCal mapping |
| [COMPONENT-DESIGN.md](COMPONENT-DESIGN.md) | Go package layout, interfaces, implementation order | `internal/models` is pure, `mailstore` is canonical storage abstraction, recommended implementation sequence |

## Requirement Coverage (PLAN.md)

| PLAN.md Requirement | Design Coverage | Status |
|---|---|---|
| IMAP4 server | `internal/imap` + PROTOCOL-DESIGN.md §1 | Covered |
| SMTP proxy | `internal/smtp` + PROTOCOL-DESIGN.md §2 | Covered |
| Gmail-style webmail | `internal/webmail` + API-SPEC.md §2 | Covered |
| CardDAV / CalDAV / WebDAV | `internal/dav` + PROTOCOL-DESIGN.md §3 | Covered |
| Multi-tenant domain support | ARCHITECTURE.md §6 + DATABASE-SCHEMA.md `domains` / `domain_members` | Covered |
| PostgreSQL mail storage | DATABASE-SCHEMA.md §3 | Covered |
| Contacts + reputation system | DATABASE-SCHEMA.md §4 + §5 + COMPONENT-DESIGN.md `reputation.Engine` | Covered |
| Argon2id password hashing | ARCHITECTURE.md §9 + COMPONENT-DESIGN.md `auth.Service` | Covered |
| Gmail-style labels + folders | DATABASE-SCHEMA.md §3.3 / 3.4 + PROTOCOL-DESIGN.md §1.2 | Covered |
| Multi-domain users supported | DATABASE-SCHEMA.md `domain_members` | Covered |
| Rich HTML editor | API-SPEC.md §2.4 (`html_body` in drafts) | Covered |
| Gmail-style autosave drafts | API-SPEC.md §2.4 (`PUT /api/v1/drafts/:id`) | Covered |
| PostgreSQL full text search (tsvector) | DATABASE-SCHEMA.md §7 + ARCHITECTURE.md `search` | Covered |
| Full DAV support | PROTOCOL-DESIGN.md §3 | Covered |
| Store exact RFC822 source permanently | DATABASE-SCHEMA.md `messages.source BYTEA` | Covered |
| Attachments stored as PostgreSQL bytea | DATABASE-SCHEMA.md `attachments.data BYTEA` | Covered |
| Immediate SMTP relay to Postmark | PROTOCOL-DESIGN.md §2.2 (synchronous foreground relay) | Covered |
| Webhook processing (bounce, delivery, retries, complaints, failures) | API-SPEC.md §4 + ARCHITECTURE.md §3.5 / 3.6 | Covered |
| IMAP4rev1, IDLE, MOVE, ACL, THREAD, SORT, QUOTA, QRESYNC, CONDSTORE, UIDPLUS | PROTOCOL-DESIGN.md §1.1 | Covered |
| Admin UI (Domains, Users, Spam rules, Postmark config, Logs) | API-SPEC.md §3 | Covered |
| User UI (Inbox, Compose, Contacts, Calendar, Search) | API-SPEC.md §2 | Covered |
| 7 Worker Processes | ARCHITECTURE.md §3.6 + COMPONENT-DESIGN.md §4 | Covered |
| Docker / Docker Compose / Nix | ARCHITECTURE.md §8 + `flake.nix` description | Covered |

## Consistency Checks

1. **Multi-tenancy**: Every table in DATABASE-SCHEMA.md includes `domain_id` + `user_id` FKs. API-SPEC.md enforces domain context via `X-Domain-ID` header. COMPONENT-DESIGN.md interfaces accept `domainID, userID` in all store methods. **Consistent**.

2. **Authentication**: ARCHITECTURE.md states Argon2id + session cookies + Bearer tokens. API-SPEC.md details the three auth modes. COMPONENT-DESIGN.md `auth.Service` exposes matching methods. DATABASE-SCHEMA.md `users.password_hash` stores Argon2id output. **Consistent**.

3. **IMAP UID mapping**: PROTOCOL-DESIGN.md §1.3 specifies an `imap_uids` table. DATABASE-SCHEMA.md §3.7 defines it with `BIGSERIAL uid`, `modseq`, and proper PK/indexes. **Consistent after fix**.

4. **SMTP relay semantics**: PLAN.md says "Immediate SMTP relay to Postmark". PROTOCOL-DESIGN.md §2.2 was updated to specify **synchronous foreground relay** (Postmark API call before returning 250 OK), with fallback to background worker for retries. This satisfies the requirement while preserving error semantics. **Consistent after fix**.

5. **Nix support**: PLAN.md lists Nix. ARCHITECTURE.md §8.3 now includes `flake.nix`, `nixosModules`, and `dockerTools` details. **Consistent after fix**.

6. **Labels vs Mailboxes**: PROTOCOL-DESIGN.md §1.2 maps Gmail labels to IMAP mailboxes with `\Special-Use` attributes. DATABASE-SCHEMA.md §3.3 seeds system labels per user. API-SPEC.md uses `label_id` for message listing. **Consistent**.

7. **Worker definitions**: ARCHITECTURE.md lists 7 workers. COMPONENT-DESIGN.md defines 8 processors (including `PostmarkSender` for deferred retries). The extra `PostmarkSender` is a retry helper, not a distinct PLAN.md worker type; the 7 logical worker types from PLAN.md are all present. **Consistent**.

8. **Search architecture**: ARCHITECTURE.md states async `tsvector` updates. DATABASE-SCHEMA.md §7.3 avoids synchronous triggers. COMPONENT-DESIGN.md `search.Indexer` has `Queue` and `ProcessQueue`. **Consistent**.

## Identified Gaps / Notes for Implementation

1. **CalDAV persistence**: PROTOCOL-DESIGN.md §3.3 describes `calendar_events` table but DATABASE-SCHEMA.md does not include it (marked optional). If calendar UI is required, add migration V5__calendar.sql before CalDAV implementation.

2. **Attachment scaling**: DATABASE-SCHEMA.md stores attachments as `BYTEA`. If average attachment size exceeds ~10MB or total volume exceeds ~100GB, migrate to S3-style object storage with `attachments.storage_url` and keep `BYTEA` for small inline attachments.

3. **IMAP UIDVALIDITY**: PROTOCOL-DESIGN.md suggests hashing domain ID + mailbox creation epoch. A simpler and more robust approach is to use a random 32-bit integer per mailbox stored in a new `mailbox_metadata` table. This should be decided during IMAP implementation.

4. **Rate limiting**: API-SPEC.md §7 defines limits but does not specify the Redis key scheme or token-bucket algorithm. COMPONENT-DESIGN.md `api.RateLimiter` middleware should use a Redis-backed sliding window (e.g., `ZREMRANGEBYSCORE` + `ZADD`).

5. **DAV authentication**: PROTOCOL-DESIGN.md §3.5 recommends Basic Auth for DAV. Consider adding Digest Auth support for legacy clients that do not send Basic over TLS.

## Next Steps

1. **Review** this design package with stakeholders.
2. **Approve** architecture decisions (particularly synchronous SMTP relay and `BYTEA` attachment storage).
3. **Begin implementation** using the order specified in COMPONENT-DESIGN.md §9:
   - Foundation: config, db, redis, logger, models
   - Core: mailstore, auth, api middleware
   - Protocols: webmail, webhook, workers
   - Advanced: IMAP, SMTP, contacts, DAV, search, reputation
