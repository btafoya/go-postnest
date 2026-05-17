# PostNest Codebase Concerns

This document catalogs technical debt, known issues, security gaps, performance concerns, fragile areas, missing features, testing gaps, and operational weaknesses found during codebase analysis.

## 1. Unimplemented Features & Stubs

### 1.1 CalDAV Completely Stubbed
- **Location**: `internal/dav/dav.go:260-304`
- **Severity**: High (advertised feature, zero implementation)
- All CalDAV backend methods return `fmt.Errorf("not implemented")`:
  - `CreateCalendar`, `ListCalendars`, `GetCalendar`, `GetCalendarObject`, `ListCalendarObjects`, `QueryCalendarObjects`, `PutCalendarObject`, `DeleteCalendarObject`
- No `calendar_events` table exists in schema; iCalendar events cannot be stored.
- The `/.well-known/caldav` redirect exists but the mounted handler is non-functional.

### 1.2 STARTTLS on IMAP/SMTP
- **Location**: `internal/imap/imap.go`, `internal/smtp/smtp.go`
- **Severity**: High
- IMAP and SMTP servers accept TLS config for implicit TLS (ports 993/465), but STARTTLS negotiation on plain ports (143/587) is **not implemented**.
- `INTEGRATION.md` explicitly lists this as an open item.
- Clients that attempt `STARTTLS` after connection will fail or fall back to plaintext.

### 1.3 WebDAV File Storage
- **Severity**: Medium
- `PROTOCOL-DESIGN.md` describes file storage under `/dav/files/`, but no implementation exists.
- Attachment access via WebDAV is not wired.

### 1.4 Admin REST API Endpoints
- **Severity**: Medium
- `API-SPEC.md` documents admin endpoints (`/api/v1/admin/domains`, `/api/v1/admin/users`, spam rules, delivery logs, reputation dashboards).
- Only a single stub endpoint exists in `cmd/server/main.go` (`/admin/api/v1/domains`).
- No CRUD for domains, users, whitelist/blacklist/greylist management, or delivery log analytics.

### 1.5 Frontend UI
- **Severity**: Medium
- No web UI exists; the project is backend-only.
- `PLAN.md` and `AGENTS.md` reference a Gmail-style webmail UI, but no HTML templates, JS, or CSS are present.

### 1.6 IMAP IDLE Pub/Sub
- **Severity**: Medium
- `ARCHITECTURE.md` and `PROTOCOL-DESIGN.md` describe Redis pub/sub for IMAP IDLE notifications.
- The IMAP backend (`internal/imap/backend.go`) does **not** publish or subscribe to Redis channels.
- IDLE clients will not receive real-time mailbox updates.

### 1.7 IMAP MOVE Command
- **Severity**: Medium
- `PROTOCOL-DESIGN.md` lists MOVE as supported, but `imapMailbox` has no `MoveMessages` method.
- The go-imap backend interface requires it; missing it may cause client errors.

### 1.8 Search Indexer Queue
- **Location**: `internal/search/search.go:33-37`
- **Severity**: Low
- `ProcessQueue` is a no-op: `return 0, nil`.
- Search vector updates happen synchronously in `inbound.go` and `pgstore.go` instead.
- No batching or asynchronous indexing pipeline exists.

---

## 2. Security Concerns

### 2.1 Plaintext Authentication Allowed in Production
- **Location**: `cmd/server/main.go:187`
- **Severity**: Critical
- When no TLS config is provided, `allowInsecureAuth = true` is set for both IMAP and SMTP.
- Passwords are transmitted in plaintext over the wire.
- There is no enforcement that production deployments must use TLS.

### 2.2 Webhook Signature Verification Is Not HMAC-SHA256
- **Location**: `internal/webhook/webhook.go:131-136`
- **Severity**: High
- `verify()` only checks `X-Postmark-Server-Token` against a simple string comparison.
- `INTEGRATIONS.md` claims HMAC-SHA256 verification is used, but the code does not compute or verify an HMAC.
- An attacker who knows the secret can forge webhooks; the current check is trivially bypassable if the header is guessable or leaked.

### 2.3 Webhook Dedup Fails Open
- **Location**: `internal/webhook/webhook.go:120-128`
- **Severity**: Medium
- If Redis is unreachable during dedup, the function returns `true` (process the webhook).
- This means Redis outages lead to duplicate message processing.

### 2.4 In-Memory Rate Limiter
- **Location**: `internal/api/middleware.go:192-260`
- **Severity**: Medium
- Rate limiting state is stored in a local `map[string][]time.Time`.
- Does not work across multiple server instances (no shared state).
- An attacker can bypass rate limits by hitting different replicas.

