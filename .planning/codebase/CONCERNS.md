# Codebase Concerns

**Analysis Date:** 2026-05-18

## Tech Debt

**IMAP backend uses `context.Background()` everywhere**
- Issue: No request-scoped cancellation or timeouts on any IMAP operation
- Files: `internal/imap/backend.go` (lines 32, 53, 66, 75, 86, 121, 155, 233, 246, 282, 360, 390)
- Impact: Hanging DB queries block IMAP goroutines indefinitely; no graceful shutdown of in-flight ops
- Fix approach: Thread `context.Context` through the `go-imap` backend interface or wrap store calls with `context.WithTimeout`

**Crude sleep-based shutdown in IMAP and SMTP**
- Issue: Servers close listener then sleep 2 seconds hoping clients disconnect
- Files: `internal/imap/imap.go` (line 58), `internal/smtp/smtp.go` (line 72)
- Impact: In-flight transactions may be mid-flight when connections are killed; data loss or client confusion
- Fix approach: Track active connections with `sync.WaitGroup`, close gracefully after drain or timeout

**New Postmark client created per email send**
- Issue: `postmark.NewClient()` allocates an `http.Client` on every outbound message
- Files: `internal/postmark/postmark.go` (lines 51-52)
- Impact: TCP connection churn, TLS handshake overhead, no connection reuse
- Fix approach: Create one `postmark.Client` per domain/token at startup, store in a map, reuse

**Generic error mapping swallows all store failures**
- Issue: Every admin handler (and many API handlers) maps all errors to `api.ErrInternal`
- Files: `internal/admin/handler.go` (passim), `internal/api/auth.go`, `internal/webmail/handlers.go`
- Impact: Unique constraint violations, missing rows, and DB outages all return identical 500; impossible to debug from client side; logs must be checked
- Fix approach: Inspect `pgx` error codes (`pgconn.PgError.Code`) and map to `ErrValidation`, `ErrNotFound`, etc.

**Fail-open webhook deduplication**
- Issue: Redis `SetNX` error returns `true` (process the webhook)
- Files: `internal/webhook/webhook.go` (line 181)
- Impact: Redis outage causes duplicate inbound message processing, double delivery to users
- Fix approach: Return `false` on Redis error and surface 503 so Postmark retries later

**JSON detail strings built with `fmt.Sprintf` inside SQL**
- Issue: Unescaped bounce/delivery metadata injected into JSON via string formatting
- Files: `internal/workers/bounce.go` (line 45), `internal/workers/delivery.go` (line 43)
- Impact: Malicious Postmark payloads containing `"` can break JSON structure; potential SQL injection if quotes escape the JSON literal context
- Fix approach: Build a Go `map[string]any` and marshal with `json.Marshal` before passing as SQL parameter

**SQL column name injection in reputation updates**
- Issue: `fmt.Sprintf` injects a column name (`sent_count` or `received_count`) directly into SQL
- Files: `internal/reputation/reputation.go` (line 90)
- Impact: The value is hardcoded internally, but the pattern is unsafe; future refactors may accept user input
- Fix approach: Use a whitelist map or separate queries for each column; never `Sprintf` identifiers

**Rate limiter trusts `X-Forwarded-For` blindly**
- Issue: Takes the first entry of `X-Forwarded-For` as the client IP without validation
- Files: `internal/api/middleware.go` (lines 209-211)
- Impact: Any client can spoof the header to bypass rate limiting or evade blocks
- Fix approach: Only parse `X-Forwarded-For` when behind a trusted proxy; otherwise use `RemoteAddr`

**Web UI proxy panics on invalid config**
- Issue: `panic(err)` if `apiBaseURL` fails to parse
- Files: `internal/webui/proxy.go` (line 23)
- Impact: Process crash on malformed environment variable
- Fix approach: Return the error to `main` and exit cleanly with a descriptive log message

**Health endpoint ignores errors**
- Issue: Multiple Redis and DB metrics fetched with `_` error discards
- Files: `cmd/server/main.go` (lines 143-147)
- Impact: Health reports "up" even when metric queries fail; monitoring blind spots
- Fix approach: Capture errors and include them in the response body

**Thread update error silently ignored**
- Issue: `_, _ = tx.Exec(...)` when appending message to thread
- Files: `internal/mailstore/pgstore.go` (line 419)
- Impact: Thread may not list the message; orphaned message with no thread visibility
- Fix approach: Check and return the error, or wrap in `pgx` batch execution

