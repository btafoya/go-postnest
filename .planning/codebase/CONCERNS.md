# PostNest Codebase Concerns

This document catalogs technical debt, known issues, security gaps, performance concerns, fragile areas, missing features, testing gaps, and operational concerns identified through thorough static analysis of the codebase.

---

## 1. Security

### 1.1 Authentication & Authorization
- **Weak default Argon2id parameters**: `ARGON2ID_MEMORY=65536` (64MB) with `ARGON2ID_TIME=3` is below current OWASP recommendations (19MB, 2 iterations is common, but 64MB/3 is reasonable for 2024; however, no upgrade path exists for existing hashes).
- **No password strength policy**: `auth.CreateUser` accepts any password without length, complexity, or entropy checks.
- **No account lockout**: Failed authentication attempts are not tracked or rate-limited per account, leaving brute-force vulnerability.
- **Session token rotation missing**: Sessions have a 7-day fixed `MaxAge` with no rotation on use.
- **Missing 2FA/MFA**: No TOTP, WebAuthn, or backup code support.
- **API keys share session table**: API keys and browser sessions share `auth_sessions` with different `type` values but identical validation paths, increasing blast radius if token hashing is broken.
- **IMAP/SMTP auth uses `context.Background()`**: Authentication in `imap/backend.go` and `smtp/smtp.go` uses background contexts with no timeout, risking goroutine leaks on slow auth queries.

### 1.2 Transport Security
- **No STARTTLS support**: Both IMAP (port 143) and SMTP (port 587) run plain-text when TLS is not configured. There is no `STARTTLS` command implementation to upgrade connections mid-flight.
- **TLS certificate hot-reload gaps**: `certmanager.Manager` stores certificates atomically but IMAP/SMTP servers hold a `*tls.Config` reference; reloading requires restart.
- **Insecure auth allowed without TLS**: `allowInsecureAuth` defaults to `true` when no certificates are configured, with no enforcement of STARTTLS before auth.
- **No OCSP stapling or certificate transparency checks**.
- **CORS allows credentials without origin validation on preflight**: `CORS` middleware allows any origin matching the list, but there is no `Access-Control-Allow-Credentials` header, so cookies are not sent cross-origin. However, this is inconsistent with the `RequireSession` middleware that reads cookies.

### 1.3 Input Validation & Sanitization
- **No Content-Type validation on API**: `webmail.Handler` endpoints accept requests with any `Content-Type`, risking JSON CSRF in legacy browsers.
- **No CSRF tokens for cookie auth**: The API uses cookie-based sessions but provides no CSRF double-submit cookie or token. While modern browsers block cross-origin JSON POSTs by default, this is a defense-in-depth gap.
- **No HTML sanitization**: `HTMLBody` from inbound/outbound messages is stored and served raw. XSS is possible if messages are rendered in a webmail UI without client-side sanitization.
- **Email address validation missing**: No RFC 5322 validation on `From`, `To`, `Cc` addresses before storage or relay.
- **No attachment malware scanning**: Attachments are stored as raw `BYTEA` with no ClamAV or similar integration.

### 1.4 Webhook Security
- **Weak webhook verification**: `webhook.Handler.verify` only checks `X-Postmark-Server-Token` against a plain string secret. No HMAC signature verification of the request body.
- **No webhook IP allowlisting**: Postmark source IPs are not validated.
- **No webhook replay protection**: Dedup uses a 5-minute Redis TTL but relies on `MessageID` presence, which may be spoofed or missing.

### 1.5 Data Protection
- **No encryption at rest**: PostgreSQL stores attachment `BYTEA`, message source, and contact data unencrypted.
- **Missing PII handling / GDPR**: No data export, right-to-erasure, or retention policy enforcement APIs.
- **Secrets in config**: `PostmarkToken`, `PostmarkWebhookSecret`, `SessionKey` are loaded from env/config without encryption or secret manager integration.

---

## 2. SQL & Data Integrity

### 2.1 SQL Injection Risks
- **Sort column injection in `ListMessages`**: `internal/mailstore/pgstore.go:143` uses `fmt.Sprintf` to interpolate `sortCol` and `order` into the query. The values are drawn from a whitelist (`created_at`, `date`, `subject`, `from`, `size`), but a bug or future change that accepts user input directly would create an injection vector. This pattern should be replaced with a parameterized switch statement.
- **Dynamic column in `UpdateReputation`**: `internal/reputation/reputation.go:79` uses `fmt.Sprintf` to interpolate a column name (`sent_count`, `received_count`, etc.). While controlled by a switch, this is fragile.