### 2.5 No CSRF Protection on Webmail API
- **Severity**: Medium
- Session cookie is `SameSite=Lax`, but there is no CSRF token validation on state-changing endpoints (POST/PUT/PATCH/DELETE).
- Cross-origin POSTs from trusted origins (due to CORS) could be exploited.

### 2.6 No Content Security Policy Headers
- **Severity**: Low
- HTTP responses do not set `Content-Security-Policy`, `X-Frame-Options`, or `X-Content-Type-Options`.
- If a web UI is added later, this becomes a larger concern.

### 2.7 SMTP LOGIN Auth Implementation Is Minimal
- **Location**: `internal/smtp/smtp.go:348-365`
- **Severity**: Low
- Custom `loginServer` struct implements LOGIN SASL but does not handle challenge/response edge cases well.
- No tests for malformed LOGIN sequences.

### 2.8 Attachment Storage in PostgreSQL BYTEA
- **Severity**: Medium
- Attachments are stored inline in `attachments.data` (BYTEA).
- For large attachments or high volume, this bloats PostgreSQL tables and WAL.
- No size limits are enforced at the database layer.
- No deduplication or external object storage (S3) integration.

### 2.9 HTML Sanitization on Drafts Is One-Way
- **Location**: `internal/webmail/webmail.go:254`, `287`
- **Severity**: Low
- `bluemonday.UGCPolicy().Sanitize()` strips dangerous HTML but may also strip legitimate content.
- No validation that the sanitized output is still useful; empty body could result.

### 2.10 Missing Input Validation on Email Addresses
- **Severity**: Medium
- SMTP `MAIL FROM` and `RCPT TO` addresses are split on `@` but not validated against RFC 5322.
- Webmail `To` addresses in draft creation are not validated.
- Invalid addresses can be persisted and later cause Postmark API errors.

---

## 3. Error Handling Gaps

### 3.1 Widespread Error Discards (`_ =`)
- **Severity**: High
- Critical paths silently discard errors:
  - `internal/webmail/webmail.go`: `writeJSON` discards encode errors.
  - `internal/auth/auth.go`: `_, _ = s.pool.Exec(ctx, ...)` for session last-used update.
  - `internal/workers/inbound.go`: `thread find/create failed` only logged.
  - `internal/workers/send.go`: `CreateDeliveryLog` error only logged.
  - `internal/imap/backend.go`: `CountTotalByLabel`, `CountUnreadByLabel` errors discarded in `Status()`.
  - `internal/imap/backend.go`: `GetFlagsBatch` error discarded in `ListMessages()`.
  - `internal/dav/dav.go`: `vcardToString` discards encode error.
  - `internal/webmail/webmail.go`: `batchMessages` ignores per-message update errors.

### 3.2 SMTP MIME Parsing Errors Do Not Abort Session
- **Location**: `internal/smtp/smtp.go:203-206`
- **Severity**: Medium
- If `mr.NextPart()` returns an error other than `io.EOF`, the loop breaks but `Data()` continues and may relay a malformed message.
- The error is only logged; the client receives `250 OK` after Postmark send.

### 3.3 IMAP Backend Swallows Errors
- **Location**: `internal/imap/backend.go`
- **Severity**: Medium
- `Status()` ignores `CountTotalByLabel` and `CountUnreadByLabel` errors, returning potentially stale/incorrect counts.
- `Expunge()` ignores `GetFlags` errors, potentially leaving deleted messages in place.
- `ListMessages()` ignores `GetFlagsBatch` errors, producing messages with missing flags.

### 3.4 Worker Job Enqueue Ignores Marshal Errors
- **Location**: `internal/workers/workers.go:157`, `internal/webmail/webmail.go:432`
- **Severity**: Medium
- `json.Marshal(job)` errors are discarded with `_` when enqueuing jobs.
- A malformed job could be silently lost.

### 3.5 Search Vector Update Failures Only Logged
- **Location**: `internal/workers/inbound.go:117-119`, `internal/mailstore/pgstore.go`
- **Severity**: Low
- `UpdateSearchVector` failures are logged but do not fail the inbound processing or retry.
- Messages may remain unsearchable until manual reindex.

---

## 4. Performance Concerns

### 4.1 IMAP Fetches Up to 10,000 Messages Without Pagination
- **Location**: `internal/imap/backend.go:130`, `177`, `218`, `267`
- **Severity**: High
- `ListMessages`, `SearchMessages`, `UpdateMessagesFlags`, `CopyMessages`, and `Expunge` all use `Limit: 10000`.
- For large mailboxes, this loads thousands of messages into memory.
- No streaming or batched processing; high memory usage and latency.

