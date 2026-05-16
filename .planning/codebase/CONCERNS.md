# PostNest Codebase Concerns

This document catalogs technical debt, security gaps, performance bottlenecks, fragile logic, and operational risks found in the Go-PostNest codebase. Every item includes the concrete file path where the issue lives.

---

## 1. Technical Debt

### TODOs / Placeholders / Stubs
- `cmd/server/main.go:103` — Admin REST handler group is mounted but contains only a `// TODO: admin handlers` comment; no actual endpoints.
- `internal/imap/backend.go:127` — `Status` reports `UidNext = 1` with a `// TODO` comment. `UidValidity` is hard-coded to `1`.
- `internal/webmail/webmail.go:60` — `listLabels` contains `// TODO: compute unread/total counts per label`; counts are omitted from the response.
- `internal/webmail/webmail.go:75` — `createLabel` uses `DomainID: u.ID` with a `// TODO: use domain context` comment. The user's UUID is used as a domain placeholder.
- `internal/webmail/webmail.go:267` — `createDraft` contains `// TODO: parse To into to_addresses`; the `To` field is decoded and then ignored (`_ = msg`).
- `internal/dav/dav.go:261` — Entire CalDAV backend is a stub; every operation returns `fmt.Errorf("not implemented")`.
- `internal/webmail/webmail.go:87` — `updateLabel` is a no-op that always returns `ErrNotFound`.
- `internal/webmail/webmail.go:272` — `updateDraft` is a no-op that returns `200 OK` without persisting anything.
- `internal/webmail/webmail.go:276` — `sendDraft` is a stub that returns `{ "status": "queued" }` without actually enqueuing a job.
- `internal/imap/backend.go:92` — `RenameMailbox` returns `fmt.Errorf("rename not supported")`.
- `internal/dav/dav.go:135` — `CreateAddressBook` returns `fmt.Errorf("create not supported")`.
- `internal/dav/dav.go:138` — `DeleteAddressBook` returns `fmt.Errorf("delete not supported")`.

### Deprecated / Brittle Patterns
- `internal/imap/backend.go` — Every IMAP handler instantiates `ctx := context.Background()` instead of deriving a context from the connection or a timeout. This makes cancellation and deadline propagation impossible.
- `internal/db/db.go:25` — `db.New` uses `context.Background()` for pool creation and the initial ping. Startup will hang indefinitely if PostgreSQL is unreachable.
- `internal/redis/redis.go:23` — `redis.New` uses `context.Background()` for the initial ping. Same hang risk.
- `internal/api/errors.go:64` — `As` is a hand-rolled wrapper around `errors.As` that only handles `*AppError` and ignores wrapped errors. Any error wrapped with `fmt.Errorf("...: %w", err)` will fall through to `ErrInternal` even when the root cause is a known `*AppError`.
- `internal/postmark/postmark.go:48` — `SendEmail` creates a new `lib.NewClient(apiToken, "")` on every invocation. The underlying library may create new HTTP transports each time, defeating connection reuse.
- `internal/workers/workers.go:97` — `Enqueue` and `worker` both use `time.Now().UnixNano()` as the job ID. Under burst load these IDs can collide because nanosecond precision is not guaranteed unique across goroutines.
- `internal/imap/backend.go:391` — `messageUID` derives the IMAP UID from the first four bytes of the message UUID (`uint32(msg.ID[0])<<24 | ...`). This is non-monotonic and collisions are likely.

---

## 2. Known Issues / Limitations

### Design-Level Gaps
- `INTEGRATION.md` §Known Limitations — Domain scoping in webmail handlers currently uses `user.ID` as a placeholder for `domainID`. The real domain context (from `X-Domain-ID` header or `domain_members` lookup) is not wired.
- `INTEGRATION.md` — The `imap_uids` table is defined in the schema but never populated by `mailstore`. UIDs are derived deterministically from message UUIDs, which breaks IMAP correctness.
- `INTEGRATION.md` — The `messages_update_search_vector()` trigger is created, but workers use direct SQL updates instead of the trigger. Worse, the direct SQL is broken (see Security / Performance sections).
- `INTEGRATION.md` — Greylist deferral / retry logic is simplified: no background delay is enforced.
- `INTEGRATION.md` — SMTP only supports `AUTH PLAIN`; `AUTH LOGIN` is not yet implemented.
- `INTEGRATION.md` — CardDAV only supports a single default address book.
- `INTEGRATION.md` — DAV auth uses Basic Auth only; Bearer token support is missing.

