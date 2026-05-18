---
status: testing
phase: webui
source: [.planning/design/DESIGN.md, .planning/design/webui-UI-REVIEW.md]
started: 2026-05-18T01:29:42Z
updated: 2026-05-18T03:32:00Z
---

## Current Test
<!-- OVERWRITE each test - shows where we are -->

status: UAT COMPLETE

## Tests

### 1. Login Flow
expected: Unauthenticated → login screen. Valid creds → Inbox loads. Invalid creds → red inline error banner, no native alert.
result: issue
reported: "Invalid creds gives Authentication required"
severity: minor

### 2. Inbox List + Unread State
expected: Inbox shows message rows. Unread messages have blue background + bold sender/subject. Read messages plain white. Date column shows time (today) or "Mon D" (older). Empty inbox shows "No messages / Your inbox is empty".
result: pass
note: Empty-state copy verified correct. Unread/read styling + date format NOT exercised (inbox empty — no messages). Coverage gap, not a defect.

### 3. Open Message + HTML Render
expected: Click a message row → opens MessageView. Marked read automatically (row would un-bold on return). HTML email renders inside sandboxed iframe (no broken layout, no script execution). Plain-text email shows in pre-wrap block.
result: pass

### 4. Compose + Send (CRITICAL — DESIGN.md §1.1)
expected: Click Compose. Enter recipient in To, subject, body in rich editor (Bold/Italic/List toolbar works). Send button disabled until valid recipient. Click Send → returns to inbox, message actually delivered to recipient (not empty draft, recipient populated).
result: pass

### 5. Compose Autosave
expected: In Compose, type content, wait ~3s without sending. "Saved {time}" appears top-right. Reload/return — draft persisted with content intact.
result: pass
note: Draft editable + autosave confirmed. User observed attachment didn't work during this test — flagged for Test 11.

### 6. Reply
expected: Open a message, click Reply icon. Compose opens prefilled: To = original sender, Subject = "Re: ...", body has quoted blockquote of original.
result: issue
reported: "Body is not blockquoted (quote collapses to plain text without line breaks when html_body is empty)"
severity: minor

### 7. Forward (KNOWN GAP — UI-REVIEW)
expected: Open a message, click Forward icon. Compose should open prefilled for forwarding. (UI-REVIEW flagged Forward has NO onClick handler — expected to do nothing. Confirm.)
result: issue
reported: "Confirm: clicking Forward does nothing"
severity: major

### 8. Star / Flag Toggle
expected: In inbox, click star icon on a row → fills yellow, persists after refresh. Click again → unfills. Same toggle works in MessageView toolbar.
result: pass
note: Root inbox empty (no received mail). Tested in Sent + Drafts folders — same handler, persists across refresh.

### 9. Batch Operations + Partial-Fail Notice (DESIGN.md §1.3)
expected: Select multiple messages via checkboxes. Archive/Spam/Delete toolbar buttons appear. Action applies. If some fail, amber notice "N of M message(s) failed to {action}" with dismissible ×. All-success → no notice.
result: pass
note: User observed no Archive folder exists to view archived messages — UX gap, action works but no way to access archived mail.

### 10. Search
expected: Type query in header search, Enter. Inbox filters to matching messages. No results → "No messages / Try a different search".
result: pass
note: Search navigates to /?q=... — Inbox nav becomes active (correct, global search behavior).

### 11. Attachments — Drag & Drop + Click (DESIGN.md §4.1)
expected: In Compose, drag a file onto the window OR click Attach. File chip appears with name + KB size. Trash icon removes it. Attachment delivered with sent mail.
result: pass
note: File chip renders with name + size. Recipient preserved after attach. Send queues draft correctly. UX gap: no Discard button in Compose (drafts accumulate); no delete in Drafts list. Body validation added to prevent empty-body send failures.

### 12. Contacts CRUD
expected: Contacts page lists contact cards. New Contact modal saves. Edit pre-fills + updates. Delete asks native confirm, removes. Search filters live. Save failure → native alert (KNOWN: UI-REVIEW flagged alert() not toast).
result: pass
note: Contact cards render with name, email, phone. New Contact modal saves correctly. Edit pre-fills and updates. Delete works with native confirm. Search filters live. Error banner replaced alert().

### 13. Calendar CRUD (DESIGN.md §3)
expected: Calendar shows month grid, today highlighted (blue circle). Click a day → New Event modal with 9–10am prefilled. Save → event chip on that day. Click event → edit modal with Delete. Prev/Next/Today navigation works.
result: pass
note: Save works (start/end keys fixed). Edit works (uid param fix). Delete works (uid param + rows affected + modal close fix).

