# Codebase Concerns

**Analysis Date:** 2026-05-18

## Tech Debt

**WebUI Proxy Panic on Misconfiguration:**
- Issue: `NewAPIProxy` calls `panic(err)` if the API base URL is invalid instead of returning an error for graceful handling.
- Files: `internal/webui/proxy.go:23`
- Impact: A malformed env var crashes the webui process on startup with no diagnostic log output.
- Fix approach: Return `(*APIProxy, error)` and handle it in `cmd/webui/main.go`.

**Search Indexer No-Op Queue:**
- Issue: `search.Indexer.ProcessQueue` is a hardcoded no-op because `Queue` performs synchronous updates. There is no actual async indexing pipeline.
- Files: `internal/search/search.go:36-39`
- Impact: Large inbound volumes will trigger full-text updates inline, blocking webhook acknowledgment and inbound processing.
- Fix approach: Implement a Redis-backed queue or use PostgreSQL `LISTEN/NOTIFY` to defer indexing.

**Spam Processor Incomplete Reputation Update:**
- Issue: `SpamProcessor.Process` logs that it cannot update reputation because `domain_id` is not present in the webhook payload. The code path is a stub.
- Files: `internal/workers/spam.go:40-44`
- Impact: Spam complaints are logged but never affect contact reputation scores.
- Fix approach: Enrich the webhook payload with `domain_id` before enqueueing in `internal/webhook/webhook.go`.

**Admin Endpoint Inline SQL:**
- Issue: The admin dashboard endpoint in `cmd/server/main.go` runs raw `SELECT COUNT(*)` inline and builds DTOs in the route closure instead of using a service layer.
- Files: `cmd/server/main.go:121-182`
- Impact: Admin logic is tightly coupled to the server main file, untestable, and duplicates store patterns.
- Fix approach: Extract admin handlers into `internal/admin` with proper stores.

**Auth Custom String Splitting:**
- Issue: `auth.Service` implements a hand-rolled `splitN` function instead of using `strings.SplitN`.
- Files: `internal/auth/auth.go:64-77`
- Impact: Unnecessary maintenance surface; standard library function is identical and better tested.
- Fix approach: Replace with `strings.SplitN`.

**Dual Router Frameworks:**
- Issue: The API server uses `chi`, but the webui uses `gin`. Two frameworks add compile time, dependency surface, and cognitive overhead for no architectural benefit.
- Files: `internal/webui/router.go`, `cmd/server/main.go`
- Impact: Divergent middleware patterns (CORS, logging, recovery), duplicate dependency trees.
- Fix approach: Standardize on chi for both; remove gin from webui.

**Calendar Handler Ignores CTag Bump Errors:**
- Issue: `BumpCTag` return values are discarded on create, update, and delete event handlers.
- Files: `internal/calendar/handler.go:171`, `212`, `235`
- Impact: CalDAV clients may see stale synchronization tokens without server-side awareness.
- Fix approach: Log or return the error.

**Contact Update Handler Calls Create:**
- Issue: `contacts.Handler.updateContact` invokes `store.Create` (relying on `ON CONFLICT`) instead of a dedicated update method.
- Files: `internal/contacts/handler.go:154`
- Impact: Semantically confusing, masks missing fields, and makes partial updates impossible.
- Fix approach: Add `Update` to `contacts.Store` and use it.

## Known Bugs

**Message List Never Returns Labels:**
- Issue: `toMessageDTOs` always passes `nil` for labels, so the message list JSON never includes label names even though the frontend contract supports them.
- Files: `internal/webmail/dto.go:93-99`
- Trigger: Any call to `GET /api/v1/messages`.
- Workaround: Fetch labels per message client-side.

**IMAP UID Collision Risk:**
- Issue: `messageUID` derives a 32-bit UID from the first 4 bytes of a UUIDv7. UUIDv7 encodes a timestamp in the first bytes, so messages created in the same millisecond will share the same prefix and collide.
- Files: `internal/imap/backend.go:420-425`
- Trigger: Bulk import or rapid message ingestion.
- Workaround: None; IMAP UIDs are not stable across sessions.

**Webhook Dedup Fails Open:**
- Issue: `webhook.Handler.dedup` returns `true` (process the webhook) when Redis returns an error, meaning duplicate webhooks are processed on Redis outages.
- Files: `internal/webhook/webhook.go:179-183`
- Trigger: Redis unavailability during Postmark webhook delivery.
- Workaround: Monitor dead-letter queue for duplicates manually.