### Broken Search-Vector Update
- `internal/mailstore/pgstore.go:467` — `UpdateSearchVector` executes:
  ```sql
  SELECT messages_update_search_vector() WHERE id=$1
  ```
  This is syntactically invalid: the trigger function takes no arguments and the `WHERE` clause filters the result set of a zero-row function call rather than targeting the row. Full-text search for new messages is effectively broken.

---

## 3. Security Concerns

### Authentication & Transport
- `cmd/server/main.go:173` — When no TLS certificates or ACME are configured, the server defaults to `allowInsecureAuth = true`. SMTP and IMAP will accept plaintext passwords.
- `internal/smtp/smtp.go:34` — `AllowInsecureAuth` is passed through from configuration with no STARTTLS upgrade path. The `go-smtp` server will advertise `AUTH PLAIN` on an unencrypted channel.
- `internal/api/middleware.go:72` — CORS middleware sets `Access-Control-Allow-Origin: *` unconditionally. For a mail platform this is overly permissive and dangerous when combined with cookie-based session auth.
- `internal/api/middleware.go:146` — `extractToken` reads a cookie named `session` but does not enforce `HttpOnly`, `Secure`, or `SameSite` attributes. Those attributes are only documented in `API-SPEC.md` but not enforced in code.
- `internal/auth/auth.go` — `SESSION_KEY` is loaded from an environment variable with no validation. The default examples use `changeme` and `change-me-in-production`, making it easy to deploy with a weak signing key.

### Input Validation & Authorization
- `internal/smtp/smtp.go:95` — `Mail` accepts any `from` address without validating that the sender domain matches the authenticated user's domain membership. The design doc (`PROTOCOL-DESIGN.md`) says this should return `550`, but the check is missing.
- `internal/smtp/smtp.go:97` — `Rcpt` accepts any recipient without format validation or rate limiting.
- `internal/webhook/webhook.go:107` — `verify` checks `X-Postmark-Server-Token` against a shared secret **or** returns `true` if the secret is empty (`h.secret == ""`). An empty secret disables all webhook authentication. Postmark's proper signature verification is not implemented.
- `internal/webmail/webmail.go` — Most handlers trust `api.UserFromContext(r.Context())` but do not verify that the requested resource belongs to the user's active domain.

### Secret Handling
- `docker-compose.yml:36` — `POSTNEST_SECURITY_SESSION_KEY` is injected from an env var but can be empty.
- `scripts/install-systemd.sh:104` — The install script creates a PostgreSQL user with the literal password `changeme` if the DB user does not exist.
- `internal/config/template.go` — The generated config template contains `postgres://postnest:changeme@localhost:5432/postnest?sslmode=disable` as the default DSN.

---

## 4. Performance Concerns

### N+1 / Large Result Sets
- `internal/imap/backend.go` — `ListMessages`, `SearchMessages`, `UpdateMessagesFlags`, `CopyMessages`, and `Expunge` all call `ListMessages(..., mailstore.ListOptions{Limit: 10000})`. For large mailboxes this loads up to 10,000 rows into memory and then filters them in Go.
- `internal/mailstore/pgstore.go` — `ListMessages` runs a `COUNT(*)` subquery before the data query. On mailboxes with millions of messages, `COUNT(*)` with a `JOIN` can be expensive.
- `internal/mailstore/pgstore.go` — `GetThread` fetches up to 1,000 messages for a thread with no pagination.
- `internal/imap/backend.go:140` — `ListMessages` fetches all messages, then calls `GetFlags` individually for each message inside the loop. This is an N+1 query pattern.