### 14. Admin Health Dashboard (DESIGN.md §5 — CONCERNS #49)
expected: Admin (admin users only) → Overview tab. System Status shows REAL Database/Redis/IMAP/SMTP up/down dots with latency_ms, not static green. Worker queue depth/dead counts shown. Polls every 15s. Domains tab lists managed domains.
result: pass
note: Real counts wired (domains/users/messages today). System Status live with latency. Domains tab lists all domains with user counts. Users/Security tabs are placeholders. Domain management (add/edit) not yet built.

### 15. CSRF Protection (DESIGN.md §2 — NFR-1)
expected: After login, a mutating action (send/star/delete) succeeds. (Behind scenes: X-CSRF-Token header from csrf cookie required; without it server 403s.) No CSRF errors during normal use; mutations don't silently fail.
result: pass
note: All prior mutating tests (send/star/delete/save) implicitly validated CSRF. No 403s encountered.

### 16. Logout + Session
expected: Click Sign out (sidebar bottom). Returns to login screen. Browser back / revisit root does NOT restore session — re-auth required.
result: pass

## Summary

total: 16
passed: 13
issues: 3
pending: 0
skipped: 0

## Gaps

- truth: "Invalid credentials show red inline banner with message 'Invalid credentials'"
  status: failed
  reason: "User reported: Invalid creds gives Authentication required"
  severity: minor
  test: 1
  root_cause: ""
  artifacts: []
  missing: []
  debug_session: ""

- truth: "Compose Send delivers message and returns to inbox (DESIGN.md §1.1 critical path)"
  status: failed
  reason: "User reported: Send returns Internal server error (POST /api/v1/drafts 500)"
  severity: blocker
  test: 4
  root_cause: >
    domainID() fallback at internal/webmail/webmail.go:86 returns u.ID (a
    USER uuid) as domain_id when the logged-in user has no domain_members
    row. createDraft INSERTs messages.domain_id = user.id, violating FK
    messages_domain_id_fkey (domain_id not present in domains). Reproduced
    directly in psql: same FK error. Users test@test.com and
    btafoya@premadev.com have NO domain_members row -> every draft
    create/autosave/send 500s for them. admin@example.com / qa@test.local
    have membership and would succeed. Secondary defect: api.WriteError
    (internal/api/errors.go:46-48) maps non-AppError to ErrInternal WITHOUT
    logging, so the Postgres error was fully swallowed (logs showed only
    status:500), blinding diagnosis.
  artifacts:
    - path: "internal/webmail/webmail.go"
      issue: "domainID() line 86 returns u.ID as domain fallback -> FK violation for users with no domain membership"
    - path: "internal/api/errors.go"
      issue: "WriteError swallows non-AppError errors (no log) -> opaque 500s"
  missing:
    - "domainID() must not fabricate a domain_id from user.ID; return a 4xx (no-domain) or guarantee every user has a domain membership at provisioning"
    - "WriteError should log the underlying error when falling back to ErrInternal"
    - "Data: test@test.com / btafoya@premadev.com lack domain_members rows"
  debug_session: ""

- truth: "Draft persisted in Drafts folder can be opened and edited to send"
  status: failed
  reason: "User reported: Draft email present in Draft folder but not editable to complete and send"
  severity: major
  test: 5
  root_cause: >
    Two bugs: (1) Inbox.jsx row click unconditionally navigated to
    /message/:id even for drafts, opening MessageView (read-only)
    instead of /compose/:id. (2) Compose.jsx never loaded existing draft
    content when draftId URL param present — opened empty compose.
  artifacts:
    - path: "web/src/components/Inbox.jsx"
      issue: "Draft rows navigate to /message/:id instead of /compose/:id"
    - path: "web/src/components/Compose.jsx"
      issue: "No draft fetch on mount when draftId present"
  missing:
    - "Inbox row click conditional: is_draft → /compose/:id"
    - "Compose effect to fetch getMessage(draftId) and populate fields"
  debug_session: ""