### 4.2 IMAP Status Counts Trigger Full Scans Per Mailbox
- **Location**: `internal/imap/backend.go:103-125`
- **Severity**: Medium
- `Status()` calls `CountTotalByLabel` and `CountUnreadByLabel` for each requested status item.
- These are separate SQL queries; requesting all 5 status items = 5 queries per mailbox.

### 4.3 Search Uses `plainto_tsquery` Only
- **Location**: `internal/mailstore/pgstore.go:395-430`
- **Severity**: Low
- No support for phrase search, prefix search, or boolean operators.
- `plainto_tsquery('english', $3)` treats the entire query as a single phrase.
- Users cannot use `"exact phrase"`, `+must`, `-exclude`, or `prefix*`.

### 4.4 Redis Queue Has No Exactly-Once Semantics
- **Location**: `internal/redis/redis.go`, `internal/workers/workers.go`
- **Severity**: Medium
- Uses Redis lists (`LPush`/`BRPop`), not Redis Streams.
- Jobs can be lost if a worker crashes between `BRPop` and `Ack`.
- No visibility timeout or in-flight tracking.

### 4.5 Worker Poll Interval Is Fixed
- **Location**: `internal/workers/workers.go`
- **Severity**: Low
- `pollInterval` is fixed (default 5s); no backoff when queues are empty.
- Redis receives a `BRPop` every 5 seconds per worker goroutine, generating constant load.

### 4.6 No Prepared Statement Reuse Mentioned
- **Severity**: Low
- All SQL is executed as raw strings via `pool.QueryRow`/`Exec`.
- `pgx` automatically prepares statements, but explicit caching is not configured.

---

## 5. Database / Schema Concerns

### 5.1 `imap_uids` Table Is Unused
- **Location**: `000001_init.up.sql`
- **Severity**: High
- The `imap_uids` table is defined but never populated by `mailstore`.
- IMAP UIDs are derived deterministically from the first 4 bytes of the message UUID (`messageUID()`).
- This is fragile: UUID v7 is time-ordered, but the first 4 bytes may not be monotonic per mailbox.
- UIDVALIDITY is derived from label ID bytes, which changes when labels are recreated.

### 5.2 Thread Subject Hashing Is Not Normalized
- **Location**: `internal/mailstore/pgstore.go:296-318`
- **Severity**: Medium
- `FindOrCreateThread` uses raw `subject` for matching, not a normalized hash.
- Replies with `Re:`, `FW:`, `[External]` prefixes create separate threads.
- `PROTOCOL-DESIGN.md` mentions `subject_hash` normalization but it is not implemented.

### 5.3 `messages.source` Is `BYTEA NOT NULL`
- **Location**: `000001_init.up.sql`
- **Severity**: Low
- All messages must have RFC822 source, but inbound from Postmark webhooks provides parsed JSON, not raw RFC822.
- The `Source` field may be empty or reconstructed, violating the "preserve exact RFC822" rule.

### 5.4 No Migration Down Files
- **Location**: `internal/migrate/migrations/`
- **Severity**: Low
- Only `.up.sql` files exist. Rollbacks are not possible with `golang-migrate` down commands.
- Schema changes are irreversible without manual intervention.

### 5.5 Greylist Table Lacks Automatic Expiration
- **Location**: `000001_init.up.sql`
- **Severity**: Low
- `greylist` rows accumulate forever unless manually cleared.
- No TTL, no cron job, no worker to purge stale greylist entries.

### 5.6 No Table Partitioning Strategy
- **Severity**: Medium
- `messages`, `delivery_logs`, `webhook_events`, and `bounce_events` will grow unbounded.
- No PostgreSQL partitioning or archival strategy is defined.

### 5.7 `UpdateMessage` Has Wrong Parameter Binding
- **Location**: `internal/mailstore/pgstore.go:176-189`
- **Severity**: High
- The `UPDATE` statement binds parameters out of order:
  ```sql
  html_body = coalesce($10, html_body),
  plain_text = coalesce($11, plain_text),
  to_addresses = coalesce($12, to_addresses),
  ```
  But the `Exec` call passes `patch.Subject` as `$10`, `patch.HTMLBody` as `$11`, `patch.PlainText` as `$12`, and `patch.ToAddresses` is missing from the argument list entirely.
- This means updating `HTMLBody` actually updates `subject`, and `ToAddresses` cannot be updated via `UpdateMessage`.

---

## 6. Operational Concerns

