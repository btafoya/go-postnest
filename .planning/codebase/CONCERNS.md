# Codebase Concerns

**Analysis Date:** 2026-05-17

## Tech Debt

**Webmail domain scoping uses user.ID as fallback:**
- Issue: `internal/webmail/webmail.go:domainID()` falls back to `u.ID` when a user has no domain memberships, treating the user's UUID as a domain ID. Multiple handlers call this without validating the result.
- Files: `internal/webmail/webmail.go` (lines 66-79)
- Why: Placeholder during MVP multi-tenant implementation; actual domain context from `X-Domain-ID` header or `domain_members` lookup not yet wired.
- Impact: Cross-domain data leakage risk; queries may match nothing or the wrong rows silently.
- Fix approach: Enforce `X-Domain-ID` header on every request, validate membership in `RequireSession` or a new middleware, and return 400 when domain is missing.

**IMAP UIDs derived from message UUIDs instead of dedicated table:**
- Issue: The `imap_uids` table exists in schema but UIDs are computed deterministically from message UUIDs (`internal/imap/backend.go:messageUID`).
- Files: `internal/imap/backend.go`
- Why: Convenience during initial IMAP implementation.
- Impact: UID validity violations on message deletion/recreation; IMAP clients may see inconsistent UIDs across sessions.
- Fix approach: Populate `imap_uids` in `mailstore` on message creation and read from it in the IMAP backend.

**Search indexer bypasses database trigger:**
- Issue: `messages_update_search_vector()` trigger is defined in migrations, but `internal/search/search.go` and `internal/mailstore/pgstore.go` update `search_vector` via direct SQL.
- Files: `internal/search/search.go`, `internal/mailstore/pgstore.go`
- Why: Trigger was added after initial store implementation; not retrofitted.
- Impact: Dual maintenance path; risk of divergent indexing logic between trigger and application code.
- Fix approach: Remove application-side `UpdateSearchVector` and rely on the trigger; verify with tests.

**Auth package reimplements standard library functions:**
- Issue: `splitN` reimplements `strings.SplitN` and `constantTimeCompare` reimplements `crypto/subtle.ConstantTimeCompare`.
- Files: `internal/auth/auth.go`
- Why: Likely added before discovering stdlib equivalents.
- Impact: Maintenance burden and risk of subtle timing bugs in hand-rolled constant-time comparison.
- Fix approach: Delete `splitN` and `constantTimeCompare`; use `strings.SplitN` and `subtle.ConstantTimeCompare`.

**Config loader uses heavy reflection for env overrides:**
- Issue: `applyEnvOverrides` and `applySectionOverrides` walk structs reflectively at runtime to map env vars to fields.
- Files: `internal/config/loader.go` (lines 163-223)
- Why: Generic env override system without code generation.
- Impact: Fragile to struct changes; panics on unexpected types; hard to debug env override failures.
- Fix approach: Replace with explicit mapping or use a struct tag-based library like `envconfig`/`caarlos0/env`.

## Known Bugs

**GetThread ignores thread membership and returns all user messages:**
- Symptoms: Thread view shows every message in the mailbox, not just messages belonging to the requested thread.
- Trigger: Open any thread in the webmail API.
- Files: `internal/mailstore/pgstore.go:GetThread` (lines 310-316)
- Root cause: Implementation calls `ListMessages(ctx, domainID, userID, nil, ListOptions{Limit: 1000, ...})` with no thread filter, completely ignoring the thread's `message_ids` array.
- Fix: Filter `ListMessages` by the thread's `message_ids` or query messages directly with `id = ANY(thread.message_ids)`.

**FindOrCreateThread passes raw subject as subject_hash:**
- Symptoms: Thread matching fails or creates duplicate threads when subjects differ only in whitespace/case.
- Trigger: Receive two inbound messages with similar subjects.
- Files: `internal/mailstore/pgstore.go:FindOrCreateThread` (lines 337-358)
- Root cause: The function passes the raw `subject` string as `$4` to match against the `subject_hash` column, but the column expects a hashed value. The `ON CONFLICT (domain_id, user_id, subject_hash)` may never fire.
- Fix: Compute a stable hash (e.g., lowercase + normalized whitespace) and use it consistently for both insert and conflict detection.

