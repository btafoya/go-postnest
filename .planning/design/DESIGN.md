# Webmail Production Completion — Design Specification

> Input: requirements spec (sc:brainstorm output)
> Authority chain: AGENTS.md > PLAN.md > INTEGRATION.md
> Output: design only. No implementation. Next: `/sc:workflow` then `/sc:implement`.

---

## 0. Locked Decisions (from brainstorm)

| # | Decision |
|---|----------|
| 1 | Ratify React. Rewrite AGENTS.md Web UI section. |
| 2 | Calendar: parsed columns only, regenerate `.ics` on CalDAV read. |
| 3 | Coverage: 80% line global, critical paths 100%. |
| 4 | Plaintext auth: hard-fail unless `POSTNEST_ALLOW_INSECURE_AUTH=true`. |
| 5 | Compose attachments: reuse `attachments` table + worker dedupe. |

---

## 1. Bug Fixes — Component Designs

### 1.1 Compose contract mismatch (CRITICAL)

**Current break**: `Compose.jsx:29` sends `{to: "string", subject, body}`. API (`webmail.go:307`) wants `to: [{address}]`, `html_body`, `plain_text`. Result: empty drafts, no recipients.

**Design — frontend `api.js` contract**:

```
createDraft / updateDraft payload:
{
  subject:    string,
  to:         [{ name?: string, address: string }],
  cc?:        [{ name?, address }],
  bcc?:       [{ name?, address }],
  html_body:  string,   // rich editor HTML output
  plain_text: string    // derived: htmlToText(html_body)
}
```

**Recipient parsing**: frontend parses comma/semicolon-separated input → `[{address}]`. Display-name form `Name <addr@x>` parsed client-side; server re-validates via `mail.ParseAddress` (already at webmail.go:391).

**Backend change**: request struct already correct. Add `cc`/`bcc` fields + `models.Message` mapping (CcAddresses/BccAddresses already in model). Bug is 100% frontend.

### 1.2 GetThread returns wrong data (CRITICAL — CONCERNS #18)

**Current**: `pgstore.GetThread` ignores `threadID`, returns last 1000 user messages.

**Design — query**:

```sql
SELECT <message cols, no source> FROM messages
WHERE domain_id=$1 AND user_id=$2 AND thread_id=$3
ORDER BY date ASC
```

`messages.thread_id` exists (`models.Message.ThreadID`). Thread row by PK. Replace `ListMessages` delegation with thread-scoped query. Signature unchanged; pure pgstore fix.

### 1.3 Batch ops swallow errors (CRITICAL — CONCERNS #19)

**Current**: `batchMessages` discards every error, always 204.

**Design — response contract**:

```
POST /api/v1/messages/batch -> 200 OK
{ "succeeded": ["uuid"...], "failed": [{"id":"uuid","error":"str"}...] }
```

200 always (partial success not HTTP error). Unknown action → 400 (currently silent). Loop collects per-id results, no `_ =` discard. Frontend `Inbox.jsx` toasts when `failed.length > 0`.

### 1.4 MessageView renders HTML as escaped text

**Current**: `MessageView.jsx:111` renders `message.body` (nonexistent field) as escaped text in `whitespace-pre-wrap`. API field is `html_body`.

**Design — sandboxed iframe render** (chosen over React raw-HTML injection for execution containment):

```
if message.html_body:
  sanitized = DOMPurify.sanitize(message.html_body)
  render <iframe sandbox="" srcdoc={sanitized}>
    - sandbox empty: no script exec, no same-origin
else:
  render message.plain_text in <pre whitespace-pre-wrap>
```

Three sanitization layers: backend bluemonday at store (webmail.go:335) + frontend DOMPurify at render + iframe sandbox for execution containment. New dep `dompurify` — justified, CONCERNS #45, no existing equivalent. React raw-HTML-injection API explicitly rejected (avoids inline-HTML XSS surface entirely).

### 1.5 Context key collision (NEW — found during design)

**Bug**: `middleware.go:131` — `RequireSession` stores session under `ctxKeyRequestID`, clobbering request ID from `RequestID` middleware. `StructuredLogger` logs garbage request_id for all authed requests.