### 6.1 No Health Checks for IMAP/SMTP
- **Location**: `cmd/server/main.go`
- **Severity**: Medium
- `/healthz` checks PostgreSQL and Redis only.
- IMAP and SMTP server health is not verified; a hung listener would not be detected.

### 6.2 No Metrics or Monitoring Integration
- **Severity**: Medium
- No Prometheus, OpenTelemetry, Sentry, or structured metrics.
- No request latency histograms, queue depth gauges, or error rate counters.
- Operational blind spots: cannot alert on worker backlog, SMTP auth failures, or delivery success rates.

### 6.3 Worker Dead Letter Queue Has No Reprocessing
- **Location**: `internal/redis/redis.go`, `internal/workers/workers.go`
- **Severity**: Medium
- Failed jobs land in `queue:jobs:dead` but there is no UI, CLI, or worker to reprocess them.
- Dead letters accumulate silently.

### 6.4 Certificate Manager DNS Provider Is Hard-Coded to Cloudflare
- **Location**: `internal/certmanager/manager.go:228-236`
- **Severity**: Low
- Only Cloudflare DNS-01 is supported. Route53, DigitalOcean, etc. are not pluggable.
- If Cloudflare credentials are missing, ACME fails with no fallback.

### 6.5 No Graceful Connection Drain for IMAP/SMTP
- **Location**: `cmd/server/main.go`
- **Severity**: Medium
- Shutdown closes IMAP/SMTP listeners immediately (`imapSrv.Stop()`, `smtpSrv.Stop()`).
- In-flight IMAP FETCH or SMTP DATA sessions may be aborted mid-transaction.

### 6.6 Logger Level Is Hard-Coded to Info
- **Location**: `internal/logger/logger.go`
- **Severity**: Low
- No way to configure `slog.Level` via config or env var.
- Debug-level logging requires code change.

---

## 7. Testing Gaps

### 7.1 No Mailstore PGStore Tests
- **Severity**: High
- `internal/mailstore/pgstore.go` has zero tests.
- The `UpdateMessage` parameter bug would have been caught by even a single test.
- No integration tests against PostgreSQL.

### 7.2 No IMAP Backend Tests
- **Severity**: High
- `internal/imap/backend.go` has zero tests.
- UID derivation, flag mapping, mailbox operations, and thread handling are untested.

### 7.3 No SMTP Server Tests
- **Severity**: High
- `internal/smtp/smtp.go` has only LOGIN mechanism tests.
- No tests for MAIL FROM validation, DATA parsing, attachment extraction, or Postmark relay.

### 7.4 No Webhook Handler Tests
- **Severity**: Medium
- `internal/webhook/webhook.go` has zero tests.
- Signature verification, deduplication, and enqueue logic are untested.

### 7.5 No Worker Processor Tests (Inbound, Bounce, Delivery, Send, Spam)
- **Severity**: Medium
- `internal/workers/` has pool/orchestration tests but no processor-specific tests.
- Inbound parsing, reputation evaluation, delivery logging, and draft sending are untested.

### 7.6 No Certmanager Tests
- **Severity**: Low
- Certificate loading, renewal logic, and ACME interactions are untested.

### 7.7 No End-to-End or Benchmark Tests
- **Severity**: Medium
- No `*_bench.go` files.
- No Docker Compose-based integration test suite.
- No load testing or protocol conformance tests.

---

## 8. Code Quality & Fragile Areas

### 8.1 IMAP UID Derivation from UUID Bytes Is Fragile
- **Location**: `internal/imap/backend.go:316-321`
- **Severity**: Medium
- `messageUID()` extracts the first 4 bytes of a UUID v7 and packs them into a `uint32`.
- UUID v7 is time-ordered, but the first bytes encode the timestamp in a way that may not be monotonic per mailbox if messages are created concurrently.
- Collisions or non-monotonic UIDs break IMAP client expectations.

### 8.2 Domain Selection in DAV and Webmail Uses First Domain
- **Location**: `internal/dav/dav.go:71-78`, `internal/webmail/webmail.go:96-109`
- **Severity**: Medium
- `domainIDFromUser()` returns `domains[0].DomainID` without allowing the user to select which domain.
- Multi-domain users cannot choose their active domain for DAV or webmail operations.
- No `X-Domain-ID` header support in DAV auth.

### 8.3 Session Stored as Request ID in Context
- **Location**: `internal/api/middleware.go:188-189`
- **Severity**: Low
- `RequireSession` stores the `session` object in `ctxKeyRequestID`, conflating two concepts.
- `RequestIDFromContext` may return a session string instead of a request ID.