**Greylist deferral has no retry logic:**
- Symptoms: Greylisted messages are silently accepted on first arrival instead of being deferred.
- Trigger: Inbound mail from an unknown sender triggers greylist evaluation.
- Files: `internal/reputation/reputation.go:EvaluateInbound`
- Root cause: The reputation engine returns `DecisionDefer` but the inbound worker does not act on it — there is no delayed retry queue for deferred messages.
- Fix: Implement a `delayed_defer` job type or add greylist-aware retry in the inbound worker.

## Security Considerations

**Webhook signature verification has insecure fallback:**
- Risk: If `X-Postmark-Signature` header is absent, the webhook handler falls back to comparing `X-Postmark-Server-Token` against the configured secret.
- Files: `internal/webhook/webhook.go:verifySignature` (lines 65-85)
- Current mitigation: Token comparison still requires knowledge of the secret.
- Recommendations: Remove the fallback entirely; reject webhooks with missing signatures. Document that the secret must be configured.

**No CSRF protection on REST API:**
- Risk: Authenticated POST/PATCH/DELETE endpoints rely solely on the session cookie; no CSRF token or double-submit cookie pattern.
- Files: `internal/webmail/webmail.go` (all mutating handlers), `internal/api/middleware.go`
- Current mitigation: `SameSite=Lax` on session cookie provides partial protection.
- Recommendations: Add `X-CSRF-Token` validation or implement double-submit cookie pattern for state-changing requests.

**SMTP and IMAP have no brute-force protection:**
- Risk: Unlimited authentication attempts against SMTP/IMAP without rate limiting.
- Files: `internal/smtp/smtp.go`, `internal/imap/backend.go`
- Current mitigation: None (HTTP has `RateLimiter` middleware, but it does not cover SMTP/IMAP).
- Recommendations: Add per-source IP rate limiting or account lockout for failed SMTP/IMAP login attempts.

**HTML sanitization may over-strip legitimate content:**
- Risk: `bluemonday.UGCPolicy()` is applied to draft HTML bodies; some email formatting (styles, inline images) may be stripped.
- Files: `internal/webmail/webmail.go:createDraft`, `internal/webmail/webmail.go:updateDraft`
- Current mitigation: Policy prevents XSS.
- Recommendations: Define an email-specific HTML policy that allows safe inline styles and cid: references for embedded images.

**AllowInsecureAuth flag permits cleartext credentials:**
- Risk: When no TLS is configured, `allowInsecureAuth=true` enables AUTH PLAIN over unencrypted SMTP/IMAP.
- Files: `cmd/server/main.go` (default branch), `internal/smtp/smtp.go`
- Current mitigation: Disabled by default; requires explicit env override.
- Recommendations: Remove the flag entirely and mandate TLS for authentication. If truly needed for local dev, gate it behind build tags.

## Performance Bottlenecks

**IMAP loads entire mailbox into memory:**
- Problem: `ListMessages` in IMAP fetches up to `maxIMAPBatchSize = 5000` messages with all columns into a slice.
- Files: `internal/imap/backend.go:ListMessages` (line 148)
- Measurement: 5000 messages × ~2KB each = ~10MB per IMAP LIST/FETCH; grows linearly.
- Cause: No cursor-based pagination; all messages matching the sequence set are materialized.
- Improvement path: Implement server-side cursor pagination for large mailboxes or fetch message metadata only, loading bodies on demand.

**Search query lacks prepared statement caching:**
- Problem: Full-text search queries use `plainto_tsquery('english', $3)` inline, which cannot be prepared by pgx because the tsquery function is in the query text.
- Files: `internal/mailstore/pgstore.go:Search` (lines 454-490)
- Cause: Dynamic `ORDER BY` clause also prevents prepared statement reuse.
- Improvement path: Use a stored procedure or accept the plan cache miss; add `tsquery` parameterization via `to_tsquery` with pre-parsed input.