**Legacy Token Fallback Allows Empty Secret:**
- Issue: If `POSTMARK_WEBHOOK_SECRET` is empty and the inbound request omits the HMAC signature header, the legacy token check (`token != "" && token == h.secret`) evaluates to false, which is safe. However, if the secret is non-empty but the request sends an empty `X-Postmark-Server-Token`, it also evaluates to false. The real risk is that the HMAC path does not reject empty secrets explicitly.
- Files: `internal/webhook/webhook.go:51-63`
- Trigger: Misconfigured secret with legacy token header.
- Workaround: Always configure a non-empty secret and use HMAC.

**Calendar Event Missing All-Day Flag Parsing:**
- Issue: `ICSToEvent` parses DTSTART/DTEND but does not inspect the VALUE=DATE parameter to set `AllDay`.
- Files: `internal/calendar/ical.go:57-87`
- Trigger: Importing an all-day event via CalDAV or ICS upload.
- Workaround: Frontend workaround not possible for DAV clients.

**SMTP Server Message Read into Memory:**
- Issue: `smtpSession.Data` reads the entire message body into a byte slice with `io.ReadAll`, even for multi-megabyte messages.
- Files: `internal/smtp/smtp.go:173-203`
- Trigger: Any SMTP DATA submission.
- Workaround: `maxMsgSize` limiter caps it, but still allocates up to the limit.

## Security Considerations

**Rate Limiter IP Spoofing via X-Forwarded-For:**
- Risk: `api.RateLimiter` trusts `X-Forwarded-For` without validating the source, allowing clients to bypass rate limits by spoofing headers.
- Files: `internal/api/middleware.go:208-211`
- Current mitigation: None.
- Recommendations: Implement a trusted proxy list or use `RemoteAddr` exclusively when behind a load balancer.

**Attachment Upload Memory Exhaustion:**
- Risk: `webmail.Handler.uploadAttachment` allocates a byte slice equal to the uploaded file size and reads it entirely into memory. No streaming to disk or object storage occurs.
- Files: `internal/webmail/webmail.go:595-599`
- Current mitigation: `maxAttachmentSize` limits the total form size, but the allocation is still heap-bound.
- Recommendations: Stream attachments to a temporary file or external storage (S3/MinIO) and store a reference.

**Insecure Auth Enabled by Default in Docker Compose:**
- Risk: `docker-compose.yml` sets `POSTNEST_SECURITY_ALLOW_INSECURE_AUTH=true`, allowing plaintext SMTP/IMAP authentication in containerized deployments.
- Files: `docker-compose.yml:42`
- Current mitigation: Commented as dev-only, but the default is dangerous.
- Recommendations: Default to `false` and document the override for local development.

**DAV Basic Auth Without Rate Limiting:**
- Risk: CardDAV/CalDAV endpoints use HTTP Basic Auth with no rate limiting or account lockout, making them susceptible to brute-force attacks.
- Files: `internal/dav/dav.go:70-86`
- Current mitigation: None.
- Recommendations: Apply the same per-IP rate limiter used on the REST API to DAV routes.

**Weak Email Validation in WebUI:**
- Risk: The Gin custom validator for email only checks for `@` and `.`, allowing malformed addresses through.
- Files: `internal/webui/router.go:162-165`
- Current mitigation: Backend in `webmail.go` uses `mail.ParseAddress` for drafts.
- Recommendations: Use a proper regex or library validator in the webui or remove the custom validator entirely.

**Certificate File Permissions:**
- Risk: `certmanager.Manager.obtainCertificate` writes the certificate with `0644` permissions (world-readable). While certificates are public, some deployment environments enforce stricter file modes.
- Files: `internal/certmanager/manager.go:347`
- Current mitigation: None.
- Recommendations: Write with `0640` or `0600` and document the permission requirement.

## Performance Bottlenecks

**Message List Count Starvation:**
- Problem: Every `ListMessages` call executes a separate `COUNT(*)` query over the full filtered result set before fetching the page.
- Files: `internal/mailstore/pgstore.go:130-138`
- Cause: Eager total count for pagination UI.
- Improvement path: Use PostgreSQL `COUNT(*) OVER()` in the same query, or cache approximate counts in Redis per label.

**IMAP Full Mailbox Load:**
- Problem: `imapMailbox.ListMessages` loads up to 5000 full message rows (including `html_body` and `plain_text`) into memory for every IMAP FETCH, even when the client only requests flags or UIDs.
- Files: `internal/imap/backend.go:153-159`
- Cause: Single query fetches all columns regardless of requested fetch items.
- Improvement path: Split into a lightweight metadata query and a body-on-demand query.