### 2.2 Transaction & Consistency Gaps
- **Non-atomic bounce processing**: `bounce.go` queries `delivery_logs` for `id` and `domain_id`, then separately inserts a `bounce_events` row. If the process crashes between steps, the bounce event is lost.
- **Auth label seeding outside user creation tx**: `auth.go:CreateUser` begins a transaction for user insertion, but the system label seeding query runs inside the same transaction (good). However, it seeds labels for *all* domain memberships via `domain_members` lookup — if the caller did not set `domain_id` before calling, labels may be missing.
- **Thread creation race**: `FindOrCreateThread` has a check-then-act pattern without serialization; concurrent inbound messages with the same subject can create duplicate threads.
- **Search vector is a no-op**: `UpdateSearchVector` (pgstore.go:504) does `UPDATE messages SET updated_at = updated_at WHERE id=$1`, failing to update `search_vector`. The trigger handles it on INSERT/UPDATE, but if the trigger is dropped or bypassed, search becomes stale.
- **Missing `delivery_logs` creation on outbound send**: `smtp.go` and `send.go` send via Postmark but never write a `delivery_logs` row, making bounce/delivery webhook correlation impossible for freshly sent messages.

### 2.3 Schema & Indexing Concerns
- **`messages.source` is `NOT NULL` but may be empty**: Many inbound paths (webhook) and outbound paths (SMTP) do not populate `Source`, yet the column is `NOT NULL` with no default.
- **Missing partial index on `messages.is_read`**: Label unread counts scan all messages per label; a partial index `WHERE is_read = false` would help.
- **`imap_uids` table is unused**: The schema defines `imap_uids` but `imap/backend.go` derives UIDs from UUID bytes, causing collisions and no stability across sessions.

---

## 3. Unimplemented / Stub Features

### 3.1 CalDAV (`internal/dav/dav.go`)
- **Entire CalDAV backend is stubbed**: All 9 methods return `fmt.Errorf("not implemented")`. Calendar sync clients will receive hard errors.
- **No WebDAV file storage backend**: Only CardDAV and CalDAV routes are mounted; no generic WebDAV file support.

### 3.2 STARTTLS
- **No `STARTTLS` command in SMTP**: `go-smtp` supports it, but the backend does not implement the `StartTLS` interface.
- **No `STARTTLS` in IMAP**: `go-imap/server` supports it, but not configured.

### 3.3 Spam & Reputation
- **Spam webhook handler exists but no processor**: `webhook.go` enqueues `"spam"` jobs, but `cmd/worker/main.go` does not register a processor. Jobs will be silently dropped with "no processor for job type" warnings.
- **Greylist decision returns but not enforced**: `reputation.Engine.EvaluateInbound` returns `DecisionGreylist`, but no code path actually delays or rejects greylisted messages.
- **No SPF, DKIM, or DMARC validation**: Inbound processing trusts Postmark's relay but does not independently verify sender authenticity.

### 3.4 Missing API Endpoints
- **No user settings API**: `users.settings` and `domains.settings` JSONB columns have no CRUD endpoints.
- **No domain administration CRUD**: Domains are created via migrations or manual DB insertion; no REST API exists.
- **No admin audit log**: No table or API for tracking admin actions (user creation, domain changes, etc.).
- **No metrics/health depth**: `/healthz` pings DB and Redis but does not check worker queue depth, dead letter queue length, or certificate expiry.

---

## 4. Performance & Scalability

### 4.1 Database
- **N+1 queries in IMAP**: `ListMailboxes` calls `CountTotalByLabel` and `CountUnreadByLabel` per label in a loop via `webmail.go`. For 20 labels, this is 40 extra queries per request.
- **Unbounded `LIMIT` in IMAP**: `imapMailbox.ListMessages` uses `mailstore.ListOptions{Limit: 10000}` with no pagination. A mailbox with 50k messages will OOM or timeout.
- **Full-table scans in search**: `Search` uses `plainto_tsquery('english', $3)` but `search_vector` is a `TSVECTOR`. The GIN index helps, but ranking with `ts_rank_cd` on large result sets is CPU-heavy. No query timeout is enforced.
- **No read replica usage**: `PostgresReadDSN` is loaded from config but never used. All queries hit the primary.