**ListMessages counts total before paginating:**
- Problem: Every label/message list performs a `COUNT(*)` followed by the data query.
- Files: `internal/mailstore/pgstore.go:ListMessages` (lines 113-127)
- Cause: Standard pagination pattern.
- Improvement path: Use keyset pagination (`cursor` + `LIMIT`) to eliminate the expensive count on large mailboxes.

## Fragile Areas

**Worker pool retry logic:**
- Files: `internal/workers/workers.go:worker` (lines 102-144)
- Why fragile: Retry count and dead-letter logic rely on Redis list ops that are not atomic with job state transitions. A crash between `RPush` (retry) and `LRem` (remove from processing) can duplicate jobs.
- Common failures: Duplicate processing of the same job under Redis connection blips.
- Safe modification: Wrap retry/dead-letter moves in a Lua script or Redis transaction.
- Test coverage: Workers have unit tests with miniredis, but no integration tests against real Redis failure modes.

**Certificate manager renewal loop:**
- Files: `internal/certmanager/manager.go`
- Why fragile: ACME renewal runs in a background goroutine with no external health exposure. Certificate expiry is only checked at startup and in the renewal loop; if the loop panics, the server continues with a stale cert.
- Common failures: DNS provider token expiry causes silent renewal failures; certificate expires without alerting.
- Safe modification: Add a `/healthz` check for certificate expiry window, and recover panics in `renewalLoop`.
- Test coverage: None.

**Webhook deduplication relies on Redis TTL only:**
- Files: `internal/webhook/webhook.go:dedup` (lines 120-140)
- Why fragile: Duplicate detection uses a 24-hour Redis TTL. If Redis is flushed or the TTL expires, duplicate webhooks re-process. No persistent dedup table.
- Common failures: Redis eviction under memory pressure causes duplicate inbound messages.
- Safe modification: Add a `processed_webhooks` table with a unique constraint on `(webhook_type, message_id)`.
- Test coverage: Only unit tests with miniredis; no persistence test.

**SMTP DATA handler parses entire message into memory:**
- Files: `internal/smtp/smtp.go:Data` (lines 149-250)
- Why fragile: The full message body is read into memory and parsed by `go-message/mail`, then relayed to Postmark. Large attachments can exhaust memory.
- Common failures: OOM on messages near `maxMessageSize`.
- Safe modification: Stream large bodies to temporary files and parse incrementally; enforce per-attachment size limits before buffering.
- Test coverage: SMTP tests only cover AUTH, not DATA relay or body parsing.

## Scaling Limits

**IMAP max batch size:**
- Current capacity: 5000 messages per mailbox query.
- Limit: Mailboxes with >5000 messages return truncated results, violating IMAP sequence numbering correctness.
- Symptoms at limit: IMAP clients show incomplete folders or corrupted message lists.
- Scaling path: Implement cursor-based message streaming for IMAP FETCH/LIST.

**Single Redis connection for queues and pub/sub:**
- Current capacity: One `go-redis` UniversalClient shared across job queues, webhook dedup, and IMAP IDLE pub/sub.
- Limit: Redis `BLPOP` blocking calls can delay pub/sub message delivery; contention under high load.
- Symptoms at limit: IMAP IDLE latency spikes; webhook processing delays.
- Scaling path: Separate Redis clients for pub/sub and queue operations, or shard queues by processor type.

## Dependencies at Risk

**emersion/go-ical, go-sasl, go-vcard at pseudo-versions:**
- Risk: All three emersion libraries track unstable pseudo-versions (`v0.0.0-...`). API changes in future commits could break CalDAV/CardDAV/IMAP/SMTP integration.
- Impact: Build breakage on `go get -u`; no semantic versioning guarantees.
- Migration plan: Pin to specific commit hashes in `go.mod` or vendor the libraries. Monitor for tagged releases.