**Design**: add `ctxKeySession ctxKey = "session"`. Store session there. `RequestIDFromContext` stays uncorrupted. Add `SessionFromContext`. Side-effect fix: CONCERNS #34 traceability.

### 1.6 Performance fixes (HIGH)

| Issue | Design |
|-------|--------|
| N+1 label counts (#27) | Single `SELECT label_id, count(*) FILTER (WHERE NOT is_read) unread, count(*) total ... GROUP BY label_id`. New `Store.CountsByLabel(ctx,did,uid) (map[uuid.UUID]LabelCounts, error)`. |
| `source` BYTEA in list (#30) | Explicit column list excluding `source` in `ListMessages`/`GetThread`/`Search`. Raw bytes via new `GetMessageSource`. Const `selectMessageCols` sans source. |

---

## 2. CSRF Protection (NFR-1, critical — CONCERNS #5)

**Threat**: cookie auth + `withCredentials:true` + no token = CSRF on every mutating endpoint.

**Design — double-submit cookie** (stateless, no shared store; AGENTS.md: no shared runtime state):

```
1. Login (api/auth.go): issue second cookie
   csrf = random 32B base64
   Cookie{ Name:"csrf", HttpOnly:false, Secure, SameSite:Strict, Path:"/" }
2. Frontend axios interceptor: read csrf cookie -> X-CSRF-Token header on
   POST/PUT/PATCH/DELETE
3. New middleware api.CSRF:
   skip GET/HEAD/OPTIONS and /api/v1/auth/login
   require hmac.Equal(header, cookie) else 403
   applied inside RequireSession group
```

Synchronizer pattern rejected: needs token store, breaks multi-instance scaling.

**Session cookie hardening** (#4, #40):
- `SameSite: StrictMode` (was Lax) — both `auth.go:74` + `middleware.go:107`. Dedupe to single `SetSessionCookie` (auth.go inlines its own).
- `MaxAge` from `cfg.SessionExpiry` (exists, config:48) not hardcoded.

---

## 3. Calendar Subsystem (FR-5)

AGENTS.md: DAV via `go-webdav` only — already scaffolded (`dav.go:14,47` `caldav.Handler`, route mounted `dav.go:64`). Backend methods stubbed `not implemented` (dav.go:275+) — those are the impl target.

### 3.1 Schema — `000007_calendar.up.sql`

Decision 2: parsed columns, regenerate `.ics` on read.

```sql
CREATE TABLE calendars (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name VARCHAR(255) NOT NULL,
  color VARCHAR(16) NOT NULL DEFAULT '#4285f4',
  description TEXT NOT NULL DEFAULT '',
  ctag BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE calendar_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  calendar_id UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  uid VARCHAR(255) NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  starts_at TIMESTAMPTZ NOT NULL,
  ends_at TIMESTAMPTZ NOT NULL,
  all_day BOOLEAN NOT NULL DEFAULT false,
  rrule TEXT NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'CONFIRMED',
  organizer VARCHAR(320) NOT NULL DEFAULT '',
  attendees JSONB NOT NULL DEFAULT '[]',
  sequence INT NOT NULL DEFAULT 0,
  etag VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (calendar_id, uid)
);
CREATE INDEX calendar_events_cal_time_idx ON calendar_events (calendar_id, starts_at);
CREATE INDEX calendar_events_user_idx ON calendar_events (domain_id, user_id);
```

Accepted risk (decision 2): X-props/VALARM/VTIMEZONE lost on round-trip. If full fidelity later: add `raw_ics BYTEA` (migration-only, no redesign).

### 3.2 Package `internal/calendar` (mirror mailstore: interface + PGStore)

```go
type Store interface {
  ListCalendars(ctx, domainID, userID uuid.UUID) ([]*models.Calendar, error)
  GetCalendar(ctx, domainID, userID, calID uuid.UUID) (*models.Calendar, error)
  CreateCalendar(ctx, *models.Calendar) error
  DeleteCalendar(ctx, domainID, userID, calID uuid.UUID) error
  ListEvents(ctx, calID uuid.UUID, from, to time.Time) ([]*models.CalendarEvent, error)
  GetEvent(ctx, calID uuid.UUID, uid string) (*models.CalendarEvent, error)
  PutEvent(ctx, *models.CalendarEvent) error        // upsert ON CONFLICT (calendar_id,uid)
  DeleteEvent(ctx, calID uuid.UUID, uid string) error
  BumpCTag(ctx, calID uuid.UUID) error
}
type PGStore struct { pool *pgxpool.Pool }
```

New models: `models.Calendar`, `models.CalendarEvent`.

### 3.3 iCalendar codec `internal/calendar/ical.go`

```
EventToICS(*models.CalendarEvent) []byte
ICSToEvent([]byte) (*models.CalendarEvent, error)
```

Dep check: `go-webdav/caldav` does DAV XML, NOT iCal serialization. Need `github.com/emersion/go-ical` (emersion ecosystem, sibling to mandated go-webdav/go-message). Flag §7.

### 3.4 REST API

Frontend `api.js:121-138` already calls:

```
GET    /api/v1/calendars
POST   /api/v1/calendars
GET    /api/v1/calendar/events?start=&end=   -> {events:[...]}
POST   /api/v1/calendar/events
PATCH  /api/v1/calendar/events/{id}
DELETE /api/v1/calendar/events/{id}
```

New `internal/calendar` HTTP handler (parallel to webmail.Handler), registered in authed group `cmd/server/main.go:100`. JSON shape matches `Calendar.jsx` (verify at implement).

### 3.5 CalDAV backend wiring

Replace `caldavBackend` stubs (dav.go:275+), delegate to `calendar.Store` + ical codec:

```
ListCalendars       -> store.ListCalendars
GetCalendarObject   -> store.GetEvent + EventToICS, ETag from row
PutCalendarObject   -> ICSToEvent + store.PutEvent + BumpCTag -> ETag
ListCalendarObjects -> store.ListEvents (range from CalendarCompRequest)
DeleteCalendarObject-> store.DeleteEvent + BumpCTag
```

`dav.NewHandler` gains `calendar.Store`. `cmd/server/main.go:220` updated.

---

## 4. Rich Compose Editor (FR-1)

AGENTS.md requires "rich compose editor". Current = plain textarea.

**Library**: TipTap (`@tiptap/react` + `@tiptap/starter-kit`). React 19 compatible, headless (Tailwind-styleable, matches design system), clean HTML output. Quill/Slate/raw-contentEditable rejected (CSS conflict / assembly / XSS surface). Flag §7.

**Design**:

```
Compose.jsx:
  to/cc/bcc: recipient chips, parse on comma/enter -> {name,address}
  editor: TipTap; on send html_body=getHTML(), plain_text=htmlToText()
  payload {subject, to[], cc[], bcc[], html_body, plain_text}
  autosave: debounce 3s -> updateDraft (create on first edit if no id)
  reply/forward: prefill quoted blockquote + recipients from source
```

### 4.1 Attachments (decision 5: reuse attachments table + worker dedupe)

```
POST   /api/v1/drafts/{id}/attachments  (multipart)
  validate size <= cfg.MaxAttachmentSize (also fix SMTP path CONCERNS #44)
  store.CreateAttachments([]*models.Attachment{MessageID:draftID,...})
  -> {id, filename, size}
DELETE /api/v1/drafts/{id}/attachments/{attID}
Dedupe: async "attachment_dedupe" worker keyed sha256(data), AGENTS.md
  Worker Rule #7, off webui critical path. Stub OK v1 if worker absent (§7).
```

`Store.CreateAttachments` exists (mailstore.go:66). Attachments on draft message ID, promoted on send (same row).

---

## 5. Admin Health Dashboard (FR-6 — CONCERNS #49)

**Current**: static green dots, fake.

**Design**:

```
GET /admin/api/v1/health  (RequireSession + RequireDomainAdmin)
{
  database:{status,latency_ms}, redis:{status,latency_ms},
  postmark:{status}, imap:{status}, smtp:{status},
  worker_queue:{depth,dead}
}
```

Reuse `/healthz` probes (main.go:71-90) + redis LLEN (`queue:jobs`/`dead`) + TCP dial IMAP/SMTP addrs. `Admin.jsx` polls 15s, renders from response.

---

## 6. AGENTS.md Amendment (decision 1)

Rewrite §"Web UI Rules" (lines 458-484):

```
Framework: React 19 + Vite + TailwindCSS (ratified; supersedes prior
  Gin/HTMX directive — frontend shipped commit 6529168).
Routing: react-router-dom v7  HTTP: axios + CSRF double-submit
Editor: TipTap  Build: web/ -> vite -> internal/webui/dist (go:embed)
Features unchanged: threaded conv, rich compose, autosave, labels,
  drag/drop attachments, keyboard shortcuts.
```

In-scope (authority chain: AGENTS.md must match reality). Edit at implement.

---

## 7. New Dependencies — Ratification Required

| Dep | Purpose | Justification |
|-----|---------|--------------|
| `github.com/emersion/go-ical` | iCalendar codec | emersion ecosystem, sibling to mandated go-webdav/go-message |
| `@tiptap/react` + `@tiptap/starter-kit` | Rich editor | AGENTS.md requires it; no existing equiv |
| `dompurify` | Frontend HTML sanitize | XSS containment CONCERNS #45, defense-in-depth |
| dev: `vitest` `@testing-library/react` `msw` | Frontend tests | NFR-2 |

AGENTS.md "Before Creating New Dependencies": none solved by approved libs, fit architecture, minimal drift. **User ratify before implement.**

---

## 8. Test Strategy (NFR-2 — 80% / 100% critical)

| Layer | Tool | Targets |
|-------|------|---------|
| Backend unit | go test | calendar.PGStore, ical codec, CSRF mw, batch result, GetThread, CountsByLabel, ctx-key fix; CONCERNS #35 zero-cov pkgs |
| Backend integ | go test + testcontainers | inbound→worker→DB→IMAP→webmail; calendar CRUD→CalDAV round-trip |
| Frontend unit | Vitest + RTL + msw | Compose payload, MessageView sanitize, Inbox batch toast, Login |
| E2E | Playwright (MCP) | login→inbox→read→reply→send; calendar create→view; batch partial-fail |

Critical (100%): auth, compose→send, calendar CRUD, CSRF, sanitization.
Gate: CI fails < 80% global OR < 100% tagged critical.

---

## 9. Sequence — Inbound HTML Render (post-fix)

```
GET /api/v1/messages/{id}
  -> getMessage -> GetMessage (cols incl html_body, excl source)
  -> JSON {id,subject,from,to,html_body,plain_text,labels}
MessageView:
  html_body? -> DOMPurify.sanitize -> <iframe sandbox="" srcdoc>
  else       -> <pre>{plain_text}</pre>
```

---

## 10. Component Map

```
NEW:
  internal/calendar/{calendar,pgstore,ical,handler}.go
  internal/migrate/migrations/000007_calendar.up.sql
  internal/api/csrf.go
  web/src/components/{RichEditor,Toast}.jsx
  web/src/test/**, internal/**/*_test.go

CHANGED (impl phase):
  internal/mailstore/{pgstore,mailstore}.go  GetThread, cols, CountsByLabel, GetMessageSource
  internal/webmail/webmail.go     batch contract, label counts
  internal/api/middleware.go      ctxKeySession, SameSiteStrict, cfg MaxAge
  internal/api/auth.go            csrf cookie, dedupe SetSessionCookie
  internal/dav/dav.go             caldavBackend impl, NewHandler sig
  cmd/server/main.go              calendar wiring, csrf mw, health, plaintext hard-fail
  web/src/api.js                  contract fix, csrf header, batch shape
  web/src/components/{Compose,MessageView,Inbox,Admin}.jsx
  web/package.json                +tiptap +dompurify +vitest +rtl +msw
  AGENTS.md                       Web UI Rules rewrite
```

---

## 11. Out of Scope (deferred, documented)

- Full iCal fidelity (VALARM/VTIMEZONE/X-props) — decision 2 accepted loss.
- IMAP UID table (#14), IMAP search criteria (#15) — not webui.
- Reputation SQLi (#3) — backend; security scope, separate work item.
- Search async queue (#12) — not webui blocker.

---

## 12. Handoff

Design complete. **Blocking gate: §7 dependency ratification** (go-ical, tiptap, dompurify, vitest stack) — AGENTS.md requires explicit approval before new deps.

Next: `/sc:workflow` for phased plan, then `/sc:implement` per phase.