### Memory Pressure / Large Allocations
- `internal/smtp/smtp.go:102` — The SMTP `Data` handler reads the entire message body with `io.ReadAll(body)` into a single `[]byte`. The config allows messages up to 50 MB, so a single email can allocate 50 MB before parsing even begins.
- `internal/smtp/smtp.go` — Each MIME part is also read into memory (`io.ReadAll(p.Body)`). Attachments are collected as in-memory slices.
- `internal/workers/inbound.go` — The inbound processor base64-decodes every attachment into memory. No size limit is enforced before decoding.
- `internal/mailstore/pgstore.go` — `CreateMessage` stores the full RFC822 source and all attachment data as PostgreSQL `BYTEA` in a single transaction. Large attachments bloat the transaction log and connection memory.

### Missing Indexes / Query Efficiency
- `internal/mailstore/pgstore.go` — `Search` queries use `WHERE domain_id=$1 AND user_id=$2 AND search_vector @@ plainto_tsquery('english',$3)`. The GIN index `messages_search_vector_idx` only covers `search_vector`; PostgreSQL will still scan a large portion of the index when the user has many messages. A composite index on `(domain_id, user_id, search_vector)` is missing.
- `internal/mailstore/pgstore.go` — `FindOrCreateThread` uses `subject` (raw text) as the lookup key instead of a normalized hash, and appends to a `TEXT[]` array (`array_append`). Arrays grow unbounded and updates become slower over time.
- `internal/mailstore/pgstore.go` — `CreateAttachments` loops over attachments and performs individual `INSERT` statements. No `COPY` or batch insert is used.

---

## 5. Fragile Areas

### IMAP State Machine
- `internal/imap/backend.go:127` — `UidNext` and `UidValidity` are hard-coded. If a client caches these values and the server restarts, clients will see unchanged values even after mailbox mutations, causing sync bugs.
- `internal/imap/backend.go:391` — `messageUID` is not stable across resyncs because it depends on the message UUID, which is random. Two different sessions can produce different UID mappings for the same message if the UID table is ever populated later.
- `internal/imap/backend.go:26` — `Login` selects `domains[0].DomainID` for multi-domain users. There is no mechanism for a user to choose which domain to access.
- `internal/imap/backend.go:213` — `SearchMessages` returns the UID of **every** message in the mailbox, completely ignoring the `criteria` parameter. Clients relying on server-side search will receive incorrect results.

### SMTP Relay Error Handling
- `internal/smtp/smtp.go` — After successfully relaying to Postmark, the code attempts to store a copy in the `Sent` label. If `CreateMessage` fails, the error is only logged (`slog.Default().Error`). The SMTP client already received `250 OK`, so the message is permanently missing from the local mailstore.
- `internal/smtp/smtp.go` — Transient vs permanent failure classification is coarse: any Postmark error returns `451`, and any `res.ErrorCode != 0` returns `550`. There is no retry backoff or queueing for transient Postmark failures inside the SMTP path.

### Webhook Idempotency
- `internal/webhook/webhook.go:107` — No signature verification and no idempotency key. Postmark retries will create duplicate Redis jobs.
- `internal/webhook/webhook.go` — Webhook payloads are not persisted to the `webhook_events` table before enqueuing. If Redis is unavailable, the event is lost with no audit trail.

### Worker Reliability
- `internal/workers/workers.go:97` — Failed jobs are re-enqueued **immediately** with no exponential backoff or jitter. A poison-pill job will hammer the CPU and Redis.
- `internal/workers/workers.go:97` — After `MaxAttempts` (3), the job is silently dropped. There is no dead-letter queue or alerting.
- `internal/workers/workers.go:55` — The `worker` loop uses `BRPop` with a timeout equal to `pollInterval` (default 5s). On shutdown, workers may block for up to 5s before noticing the cancelled context.
- `internal/workers/workers.go` — `LPush` / `BRPop` provides at-most-once delivery semantics if a worker crashes between `BRPop` and `Process`. No acknowledgment mechanism exists.

### Transaction / Data Integrity
- `internal/mailstore/pgstore.go:268` — `ApplyLabels` ignores errors from the `DELETE` statement (`_, _ = tx.Exec(...)`). If the delete fails but the subsequent insert succeeds, the message will end up with more labels than intended.
- `internal/mailstore/pgstore.go:353` — `CreateAttachments` does not run inside the `CreateMessage` transaction. If the message insert succeeds but an attachment insert fails, the message exists without its attachments.

