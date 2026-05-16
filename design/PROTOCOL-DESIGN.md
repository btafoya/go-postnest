# Protocol Design — IMAP, SMTP, DAV

## 1. IMAP4rev1 Server Design

### 1.1 Capabilities

The server advertises the following capabilities after STARTTLS / authentication:

```
* CAPABILITY IMAP4rev1 IDLE MOVE ACL THREAD=ORDEREDSUBJECT THREAD=REFERENCES SORT QUOTA QRESYNC CONDSTORE UIDPLUS LITERAL+ SASL-IR AUTH=PLAIN AUTH=LOGIN
```

| Capability | Implementation Notes |
|---|---|
| `IMAP4rev1` | Full RFC 3501 command set. |
| `IDLE` | Long-polling notifications. Uses Redis pub/sub to broadcast `EXISTS`, `EXPUNGE`, `FETCH` updates to idle connections. |
| `MOVE` | Atomic `COPY` + `STORE \Deleted` + `EXPUNGE` via `mailstore` transaction. |
| `ACL` | Simplified: `admin` = `lrwipekxcda`, `user` = `lrwip`, `readonly` = `lr`. Stored in `domain_members.role` mapped to IMAP rights. |
| `THREAD=ORDEREDSUBJECT` | Threads by `subject_hash` normalization (RE/FW stripped). |
| `THREAD=REFERENCES` | Threads by `In-Reply-To` / `References` correlation using `threads` table. |
| `SORT` | Sort by `DATE`, `FROM`, `TO`, `SUBJECT`, `SIZE`, `ARRIVAL`. Delegated to PostgreSQL `ORDER BY`. |
| `QUOTA` | Returns hard-coded quota from `users.settings` JSONB (`quota_bytes`, `quota_messages`). |
| `QRESYNC` | Enables `UIDVALIDITY` + `UIDNEXT` + `MODSEQ` based on `messages.created_at` / `updated_at` versioning. |
| `CONDSTORE` | Per-message `MODSEQ` derived from `messages.updated_at` as Unix nanosecond timestamp. |
| `UIDPLUS` | `UID EXPUNGE` and `APPEND` returning `UID`. |

### 1.2 Mailbox Mapping

Gmail-style labels are exposed as IMAP mailboxes:

| Label | Mailbox Name | Attributes | Special-Use |
|---|---|---|---|
| `INBOX` | `INBOX` | `\Inbox` | `\Inbox` |
| `SENT` | `Sent` | `\Sent` | `\Sent` |
| `DRAFTS` | `Drafts` | `\Drafts` | `\Drafts` |
| `TRASH` | `Trash` | `\Trash` | `\Trash` |
| `JUNK` | `Junk` | `\Junk` | `\Junk` |
| `IMPORTANT` | `Important` |  |  |
| `STARRED` | `Starred` | `\Flagged` | `\Flagged` |
| User labels | `Labels/Work`, `Labels/Projects` |  |  |

**LIST Response**:
```
* LIST (\Inbox) "/" "INBOX"
* LIST (\Sent) "/" "Sent"
* LIST (\Drafts) "/" "Drafts"
* LIST (\Trash) "/" "Trash"
* LIST (\Junk) "/" "Junk"
* LIST (\Flagged) "/" "Starred"
* LIST () "/" "Labels/Work"
```

### 1.3 Message Sequence ↔ UID Mapping

- `messages.id` (UUID) is not used for IMAP sequence numbers.
- IMAP UIDs are synthetic integers derived from `messages.created_at` ordered monotonically per mailbox, or from a dedicated `imap_uids` mapping table.
- **Recommended**: Maintain `imap_uids(uid BIGINT, message_id UUID, user_id UUID, mailbox VARCHAR)` with a per-user `uidnext` counter.
- This avoids UUID-to-integer conversion issues and enables reliable `UIDVALIDITY`.

### 1.4 Flag Mapping

| IMAP Flag | `messages` column | `message_labels` / notes |
|---|---|---|
| `\Seen` | `is_read = true` | |
| `\Answered` | `is_answered = true` | |
| `\Flagged` | `is_flagged = true` | Also adds `STARRED` label |
| `\Deleted` |  | Triggers move to `TRASH` label on EXPUNGE |
| `\Draft` | `is_draft = true` | Also in `DRAFTS` label |
| `\Recent` |  | Computed at session start (messages not yet seen by any session) |
| `$NotJunk` | `message_flags` row | |
| `$Junk` | `message_flags` row | Also adds `JUNK` label |

### 1.5 IDLE Implementation

```go
// Redis channel per user+mailbox
channel := fmt.Sprintf("imap:idle:%s:%s", userID, mailbox)

// On message creation / update:
redis.Publish(channel, "EXISTS 3")  // new message UID 3
redis.Publish(channel, "EXPUNGE 2") // message removed
```