### 4.2 Memory & Goroutines
- **Unbounded rate limiter map**: `api.RateLimiter.clients` grows forever. IPs that hit the limiter once are never evicted from the map until process restart.
- **IMAP `context.Background()` leaks**: Every IMAP command spawns a new background context. A long-lived IMAP session with thousands of commands will leak context trees.
- **Attachment streaming**: `GetAttachment` loads full `BYTEA` into memory. Large attachments will cause memory pressure.

### 4.3 Caching
- **No caching layer for hot paths**: Label lists, user sessions, domain configs are queried on every request.
- **Redis only used for queues and dedup**: No session cache, no rate-limiting cache, no domain config cache.

---

## 5. Error Handling & Robustness

### 5.1 Silent Failures
- **`UpdateSearchVector` is a no-op**: See 2.2. It silently does nothing.
- **Label count errors ignored**: `webmail.listLabels` calls `CountTotalByLabel` and `CountUnreadByLabel` but discards errors with `_`. Database outages will show zero counts without error.
- **Auth label seeding ignores errors**: `CreateUser` uses `_, _ = tx.Exec(...)` for label seeding. If label insertion fails, the user is created without labels, breaking their inbox.
- **Thread creation failure is only a warning**: `inbound.go` logs `thread find/create failed` as Warn but continues. Messages may be orphaned from threads.
- **Attachment decode failure is a warning**: Inbound attachments that fail base64 decode are skipped with a warning; the message is stored without them.

### 5.2 Missing Timeouts
- **Postmark client has no HTTP timeout**: `postmark.NewClient` wraps `mrz1836/postmark` but does not configure the underlying HTTP client timeout. A hung Postmark API call will block workers indefinitely.
- **IMAP server `ListenAndServe` has no connection timeout**: Idle IMAP connections can hang forever.
- **SMTP `Data` uses a 60s timeout but no deadline on the reader**: A slow client can hold the connection open.

### 5.3 Worker Resilience
- **No circuit breaker for Postmark**: If Postmark is down, workers will retry with fixed 5s backoff, hammering the downstream.
- **No jitter in retry backoff**: Workers use `time.Duration(job.Attempts) * 5 * time.Second`, causing thundering herd on recovery.
- **Dead letter queue is a black hole**: `queue:dead` has no inspection API, no alerting, and no automatic reprocessing.
- **Job payload has no schema validation**: Processors unmarshal raw JSON into `map[string]any` or structs without version fields. A schema change will cause silent deserialization failures.

---

## 6. Testing Gaps

### 6.1 Missing Unit Tests
The following packages have **zero** tests:
- `internal/auth` (critical: password hashing, session management)
- `internal/certmanager` (critical: TLS certificate lifecycle)
- `internal/contacts`
- `internal/dav` (CardDAV/CalDAV logic)
- `internal/db`
- `internal/imap` (backend logic)
- `internal/logger`
- `internal/migrate`
- `internal/models`
- `internal/postmark`
- `internal/reputation`
- `internal/search`
- `internal/smtp` (only `loginServer` and mechanism enumeration tested)
- `internal/webmail` (only draft create/update tested)

### 6.2 Integration Test Gaps
- **No end-to-end SMTP/IMAP tests**: No black-box tests against a running server.
- **No webhook signature/hMAC tests**: Tests only cover dedup logic.
- **No worker integration tests with real Postgres**: All worker tests use miniredis but mock processors; no test validates database side effects.
- **No migration rollback tests**: Migrations have `.up.sql` but no `.down.sql` files.

### 6.3 Test Quality Issues
- `webmail_test.go` uses a mock store that ignores `domainID` and `userID` filters, so authorization logic is untested.
- `middleware_test.go` tests CORS and rate limiting but does not test `RequireSession` or `RequireDomainAdmin`.
- `smtp_test.go` does not test `Mail`, `Rcpt`, or `Data` commands.

---

## 7. Operational Concerns

### 7.1 Observability
- **No structured metrics**: No Prometheus, StatsD, or OpenTelemetry metrics. Queue depth, auth failures, SMTP commands, IMAP operations are invisible.
- **No distributed tracing**: No trace IDs propagated to Postmark or between worker jobs.
- **Logging lacks severity tuning**: `logger.New` hardcodes `slog.LevelInfo` with no env override in production.
- **No request logging for IMAP/SMTP**: Only HTTP requests are logged.

### 7.2 Deployment & Configuration
- **Config file path is hardcoded to `/etc/postnest/postnest.conf`**: No graceful fallback if the file is missing (it does skip, but the default path is Unix-specific).
- **Dockerfile uses `root`**: `Dockerfile.server`, `Dockerfile.worker`, `Dockerfile.migrate` do not create a non-root user.
- **No graceful shutdown for IMAP**: `imap.Server.Stop()` calls `srv.Close()` but does not wait for active connections to finish.
- **No readiness probe endpoint**: `/healthz` is used for liveness but doubles as readiness. A separate readiness endpoint that checks worker registration would be safer.