**Synchronous Search Vector Update:**
- Problem: `InboundProcessor` calls `UpdateSearchVector` inline after storing each inbound message, adding a second UPDATE per message.
- Files: `internal/workers/inbound.go:156-159`
- Cause: No async indexing pipeline.
- Improvement path: Use a PostgreSQL trigger (migration `000004_fts_trigger.up.sql` exists but may not cover all paths) or background queue.

**Redis Delayed Job Promotion Race:**
- Problem: `PromoteReadyDelayed` is not atomic: it reads from the sorted set, pushes to the list, then removes from the sorted set. A crash or retry between push and rem duplicates jobs.
- Files: `internal/redis/redis.go:57-74`
- Cause: Missing Lua script or Redis transaction.
- Improvement path: Wrap promote logic in a Lua script executed atomically.

## Fragile Areas

**Thread FindOrCreate Transaction:**
- Files: `internal/mailstore/pgstore.go:401-457`
- Why fragile: Uses `FOR UPDATE`, `ON CONFLICT DO NOTHING`, and a fallback `SELECT` in the same transaction. Correct but subtle; any schema change to the unique constraint breaks the logic silently.
- Safe modification: Add an integration test that fires concurrent inserts for the same thread.
- Test coverage: None; `pgstore.go` has zero tests.

**Postmark Client Per-Request Instantiation:**
- Files: `internal/postmark/postmark.go:50-52`
- Why fragile: Creates a new `lib.Client` and a new `http.Client` on every `SendEmail` call, preventing connection reuse and making timeouts non-deterministic.
- Safe modification: Cache the `lib.Client` or the HTTP transport in the `postmark.Client` struct.
- Test coverage: `postmark/postmark.go` has no tests.

**Domain Fallback in Multi-Domain Users:**
- Files: `internal/webmail/webmail.go:75-90`, `internal/calendar/handler.go:43-60`, `internal/dav/dav.go:93-99`
- Why fragile: When no explicit domain is provided, the code falls back to the user's first domain (`ORDER BY created_at ASC`). This is arbitrary and breaks when users belong to multiple domains.
- Safe modification: Require explicit `domain_id` on every mutating request and reject with 400 if missing.
- Test coverage: No tests cover multi-domain behavior.

**Worker Dead Letter Queue Opacity:**
- Files: `internal/workers/workers.go:159-167`
- Why fragile: Dead jobs are pushed to `queue:jobs:dead` with no metrics, no alerting, and no replay mechanism. Failures accumulate silently.
- Safe modification: Expose dead queue depth via metrics endpoint and add a CLI replay command.
- Test coverage: `workers_test.go` exists but does not test dead-letter behavior.

**WebUI Embedded Dist Check:**
- Files: `internal/webui/router.go:69-92`
- Why fragile: The `NoRoute` handler manually reads `dist/index.html` and `dist` static assets. If the frontend build is missing, the server returns a plaintext error instead of a proper 404/500.
- Safe modification: Verify `distFS` at startup and fail fast; serve static files with `http.FileServer` over the embedded FS.
- Test coverage: `DistFS()` helper is tested implicitly but not the router fallback logic.

## Scaling Limits

**In-Memory Rate Limiter:**
- Current capacity: 100 requests/minute per IP, stored in a process-local map.
- Limit: Cannot scale horizontally; each server instance maintains its own counters. A client can bypass limits by hitting different replicas.
- Scaling path: Move counters to Redis with sliding-window keys.

**SSE Hub Process Local:**
- Current capacity: In-memory `map[chan string]bool` for SSE clients.
- Limit: Broadcasts do not cross process boundaries. Multiple webui replicas mean users connected to different instances will not receive real-time events.
- Scaling path: Route all SSE connections through a single Redis pub/sub channel and use Redis as the fan-out backbone.

**IMAP Batch Ceiling:**
- Current capacity: `maxIMAPBatchSize = 5000` messages per query.
- Limit: Mailboxes with more than 5000 messages will have truncated IMAP views. Sequence numbering will be incorrect for messages beyond the limit.
- Scaling path: Implement cursor-based pagination or PostgreSQL `OFFSET` iteration in IMAP backend.

**Monolithic Server Process:**
- Current capacity: HTTP, IMAP, SMTP, and ACME renewal all run in `cmd/server`.
- Limit: A memory leak or panic in any one subsystem (e.g., IMAP fetch) affects the entire process. Vertical scaling only.
- Scaling path: Split IMAP and SMTP into standalone deployable binaries (the `cmd/` layout already supports this but `docker-compose.yml` only runs the monolith).