IMAP server subscribes to Redis channel while in IDLE state. On publish, writes untagged response to client and resets idle timer.

### 1.6 QRESYNC / CONDSTORE

- `UIDVALIDITY`: Hash of user's mailbox creation epoch + domain ID.
- `UIDNEXT`: `MAX(uid) + 1` from `imap_uids` for the selected mailbox.
- `MODSEQ`: `EXTRACT(EPOCH FROM messages.updated_at) * 1_000_000_000` (nanoseconds) truncated to 64-bit integer.
- `VANISHED`: On QRESYNC SELECT, return UIDs of messages deleted since last `HIGHESTMODSEQ`.

---

## 2. SMTP Proxy Design

### 2.1 Supported Commands

| Command | Behavior |
|---|---|
| `EHLO` / `HELO` | Advertise `STARTTLS`, `AUTH PLAIN`, `AUTH LOGIN`, `SIZE 52428800`, `8BITMIME`, `PIPELINING` |
| `STARTTLS` | Upgrade to TLS 1.3 using configured certificate |
| `AUTH PLAIN` | Validate `\0user\0pass` via `auth.Service` |
| `AUTH LOGIN` | Two-step base64 username then password |
| `MAIL FROM:` | Validate sender domain matches authenticated user's domain membership. Enforce `SIZE` limit. |
| `RCPT TO:` | Validate recipient format. For outbound, no local delivery check (always relayed). |
| `DATA` | Buffer up to 50MB. Inject into `mailstore` as outgoing draft with `Sent` label. **Immediately relay to Postmark API in the foreground**; on Postmark success return `250 OK`, on transient failure return `451` (client should retry), on permanent failure return `550`. Fallback to background worker only for internal queue overload. |
| `RSET` | Clear transaction state |
| `NOOP` | Return `250 OK` (used for health checks) |
| `QUIT` | Close connection |

### 2.2 Outbound Flow

```
Client → SMTP Proxy:
  MAIL FROM:<user@example.com>
  RCPT TO:<recipient@external.com>
  DATA
  ...RFC822 source...

SMTP Proxy (foreground, immediate relay per PLAN.md):
  1. Parse MIME headers.
  2. Call mailstore.CreateMessage() with:
     - is_outbound = true
     - mailbox = 'SENT'
     - labels = ['SENT']
     - source = raw DATA
  3. Create delivery_logs rows for each RCPT TO (status = pending).
  4. Call postmark.Client.SendEmail() synchronously.
     a. On success (2xx from Postmark):
        - Update delivery_logs with postmark_message_id, status = sent.
        - Return 250 OK to client.
     b. On transient failure (4xx / timeout):
        - Update delivery_logs status = deferred.
        - Enqueue Redis job `postmark:send` for retry.
        - Return 451 Try again later to client.
     c. On permanent failure (hard 5xx):
        - Update delivery_logs status = bounced.
        - Return 550 Permanent failure to client.

Background Worker (postmark:send):
  1. Dequeue retry job for deferred messages.
  2. Call postmark.Client.SendEmail().
  3. Update delivery_logs on final disposition.

### 2.3 Inbound Flow

Inbound email does NOT arrive via SMTP (Postmark receives it). Instead:

1. Postmark processes inbound email.
2. Postmark POSTs to `/webhooks/postmark/inbound`.
3. Webhook receiver enqueues `webhook:process_inbound`.
4. Worker parses RFC822, creates `messages` + `attachments`, applies spam rules, assigns labels.

### 2.4 Authentication

- **SASL PLAIN**: `AUTH PLAIN base64(\0username\0password)`
- **SASL LOGIN**: Two-prompt base64 exchange.
- **TLS Required**: Plaintext authentication rejected without TLS session.
- **Rate Limiting**: Max 5 AUTH failures per IP per minute → temporary `421`.

### 2.5 Multi-Domain Handling

- SMTP login username is the user's `email` (e.g., `user@example.com`).
- `MAIL FROM:` address must be a domain the user is a member of.
- Violation returns `550` with message `MAIL FROM domain not authorized for user`.

---

## 3. DAV Protocol Design (CardDAV / CalDAV / WebDAV)

### 3.1 Routing

| Well-Known | Path | Protocol |
|---|---|---|
| `/.well-known/carddav` | Redirects to `/dav/contacts/` | CardDAV |
| `/.well-known/caldav` | Redirects to `/dav/calendar/` | CalDAV |
| `/dav/contacts/` | Address book collection | CardDAV |
| `/dav/calendar/` | Calendar collection | CalDAV |
| `/dav/files/` | File storage collection | WebDAV |

All DAV endpoints require Basic Auth (username = email, password = account password) or Bearer token in `Authorization` header.

### 3.2 CardDAV (Contacts)

**Address Book Discovery**:
```xml
<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:response>
    <d:href>/dav/contacts/</d:href>
    <d:propstat>
      <d:prop>
        <d:displayname>Contacts</d:displayname>
        <card:addressbook-description>Main Address Book</card:addressbook-description>
        <card:supported-addressbook-data>
          <card:addressbook-data content-type="text/vcard" version="4.0"/>
        </card:supported-addressbook-data>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>