**Config dual-system drift**
- Issue: Legacy `internal/config/config.go` (env-based) exists alongside TOML loader; both may be used inconsistently
- Files: `internal/config/config.go`, `cmd/server/main.go`
- Impact: Setting a value via env may not override TOML, or vice versa; source of truth unclear
- Fix approach: Remove the legacy env-only config package; standardize on TOML with documented env overrides

**Frontend uses `console.error` for runtime errors**
- Issue: No structured logging or error reporting service on the frontend
- Files: `web/src/components/*.jsx` (scattered)
- Impact: Errors in production are invisible unless users report them
- Fix approach: Centralize error handling; push to backend endpoint or Sentry

**No React error boundaries**
- Issue: Any unhandled exception in a component crashes the entire SPA
- Files: `web/src/App.jsx` and all route components
- Impact: White screen of death on JS errors; user must reload
- Fix approach: Add a top-level `<ErrorBoundary>` and per-section boundaries

## Known Bugs

**Empty `sandbox` attribute on message iframe**
- Symptoms: Inbound HTML runs with no sandbox restrictions (`""` is treated as no restrictions in some browsers)
- Files: `web/src/components/MessageView.jsx` (lines 113-117)
- Trigger: Open any HTML message
- Workaround: None; XSS risk from untrusted sender HTML
- Fix approach: Use `sandbox="allow-same-origin"` or stricter; rely on DOMPurify but do not trust it alone

**Admin listUsers does not deep-copy user structs before clearing password hash**
- Symptoms: The loop modifies slice elements directly; if `users` slice is reused or logged elsewhere, hash may leak
- Files: `internal/admin/handler.go` (lines 183-186)
- Trigger: Concurrent access or logging middleware inspecting response structs
- Workaround: Not a guaranteed leak, but unsafe mutation of store-returned data
- Fix approach: Deep-copy user objects into a DTO before clearing sensitive fields

## Security Considerations

**Postmark tokens stored plaintext in PostgreSQL**
- Risk: Database dump exposes domain Postmark API tokens
- Files: `internal/models/models.go` (Domain struct), `internal/admin/handler.go`
- Current mitigation: Standard DB access controls
- Recommendations: Encrypt at rest with AES-GCM and a key from env; decrypt only when needed for sending

**Argon2 parameters configurable but unbounded**
- Risk: Admin could set `memory=1` or extremely low values, weakening password hashes
- Files: `cmd/server/main.go` (config load), `internal/admin/handler.go` (hasher creation)
- Current mitigation: Defaults are reasonable
- Recommendations: Enforce minimum Argon2id parameters in code; reject config with values below OWASP minimums

**Missing separate rate limiting on auth endpoints**
- Risk: Brute-force password guessing against `/auth/login`
- Files: `internal/api/auth.go`
- Current mitigation: Global rate limiter (100 req/min) applies to all routes
- Recommendations: Stricter per-IP rate limit on auth endpoints; add account lockout or exponential backoff

**Webhook signature verification does not enforce constant-time comparison**
- Risk: Timing side-channel on HMAC comparison (if any; verify actual implementation)
- Files: `internal/webhook/webhook.go`
- Current mitigation: Standard string comparison used in some paths
- Recommendations: Use `hmac.Equal` or `crypto/subtle.ConstantTimeCompare`

**SMTP `ALLOW_INSECURE_AUTH` bypass**
- Risk: Development flag can be left on in production, exposing plaintext passwords
- Files: `cmd/server/main.go` (lines 228-235)
- Current mitigation: Refuses to start if no TLS and flag not set
- Recommendations: Add loud startup warning banner and metric; consider removing the flag entirely and requiring TLS

## Performance Bottlenecks

**No connection reuse for Postmark**
- Problem: New HTTP client per send means no keep-alive
- Files: `internal/postmark/postmark.go`
- Cause: `NewClient` called inside the send path
- Improvement path: Cache clients per domain token; reuse `http.Client` with `Transport.MaxIdleConns`

**IMAP `SEARCH` without index hints**
- Problem: Full-text search falls back to PostgreSQL `tsvector`; large mailboxes may scan heavily
- Files: `internal/mailstore/pgstore.go` (search query)
- Cause: Complex OR query across subject/sender/body without proper GIN index coverage confirmation
- Improvement path: Add `EXPLAIN ANALYZE` checks to migrations; ensure `GIN` index on `search_vector` is present and not bloated