### 7.3 Data Retention & Compliance
- **No automated message retention**: No policy engine to delete messages older than N days.
- **No attachment deduplication**: Identical attachments across messages are stored separately.
- **No backup/restore API**: No export of user mailboxes or domain data.

---

## 8. Code Quality & Maintainability

### 8.1 Duplication
- **`isClosedErr` duplicated** in `internal/imap/imap.go` and `internal/smtp/smtp.go`.
- **`context.Background()` auth pattern duplicated** in IMAP `Login`, SMTP `Auth`, and SMTP `Mail`.
- **`writeJSON` duplicated** in `cmd/server/main.go` and `internal/webmail/webmail.go`.

### 8.2 Magic Numbers & Strings
- `10000` used as IMAP message limit in multiple places.
- `5 * time.Second` used as SMTP auth timeout, webmail domain lookup timeout, and Redis ping timeout.
- `30*time.Second` used as shutdown timeout in server and worker.
- `720*time.Hour` (30 days) hardcoded as cert renewal threshold.
- Color `#4285f4` hardcoded as default label color in schema, migrations, and Go code.

### 8.3 Interface Bloat
- `mailstore.Store` has 25 methods, making it a "fat interface." The mock in tests must implement all of them, even though most endpoints only use a subset.
- `auth.Service` mixes authentication, user management, domain queries, and session management. Separation into `UserStore`, `SessionStore`, and `DomainStore` would improve testability.

---

## 9. IMAP/SMTP Protocol Compliance

### 9.1 IMAP
- **UIDs are not stable**: `messageUID()` derives UIDs from the first 4 bytes of the UUID. This is not stable across mailbox reloads and can collide.
- **MODSEQ not implemented**: `Status` returns `UidValidity` derived from label ID but `MODSEQ` is not tracked, breaking CONDSTORE/QRESYNC.
- **BodyStructure is a stub**: `FetchBodyStructure` always returns `text/plain` with the length of `PlainText`, even for HTML or multipart messages.
- **No RFC822 source for outbound messages**: Outbound messages stored via SMTP `Data` may lack `Source`, so `FetchRFC822` returns an empty body.
- **Search is a pass-through**: `SearchMessages` ignores criteria and returns all message UIDs.

### 9.2 SMTP
- **No `SIZE` extension advertisement**: `maxMsgSize` is enforced in `Data` but not advertised in `EHLO` response.
- **No `8BITMIME` or `SMTPUTF8` support**.
- **Bcc addresses are exposed**: `Data` stores `BccAddresses` in the message record. While Postmark strips Bcc, local storage does not.
- **No queueing for outbound SMTP**: Messages are relayed to Postmark synchronously in `Data`. A Postmark outage causes immediate 451/550 errors to the client.

---

## 10. Recommended Priority Order

| Priority | Concern | Files |
|----------|---------|-------|
| **P0** | SQL injection via `fmt.Sprintf` ORDER BY | `mailstore/pgstore.go` |
| **P0** | `UpdateSearchVector` is a no-op | `mailstore/pgstore.go` |
| **P0** | Missing `delivery_logs` on outbound send | `smtp/smtp.go`, `workers/send.go` |
| **P1** | No STARTTLS support | `smtp/smtp.go`, `imap/imap.go` |
| **P1** | CalDAV entirely stubbed | `dav/dav.go` |
| **P1** | Unbounded rate limiter memory growth | `api/middleware.go` |
| **P1** | No Postmark HTTP timeout | `postmark/postmark.go` |
| **P1** | Missing auth/account lockout tests | `auth/auth.go` |
| **P2** | HTML sanitization missing | `workers/inbound.go`, `webmail/webmail.go` |
| **P2** | Greylist decision not enforced | `reputation/reputation.go`, `workers/inbound.go` |
| **P2** | Spam processor missing | `cmd/worker/main.go` |
| **P2** | No metrics/observability | `cmd/server/main.go` |
| **P3** | Read replica unused | `db/db.go`, `mailstore/pgstore.go` |
| **P3** | Attachment deduplication | `mailstore/pgstore.go` |
| **P3** | IMAP UID stability | `imap/backend.go` |

---

*Generated from static analysis of commit at time of writing.*