- truth: "Compose draft preserves all recipient fields after attachment upload"
  status: fixed
  reason: "Multiple bugs: (1) Backend DTO sends 'email', frontend read 'address' — field mismatch wiped To. (2) Draft fetch effect had no Cc/Bcc loading. (3) ensureDraft triggered fetch effect which overwrote user input. (4) GetUserDomains had no ORDER BY — unstable domainID. (5) UpdateMessage silently returned 204 on 0 rows. (6) DEPLOYMENT BUG: docker compose restart webui does NOT rebuild image — frontend changes were not deployed until docker compose build webui was run."
  severity: blocker
  test: 11
  root_cause: >
    Frontend: Compose.jsx effect read t.address (should be t.email), missed cc/bcc setters, and re-ran after ensureDraft creating race condition. Fix: effect now depends on routeDraftId (URL param) instead of draftId state, and uses loadedDraftId ref to prevent duplicate loads.
    Backend: GetUserDomains lacked ORDER BY; UpdateMessage ignored RowsAffected.
    Deployment: webui is a Docker image built at compose-up time; restart uses old image. Frontend changes require docker compose build webui.
  artifacts:
    - path: "web/src/components/Compose.jsx"
      issue: "Draft load effect used t.address, missing cc/bcc setters, no guard against ensureDraft race"
    - path: "internal/auth/auth.go"
      issue: "GetUserDomains SELECT had no ORDER BY"
    - path: "internal/mailstore/pgstore.go"
      issue: "UpdateMessage ignored RowsAffected, silently no-op on domain mismatch"
  missing:
    - "t.address → t.email in draft population effect"
    - "Add setCc/setBcc to draft population effect"
    - "Effect depend on routeDraftId instead of draftId state + loadedDraftId ref"
    - "GetUserDomains ORDER BY created_at ASC"
    - "UpdateMessage check RowsAffected == 0 → ErrNotFound"
    - "Document: docker compose build webui required for frontend deploy"
  debug_session: ""

- truth: "Attachments delivered with sent mail"
  status: fixed
  reason: "User reported: message delivered but attachment missing. Worker send.go built postmark.OutboundMessage without loading attachments from store."
  severity: major
  test: 11
  root_cause: "SendProcessor.Process never called store.ListMessageAttachments(draftID) to populate pmMsg.Attachments."
  artifacts:
    - path: "internal/workers/send.go"
      issue: "OutboundMessage built without Attachments field"
  missing:
    - "Add ListMessageAttachments call and map to postmark.Attachment before SendEmail"
  debug_session: ""

- truth: "Contacts CRUD works end-to-end"
  status: fixed
  reason: "User reported: no contacts load, new contact save fails with 'Failed to save contact' alert. Backend had NO HTTP handler for contacts — the Store interface existed but no routes were registered in the server. Frontend also had alert() for errors (UI-REVIEW flagged) and mapped notes field to non-existent backend column."
  severity: major
  test: 12
  root_cause: >
    Backend: internal/contacts/ only had Store (DB layer) — no HTTP handler or RegisterRoutes. cmd/server/main.go never called contacts.NewHandler(...).RegisterRoutes(r), so all /api/v1/contacts requests returned 404.
    Frontend: Contacts.jsx used alert() on save/delete errors. Backend Contact model has no 'notes' field; frontend sent notes which was silently dropped.
  artifacts:
    - path: "internal/contacts/"
      issue: "No HTTP handler; only Store interface existed"
    - path: "cmd/server/main.go"
      issue: "contacts.NewHandler never registered"
    - path: "web/src/components/Contacts.jsx"
      issue: "alert() on errors; notes field not mapped to backend vcard_data"
  missing:
    - "Create internal/contacts/handler.go with list/create/update/delete endpoints"
    - "Register contacts handler in authenticated API group in cmd/server/main.go"
    - "Replace alert() with inline red error banner in Contacts modal"
    - "Map notes ↔ vcard_data between frontend and backend"
  debug_session: ""

- truth: "Calendar events save correctly"
  status: fixed
  reason: "User reported: 'Failed to save event' alert. Frontend sent start_time/end_time keys but backend decodeEvent only accepted start/end/starts_at/ends_at."
  severity: major
  test: 13
  root_cause: "API contract mismatch: Calendar.jsx payload used start_time/end_time; backend expected start/end."
  artifacts:
    - path: "web/src/components/Calendar.jsx"
      issue: "Payload sent start_time/end_time instead of start/end"
  missing:
    - "Fix payload keys: start_time → start, end_time → end"
    - "Replace alert() with inline red error banner"
  debug_session: ""

- truth: "Forward button opens compose prefilled for forwarding"
  status: failed
  reason: "User confirmed: clicking Forward does nothing"
  severity: major
  test: 7
  root_cause: "MessageView.jsx:66 Forward button has no onClick handler"
  artifacts:
    - path: "web/src/components/MessageView.jsx"
      issue: "Forward button missing onClick handler"
  missing:
    - "Add onClick to navigate('/compose', { state: { forwardTo: message } }) and prefill Compose for forwarding"
  debug_session: ""