## Dependencies at Risk

**go-imap / go-smtp (emersion):**
- Risk: Niche libraries with a small maintainer pool. `go-imap` v1 is stable but v2 is in development; future breaking changes may strand the codebase.
- Impact: IMAP and SMTP server functionality.
- Migration plan: Monitor emersion repositories for v2 announcements and evaluate migration effort early.

**mrz1836/postmark:**
- Risk: Third-party wrapper around Postmark API. If the library falls behind Postmark API changes (e.g., new fields, deprecated endpoints), outbound mail breaks.
- Impact: All outbound email via Postmark.
- Migration plan: Vendor the thin wrapper internally or switch to a raw HTTP client for critical paths.

**lego (ACME):**
- Risk: Large dependency tree for a single feature. ACME protocol changes or DNS provider deprecation could force upgrades across many transitive packages.
- Impact: Certificate issuance and renewal.
- Migration plan: Evaluate whether `golang.org/x/crypto/acme/autocert` is sufficient for the single-domain use case.

## Missing Critical Features

**Read Replica Support:**
- Problem: `Config` defines `PostgresReadDSN` but `db.New` and all stores ignore it. Every query hits the primary.
- Blocks: Horizontal read scaling for large mailboxes.

**Async Search Indexing:**
- Problem: The `search` package has a `ProcessQueue` stub. FTS updates are synchronous.
- Blocks: High-throughput inbound processing without latency spikes.

**Contact Partial Update:**
- Problem: No `Update` method exists on `contacts.Store`; updates are upserts that require sending all fields.
- Blocks: Efficient contact editing from mobile clients.

**Attachment Streaming / Object Storage:**
- Problem: Attachments are stored as `BYTEA` in PostgreSQL and loaded entirely into memory.
- Blocks: Scalability beyond small attachment volumes; database bloat.

**Audit Logging:**
- Problem: No audit trail for admin actions (domain creation, password resets, user additions).
- Blocks: Compliance requirements (SOC 2, etc.).

**IMAP/SMTP STARTTLS:**
- Problem: TLS is either on or off at the listener level. No STARTTLS upgrade path for clients that require it.
- Blocks: Compatibility with some MUAs and corporate firewalls.

## Test Coverage Gaps

**Untested Core Persistence:**
- What's not tested: All CRUD operations in `mailstore.PGStore`, `calendar.PGStore`, `contacts.PGStore`.
- Files: `internal/mailstore/pgstore.go`, `internal/calendar/pgstore.go`, `internal/contacts/contacts.go`
- Risk: Schema changes, query regressions, and transaction deadlocks go undetected.
- Priority: High.

**Untested Protocol Backends:**
- What's not tested: IMAP backend message listing, flag updates, expunge, and copy. SMTP session auth, mail, rcpt, and data.
- Files: `internal/imap/backend.go`, `internal/smtp/smtp.go`
- Risk: Protocol regressions break email clients.
- Priority: High.

**Untested Auth Service:**
- What's not tested: Session creation, validation, expiry, API key flows, and password hashing edge cases.
- Files: `internal/auth/auth.go`
- Risk: Authentication bypass or session fixation bugs.
- Priority: Critical.

**Untested Webhook Handlers:**
- What's not tested: Inbound, bounce, delivery, and spam webhook HTTP handlers and signature verification.
- Files: `internal/webhook/webhook.go`
- Risk: Malformed Postmark payloads cause panics or unauthorized queueing.
- Priority: High.

**Untested Worker Processors:**
- What's not tested: `SendProcessor`, `InboundProcessor`, `BounceProcessor`, `SpamProcessor`, `DeliveryProcessor` end-to-end against real stores.
- Files: `internal/workers/*.go`
- Risk: Job processing regressions break email delivery.
- Priority: High.

**Untested Certificate Manager:**
- What's not tested: ACME registration, certificate loading, renewal loop, and DNS provider setup.
- Files: `internal/certmanager/manager.go`
- Risk: TLS expiration in production due to renewal logic bugs.
- Priority: Medium.

**No Integration Tests:**
- What's not tested: No test spins up PostgreSQL + Redis + server and exercises the full request lifecycle.
- Risk: Middleware ordering bugs, CORS misconfigurations, and database constraint violations only surface in manual testing.
- Priority: High.

---

*Concerns audit: 2026-05-18*