```

**Contact Resources**:
- URI: `/dav/contacts/{contact-id}.vcf`
- Storage: `contacts.vcard_data` (vCard 4.0 text).
- Operations: `PROPFIND`, `REPORT addressbook-query`, `REPORT addressbook-multiget`, `PUT`, `DELETE`, `MKCOL` (no-op for address books).

**Mapping**:
| vCard Property | `contacts` column |
|---|---|
| `FN` | `name` |
| `N` (Given) | `given_name` |
| `N` (Family) | `family_name` |
| `EMAIL` | `email` |
| `ORG` | `organization` |
| `TEL` | `phone` |
| Full vCard | `vcard_data` |

### 3.3 CalDAV (Calendar)

**Calendar Discovery**:
```xml
<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:response>
    <d:href>/dav/calendar/</d:href>
    <d:propstat>
      <d:prop>
        <d:displayname>Calendar</d:displayname>
        <c:calendar-description>Main Calendar</c:calendar-description>
        <c:supported-calendar-component-set>
          <c:comp name="VEVENT"/>
          <c:comp name="VTODO"/>
        </c:supported-calendar-component-set>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>
```

**Event Resources**:
- URI: `/dav/calendar/{event-id}.ics`
- Storage: Dedicated `calendar_events` table (see extension note below) or stored as `BYTEA` blobs with CalDAV index properties.
- Operations: `PROPFIND`, `REPORT calendar-query`, `REPORT calendar-multiget`, `PUT`, `DELETE`, `MKCALENDAR`.

**Mapping**:
| iCal Property | Table column |
|---|---|
| `UID` | `event_uid` (VARCHAR) |
| `SUMMARY` | `summary` |
| `DTSTART` | `start_time` (TIMESTAMPTZ) |
| `DTEND` | `end_time` (TIMESTAMPTZ) |
| `DESCRIPTION` | `description` |
| `LOCATION` | `location` |
| `ORGANIZER` | `organizer_email` |
| `ATTENDEE` | `attendees` (JSONB) |
| Full iCal | `ical_data` (TEXT) |

*Note: If the project scope does not include a full calendar UI, `calendar_events` may be omitted and CalDAV can return `501 Not Implemented` for `MKCALENDAR` while still supporting read-only event access from mail headers (e.g., meeting invites stored as `text/calendar` attachments).*

### 3.4 WebDAV (Files)

Simplified WebDAV over mail attachments and user-uploaded files.

**Collection**:
- URI: `/dav/files/{folder}/`
- `PROPFIND` lists files with `DAV:displayname`, `DAV:getcontentlength`, `DAV:getcontenttype`, `DAV:getlastmodified`.

**File Mapping**:
| WebDAV Resource | Source |
|---|---|
| `/dav/files/attachments/{message-id}/{filename}` | `attachments` table |
| `/dav/files/uploads/{filename}` | Future user file store (optional) |

**Operations**:
- `GET`: Stream `attachments.data` with `Content-Type`.
- `PROPFIND`: Directory listing.
- `DELETE`: Not allowed on attachments (read-only).
- `PUT` / `MKCOL`: Supported only under `/dav/files/uploads/` if file storage feature is enabled.

### 3.5 DAV Authentication

```
Authorization: Basic base64(email:password)
```
- Validated against `auth.Service.Authenticate()`.
- Session not created; each request is stateless.
- For Bearer token: `Authorization: Bearer <token>` → validated via `auth.ValidateAPIKey()`.

---

## 4. Protocol Security

| Protocol | Port | TLS | Auth | Notes |
|---|---|---|---|---|
| IMAP | 143 | STARTTLS | Plain / Login | TLS required before AUTH |
| IMAPS | 993 | Implicit TLS | Plain / Login | Preferred |
| SMTP | 587 | STARTTLS | Plain / Login | TLS required before AUTH |
| SMTPS | 465 | Implicit TLS | Plain / Login | Preferred |
| HTTP (Web) | 8080 | Reverse proxy TLS | Session / Bearer | TLS terminated at LB |
| DAV | 8080 | Reverse proxy TLS | Basic / Bearer | Same as HTTP |

**Certificate Management**:
- Single wildcard or per-domain TLS certificate mounted into containers.
- For multi-tenant custom domains, integrate Let's Encrypt via `autocert` (Go) or terminate at external load balancer.