---

## 6. Missing Features / Incomplete Implementations

- **Admin REST API** — `cmd/server/main.go:103` mounts the admin route group but registers zero handlers.
- **CalDAV** — `internal/dav/dav.go:261` is a complete stub; no `calendar_events` table exists.
- **WebDAV Files** — `design/PROTOCOL-DESIGN.md` §3.4 describes file storage endpoints, but no WebDAV handler is implemented in `internal/dav`.
- **STARTTLS** — `INTEGRATION.md` lists STARTTLS on IMAP/SMTP as unimplemented.
- **SMTP LOGIN Auth** — Only `AUTH PLAIN` is advertised (`internal/smtp/smtp.go:266`).
- **Rate Limiting** — `design/COMPONENT-DESIGN.md` documents a `RateLimiter` middleware, but it is not present in `internal/api/middleware.go`.
- **Greylist Retry Delay** — Simplified evaluation with no background timer or deferral queue.
- **IMAP UID Table Population** — `imap_uids` is defined in the schema but never written to by `mailstore`.
- **Draft Send / Update** — `internal/webmail/webmail.go:272` and `:276` are no-op stubs.

---

## 7. Testing Gaps

- **Only one test file exists:** `internal/config/loader_test.go`.
- **Zero unit tests for:** `internal/auth`, `internal/mailstore`, `internal/api`, `internal/imap`, `internal/smtp`, `internal/webhook`, `internal/workers`, `internal/postmark`, `internal/dav`, `internal/contacts`, `internal/reputation`, `internal/search`, `internal/certmanager`, `internal/db`, `internal/redis`.
- **No integration tests** for the SMTP relay path, webhook processing, IMAP command sequences, or search indexing.
- **No benchmarks** for hot paths such as `ListMessages`, `Search`, or `CreateMessage` with large attachments.
- **No fuzz tests** for MIME parsing in `internal/smtp/smtp.go` or vCard parsing in `internal/dav/dav.go`.

---

## 8. Operational Concerns

### Observability
- No metrics (Prometheus / OpenTelemetry) are exposed. Queue depth, processing latency, DB connection pool usage, and SMTP transaction rates are invisible.
- `internal/api/middleware.go:59` — The `Recovery` middleware catches panics but only emits a generic `fmt.Errorf("panic: %v", rec)` with no stack trace. Debugging production panics is extremely difficult.

### Graceful Shutdown
- `cmd/server/main.go` — IMAP and SMTP servers are stopped with `Close()`. There is no drain period for active TCP connections; clients mid-transaction will be force-disconnected.
- `cmd/worker/main.go` — Shutdown waits a fixed 30 seconds (`shutdownCtx.Done()`) but does not track in-flight jobs. A long-running job can be aborted mid-process.
- `cmd/server/main.go:145` — `certMgr.Start(context.Background())` uses a background context that is never cancelled. If the ACME directory is unreachable, startup hangs indefinitely.

### Health Checks
- `/healthz` only pings PostgreSQL and Redis. There is no health check for the SMTP listener, IMAP listener, or certificate validity.

### Data Durability
- Webhook events are not written to `webhook_events` before enqueueing. If Redis is down or the worker crashes, the event is gone with no record.
- `internal/redis/redis.go` — No connection retry or circuit-breaker logic. A transient Redis network blip will cause immediate worker failures and webhook `500` responses.

### Resource Leaks
- `internal/smtp/smtp.go:42` — `Start` spawns a goroutine that waits on `ctx.Done()` and then calls `s.srv.Close()`. If the server is stopped via `Stop()` first, the goroutine leaks because `ctx` may never be cancelled (the SMTP `Server.Start` takes a background context in `cmd/server/main.go`).
- `internal/certmanager/manager.go` — The renewal ticker goroutine is started but `Stop` only signals `stopCh`. If `renewalLoop` is blocked on network I/O, it may not exit promptly.

---

*Document generated from direct source inspection. All file paths are relative to repository root.*