**Redis `LLen` on large queues every health check**
- Problem: Health endpoint calls `LLen` on potentially massive lists
- Files: `cmd/server/main.go` (lines 143-144)
- Cause: `LLen` is O(1) but combined with other checks adds latency under load
- Improvement path: Cache health metrics for a few seconds; avoid calling on every `/healthz` request

## Fragile Areas

**Worker queue failure handling**
- Files: `internal/workers/*.go`, `internal/queue/queue.go`
- Why fragile: Dead-letter list grows unbounded if no consumer clears it; no retry backoff strategy visible
- Safe modification: Always test worker processing with a full Redis flush and restart
- Test coverage: No tests for worker retry, dead-letter, or idempotency

**CalDAV/CardDAV date parsing**
- Files: `internal/calendar/ical.go`, `internal/dav/*.go`
- Why fragile: iCalendar parsing has many edge cases (timezone, recurrence, all-day). Custom parsing can mishandle DST or malformed DTSTART
- Safe modification: Use `github.com/emersion/go-ical` consistently; add fuzz tests
- Test coverage: Limited to happy-path

**SMTP DATA handler does not enforce max size before streaming**
- Files: `internal/smtp/smtp.go`
- Why fragile: Large messages may OOM before `MaxMessageSize` check
- Safe modification: Stream to temp file with size counter, reject mid-stream if exceeded
- Test coverage: None for oversized messages

**Frontend state management scattered**
- Files: `web/src/components/Admin.jsx`, `web/src/components/Compose.jsx`
- Why fragile: Multiple components maintain local copies of API data; stale state after mutations
- Safe modification: Consolidate cache in a lightweight global store (React Query or Zustand)
- Test coverage: No frontend unit tests detected

## Scaling Limits

**Redis single-instance queue depth**
- Current capacity: LPUSH/BRPOP on a single Redis instance
- Limit: One Redis node; no clustering for queues
- Scaling path: Shard queues by domain or migrate to Redis Streams with consumer groups

**PostgreSQL connection pool**
- Current capacity: `cfg.MaxDBConns`
- Limit: Static pool size; no adaptive scaling
- Scaling path: Add `pgxpool` dynamic sizing or connection pooler (PgBouncer) documentation

**Attachment storage in PostgreSQL BYTEA**
- Current capacity: Single PG instance
- Limit: Large attachments bloat tables and slow backups
- Scaling path: Offload to S3-compatible object store; keep metadata in PG

## Dependencies at Risk

**`github.com/mrz1836/postmark`**
- Risk: Third-party wrapper around Postmark; if abandoned, no official Go SDK exists
- Impact: Security patches or API changes may lag
- Migration plan: Fork or wrap with internal interface; the project already wraps it lightly in `internal/postmark/postmark.go`

**ACME certificate manager (`lego`)**
- Risk: DNS provider integration (Cloudflare only) is narrow
- Impact: Users with other DNS providers cannot auto-provision TLS
- Migration plan: Add more DNS provider configs or document manual cert provisioning

## Missing Critical Features

**Retry logic for Postmark sends**
- Problem: No retry on transient Postmark errors
- Blocks: SMTP client gets immediate failure; user must retry manually

**Mailbox quota enforcement**
- Problem: No storage limit per user/domain
- Blocks: Unlimited growth; no billing tier enforcement

**Frontend test suite**
- Problem: No unit or integration tests for React components
- Blocks: Refactoring UI is high-risk; regressions go undetected

**IMAP QRESYNC state persistence**
- Problem: MODSEQ and UIDVALIDITY maintained but not backed up or replicated
- Blocks: Failover or migration loses client sync state

## Test Coverage Gaps

**Worker idempotency**
- What's not tested: Processing the same inbound webhook twice
- Files: `internal/workers/inbound.go`, `internal/workers/bounce.go`
- Risk: Duplicate messages or delivery logs on Redis/dedup failure
- Priority: High

**SMTP authentication edge cases**
- What's not tested: Invalid credentials, STARTTLS downgrade, oversized DATA
- Files: `internal/smtp/smtp.go`
- Risk: Auth bypass or DoS
- Priority: High

**Admin CRUD validation**
- What's not tested: Invalid UUIDs, empty names, SQL injection attempts
- Files: `internal/admin/handler.go`
- Risk: Panics or bad data persistence
- Priority: Medium

**CalDAV/CardDAV round-trip**
- What's not tested: Full PROPFIND/PUT/GET cycle with real DAV client
- Files: `internal/dav/*.go`
- Risk: Protocol breakage on refactor
- Priority: Medium

---

*Concerns audit: 2026-05-18*