**github.com/yuin/gopher-lua (indirect):**
- Risk: Pulled in via `go-acme/lego/v4` → `microcosm-cc/bluemonday` dependency chain. Used for Lua scripting in lego DNS provider.
- Impact: Unnecessary dependency surface for a mail server; CVE exposure.
- Migration plan: Evaluate whether `bluemonday` can be replaced with a lighter HTML sanitizer, or switch to `microcosm-cc/bluemonday/v2` if it reduces indirect deps.

## Missing Critical Features

**Admin REST handlers:**
- Problem: Only `/admin/api/v1/domains` exists as an inline handler in `cmd/server/main.go`. No user provisioning, domain management, delivery logs, or reputation dashboards.
- Current workaround: Direct database access or ad-hoc scripts.
- Blocks: Multi-tenant self-service onboarding.
- Implementation complexity: Medium (requires additional middleware, handlers, and store methods).

**STARTTLS for IMAP/SMTP:**
- Problem: TLS is all-or-nothing (either ACME/static certs at startup or plaintext). No STARTTLS upgrade on existing listeners.
- Current workaround: Run on separate TLS ports (993/465) or without encryption.
- Blocks: Standard email client configurations that expect STARTTLS on 143/587.
- Implementation complexity: Low-Medium (go-imap and go-smtp both support STARTTLS).

**CalDAV implementation:**
- Problem: All CalDAV methods return `fmt.Errorf("not implemented")`.
- Files: `internal/dav/dav.go` (caldavBackend stubs)
- Current workaround: None.
- Blocks: Calendar sync with Apple/Google clients.
- Implementation complexity: High (requires `calendar_events` table, recurrence handling, iCalendar parsing).

**Asynchronous search indexing:**
- Problem: `Indexer.Queue` updates `search_vector` synchronously via direct SQL. No background queue for index updates.
- Files: `internal/search/search.go`
- Current workaround: Synchronous updates on message creation.
- Blocks: High-volume inbound processing throughput.
- Implementation complexity: Low (add `search_index` job type to worker pool).

## Test Coverage Gaps

**Mailstore package (zero tests):**
- What's not tested: All 25+ methods of `PGStore` — CreateMessage, UpdateMessage, ListMessages, Search, thread management, label CRUD, attachment handling.
- Risk: SQL errors, transaction bugs, and schema mismatch regressions go undetected.
- Priority: High
- Difficulty to test: Medium; requires PostgreSQL testcontainer or `pgxmock`.

**Auth package (zero tests):**
- What's not tested: Argon2id hashing, session lifecycle, API key validation, password updates, domain membership checks.
- Risk: Authentication bypass, session fixation, or hash parameter changes breaking logins.
- Priority: High
- Difficulty to test: Low-Medium; requires `pgxmock` or test database.

**IMAP backend (zero tests):**
- What's not tested: Mailbox listing, FETCH, SEARCH, APPEND, EXPUNGE, COPY, flag updates, UID mapping.
- Risk: Protocol compliance regressions, data corruption under concurrent access.
- Priority: High
- Difficulty to test: High; requires an IMAP client library in tests or the go-imap test helpers.

**Certmanager (zero tests):**
- What's not tested: ACME registration, certificate loading, renewal logic, DNS provider setup.
- Risk: Certificate expiry in production due to untested renewal paths.
- Priority: Medium
- Difficulty to test: High; requires ACME staging server mocking.

**Postmark client (zero tests):**
- What's not tested: SendEmail, ParseInbound, error handling for Postmark API responses.
- Risk: Payload serialization bugs or response parsing failures on live API changes.
- Priority: Medium
- Difficulty to test: Low; mock HTTP server sufficient.

**SMTP DATA relay (zero tests):**
- What's not tested: End-to-end message parsing, Postmark relay, Sent copy persistence.
- Files: `internal/smtp/smtp.go:Data`
- Risk: Message corruption or relay failure in production.
- Priority: High
- Difficulty to test: Medium; requires mock Postmark server and MIME fixture data.

---

*Concerns audit: 2026-05-17*
*Update as issues are fixed or new ones discovered*