### 8.4 Context Key Type Is String, Not Struct
- **Location**: `internal/api/middleware.go:24-28`, `internal/dav/dav.go:19`
- **Severity**: Low
- Context keys use a typed string (`type ctxKey string`), which is better than bare strings but still vulnerable to collision across packages.
- Best practice is an unexported struct type (`type ctxKey struct{}`).

### 8.5 `FindOrCreateThread` Is Race-Prone
- **Location**: `internal/mailstore/pgstore.go:296-318`
- **Severity**: Medium
- Two concurrent inbound messages with the same subject could create duplicate threads.
- No `SELECT FOR UPDATE` or unique constraint on `(domain_id, user_id, subject_hash)`.

### 8.6 `splitN` Is a Hand-Rolled String Splitter
- **Location**: `internal/auth/auth.go:123-133`
- **Severity**: Low
- A custom `splitN` function splits on `$` for password hash parsing instead of using `strings.SplitN`.
- It only handles `n=2` and does not use `strings.Index` efficiently.
- Unnecessary maintenance burden; `strings.SplitN(encodedHash, "$", 2)` is equivalent.

---

## 9. Items Previously Identified as Concerns (Now Addressed)

The following items were noted in earlier analysis or `INTEGRATION.md` and have since been resolved:

| Item | Status | Evidence |
|------|--------|----------|
| Worker pool with retry logic | **Resolved** | `internal/workers/workers.go` implements retry with exponential backoff and dead-letter queue. |
| TLS configuration for IMAP/SMTP | **Resolved** | `cmd/server/main.go` supports ACME, static certs, and plaintext fallback. |
| Health check endpoint | **Resolved** | `/healthz` checks PostgreSQL and Redis in `cmd/server/main.go`. |
| Graceful shutdown for HTTP/IMAP/SMTP | **Resolved** | `cmd/server/main.go` handles `SIGINT`/`SIGTERM` with `srv.Shutdown()`, `imapSrv.Stop()`, `smtpSrv.Stop()`. |
| Webhook handler enqueues properly formatted jobs | **Resolved** | `internal/webhook/webhook.go` creates `workers.Job` structs and enqueues to `queue:jobs`. |
| TOML configuration with env overrides | **Resolved** | `internal/config/loader.go` implements full TOML + env override system. |
| Migration CLI | **Resolved** | `cmd/migrate/main.go` and `internal/migrate/migrate.go` provide embedded migration runner. |
| Argon2id password hashing | **Resolved** | `internal/auth/auth.go` uses `golang.org/x/crypto/argon2`. |
| IMAP APPEND, EXPUNGE, COPY, flag updates | **Resolved** | `internal/imap/backend.go` implements these operations. |
| SMTP LOGIN auth | **Resolved** | `internal/smtp/smtp.go:348-365` implements LOGIN mechanism. |
| CardDAV backend | **Resolved** | `internal/dav/dav.go` implements list/get/put/delete for contacts. |
| Search composite index | **Resolved** | `000005_search_composite.up.sql` adds `messages(domain_id, user_id)` index. |
| Systemd service integration | **Resolved** | `scripts/install-systemd.sh` generates units and installs binaries. |
| Docker Compose stack | **Resolved** | `docker-compose.yml` and `scripts/install-docker.sh` are present. |
| Nix flake and NixOS module | **Resolved** | `flake.nix` and `nix/module.nix` provide packages and systemd services. |

---

## 10. Summary by Severity

| Severity | Count | Key Items |
|----------|-------|-----------|
| **Critical** | 2 | Plaintext auth allowed in production; `UpdateMessage` parameter binding bug |
| **High** | 7 | CalDAV stub; STARTTLS missing; webhook HMAC not implemented; `imap_uids` unused; no PGStore tests; no IMAP tests; no SMTP tests |
| **Medium** | 18 | Admin API missing; IMAP IDLE missing; IMAP MOVE missing; error discards; SMTP MIME errors not fatal; IMAP status errors swallowed; in-memory rate limiter; no CSRF; attachment BYTEA storage; IMAP 10k fetch limit; no exactly-once queue; no table partitioning; no health check for IMAP/SMTP; no metrics; dead letter no reprocess; no graceful IMAP/SMTP drain; `FindOrCreateThread` race; IMAP UID fragility; multi-domain selection broken |
| **Low** | 12 | WebDAV missing; no CSP headers; LOGIN edge cases; HTML sanitization issues; no email validation; search query limitations; no prepared statement config; `messages.source` may be empty; no down migrations; greylist no expiry; hard-coded logger level; context key type |
