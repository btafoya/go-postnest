# API Specification — Go-PostNest Postmark Mail Platform

## 1. Authentication

### 1.1 Session Authentication (Webmail)
- **Login**: `POST /api/v1/auth/login`
  - Body: `{ "email": "user@example.com", "password": "...", "remember": false }`
  - Response: `Set-Cookie: session=<token>; HttpOnly; Secure; SameSite=Lax`
  - CSRF token returned in body: `{ "csrf_token": "..." }`
- **Logout**: `POST /api/v1/auth/logout`
- **Me**: `GET /api/v1/auth/me` → `{ "id", "email", "display_name", "domains": [...] }`

### 1.2 API Key Authentication (Third-party / DAV)
- **Header**: `Authorization: Bearer <api_key>`
- API keys are stored in `auth_sessions` with `type = 'api_key'`.
- No CSRF required for Bearer requests.

### 1.3 Multi-Domain Context
- Every authenticated request carries an implicit `domain_id`.
- For users with multiple domains, a `X-Domain-ID` header or query param selects the active domain.
- Admin endpoints require `role = 'admin'` in `domain_members`.

---

## 2. Webmail REST API

### 2.1 Mailboxes / Labels

**List Labels**
```
GET /api/v1/labels
Authorization: Bearer <token>

Response 200:
{
  "labels": [
    { "id": "uuid", "name": "INBOX", "color": "#4285f4", "is_system": true, "unread_count": 3, "total_count": 42 },
    { "id": "uuid", "name": "Projects", "color": "#34a853", "is_system": false, "unread_count": 0, "total_count": 12 }
  ]
}
```

**Create Label**
```
POST /api/v1/labels
{ "name": "Projects", "color": "#34a853" }

Response 201:
{ "id": "uuid", "name": "Projects", "color": "#34a853", "is_system": false }
```

**Update Label**
```
PATCH /api/v1/labels/:id
{ "name": "Work Projects", "color": "#ff0000" }
```

**Delete Label**
```
DELETE /api/v1/labels/:id
```
- System labels return `409 Conflict`.

### 2.2 Messages

**List Messages (by label)**
```
GET /api/v1/messages?label_id=<id>&limit=50&offset=0&sort=date_desc&q=search+term

Response 200:
{
  "messages": [
    {
      "id": "uuid",
      "thread_id": "uuid",
      "subject": "Hello",
      "from": { "name": "Alice", "email": "alice@example.com" },
      "to": [{ "name": "", "email": "user@example.com" }],
      "date": "2024-01-15T09:30:00Z",
      "is_read": false,
      "is_flagged": true,
      "snippet": "Hello, I wanted to reach out about...",
      "labels": ["INBOX", "Important"],
      "size_bytes": 12450,
      "has_attachments": true
    }
  ],
  "total": 142,
  "unread": 3
}
```

**Get Message**
```
GET /api/v1/messages/:id

Response 200:
{
  "id": "uuid",
  "thread_id": "uuid",
  "subject": "Hello",
  "from": { ... },
  "to": [...],
  "cc": [...],
  "bcc": [...],
  "reply_to": "",
  "date": "2024-01-15T09:30:00Z",
  "plain_text": "Hello, I wanted to...",
  "html_body": "<html>...</html>",
  "source_url": "/api/v1/messages/:id/source",
  "is_read": true,
  "is_flagged": false,
  "is_draft": false,
  "labels": ["INBOX"],
  "attachments": [
    { "id": "uuid", "filename": "report.pdf", "content_type": "application/pdf", "size_bytes": 45000, "url": "/api/v1/attachments/:id" }
  ],
  "thread_messages": [
    { "id": "uuid", "subject": "Re: Hello", "from": { ... }, "date": "...", "is_read": true }
  ]
}
```

**Get Raw Source**
```
GET /api/v1/messages/:id/source
Content-Type: message/rfc822
```

**Toggle Read**
```
PATCH /api/v1/messages/:id
{ "is_read": true }
```

**Toggle Flagged**
```
PATCH /api/v1/messages/:id
{ "is_flagged": true }
```

**Apply Labels**
```
POST /api/v1/messages/:id/labels
{ "label_ids": ["uuid1", "uuid2"], "remove_label_ids": ["uuid3"] }
```

**Delete / Move to Trash**
```
DELETE /api/v1/messages/:id
```
- Moves to `TRASH` label (Gmail style). Hard delete only if already in `TRASH`.

**Batch Operations**
```
POST /api/v1/messages/batch
{
  "action": "mark_read|mark_unread|star|unstar|trash|delete|apply_labels",
  "message_ids": ["uuid1", "uuid2"],
  "label_ids": ["uuid"]  // for apply_labels
}
```

### 2.3 Threads

**Get Thread**
```
GET /api/v1/threads/:id

Response 200:
{
  "id": "uuid",
  "subject": "Project Update",
  "messages": [
    { "id": "uuid", ... },
    { "id": "uuid", ... }
  ],
  "participants": [
    { "name": "Alice", "email": "alice@example.com" }
  ],
  "last_message_date": "2024-01-15T09:30:00Z",
  "total_messages": 5,
  "unread_count": 1
}
```

### 2.4 Compose / Send

**Create Draft**
```
POST /api/v1/drafts
{
  "subject": "",
  "to": [{ "email": "", "name": "" }],
  "cc": [],
  "bcc": [],
  "reply_to": "",
  "html_body": "",
  "plain_text": "",
  "attachments": []  // multipart upload or reference existing attachment IDs
}

Response 201:
{ "id": "uuid", "message_id": "uuid", "updated_at": "..." }
```

**Auto-Save Draft**
```
PUT /api/v1/drafts/:id
{
  "subject": "...",
  "to": [...],
  "html_body": "...",
  "plain_text": "..."
}
```
- Debounced on client; server accepts partial updates.
- Returns `200` with updated `updated_at`.

**Send Draft**
```
POST /api/v1/drafts/:id/send

Response 202:
{ "message_id": "uuid", "status": "queued" }
```
- Converts draft to outbox message, relays via Postmark.

**Reply / Forward**
```
POST /api/v1/messages/:id/reply
POST /api/v1/messages/:id/forward
```
- Pre-populates compose form with quoted content / headers.

### 2.5 Search

```
GET /api/v1/search?q=term&label_id=uuid&from=alice@example.com&to=user@example.com&has_attachment=true&date_after=2024-01-01&date_before=2024-02-01&limit=50

Response 200:
{
  "results": [ /* message objects */ ],
  "total": 23,
  "suggestions": ["alice", "project"]
}
```

- `q` searches `subject`, `from_address`, `from_name`, `plain_text` via `tsvector`.
- Additional filters applied as PostgreSQL `WHERE` clauses.

### 2.6 Attachments

**Upload**
```
POST /api/v1/attachments
Content-Type: multipart/form-data
file=<binary>

Response 201:
{ "id": "uuid", "filename": "report.pdf", "size_bytes": 45000, "content_type": "application/pdf" }
```

**Download**
```
GET /api/v1/attachments/:id
Content-Type: <content_type>
Content-Disposition: attachment; filename="..."
```

---

## 3. Admin REST API

All endpoints prefixed with `/api/v1/admin`. Requires `domain_members.role = 'admin'` or `users.is_super_admin = true`.

### 3.1 Domains

```
GET    /api/v1/admin/domains
POST   /api/v1/admin/domains           { "name": "example.com", "postmark_token": "..." }
GET    /api/v1/admin/domains/:id
PATCH  /api/v1/admin/domains/:id       { "postmark_token": "...", "settings": {} }
DELETE /api/v1/admin/domains/:id
```

### 3.2 Users

```
GET    /api/v1/admin/users?domain_id=<id>
POST   /api/v1/admin/users             { "email": "", "password": "", "display_name": "", "domain_id": "", "role": "user" }
GET    /api/v1/admin/users/:id
PATCH  /api/v1/admin/users/:id         { "display_name": "", "role": "", "disabled": false }
DELETE /api/v1/admin/users/:id
```

### 3.3 Spam Rules

```
GET    /api/v1/admin/spam/whitelist?domain_id=<id>
POST   /api/v1/admin/spam/whitelist    { "type": "email", "value": "trusted@example.com", "note": "" }
DELETE /api/v1/admin/spam/whitelist/:id

GET    /api/v1/admin/spam/blacklist?domain_id=<id>
POST   /api/v1/admin/spam/blacklist    { "type": "email|domain|ip", "value": "...", "note": "" }
DELETE /api/v1/admin/spam/blacklist/:id

GET    /api/v1/admin/spam/greylist?domain_id=<id>
DELETE /api/v1/admin/spam/greylist/:id   // manual clear
```

### 3.4 Postmark Config

```
GET    /api/v1/admin/postmark/config?domain_id=<id>
PATCH  /api/v1/admin/postmark/config   { "postmark_token": "...", "postmark_stream": "outbound", "inbound_enabled": true }
```

### 3.5 Logs & Analytics

```
GET /api/v1/admin/delivery_logs?domain_id=<id>&status=bounced&limit=50

Response 200:
{
  "logs": [
    { "id": "uuid", "message_id": "uuid", "recipient": "", "status": "bounced", "details": {}, "created_at": "" }
  ],
  "total": 12
}
```

```
GET /api/v1/admin/webhook_events?domain_id=<id>&event_type=Bounce&limit=50
```

```
GET /api/v1/admin/reputation?domain_id=<id>&sort=score_asc&limit=50
```

---

## 4. Webhook API (Postmark)

### 4.1 Inbound Email

```
POST /webhooks/postmark/inbound
Content-Type: application/json
X-Postmark-Server-Token: <configured secret>

Body (Postmark Inbound payload):
{
  "FromName": "Alice",
  "From": "alice@example.com",
  "To": "user@example.com",
  "Subject": "Hello",
  "TextBody": "...",
  "HtmlBody": "...",
  "MessageID": "...",
  "OriginalRecipient": "user@example.com",
  "MailboxHash": "",
  "Date": "...",
  "Headers": [...],
  "Attachments": [
    { "Name": "file.pdf", "ContentType": "application/pdf", "ContentLength": 45000, "Content": "base64..." }
  ]
}
```

**Processing**:
1. Verify `X-Postmark-Server-Token` against `domains.postmark_token`.
2. Look up recipient domain via `OriginalRecipient`.
3. Enqueue Redis job: `webhook:process_inbound`.
4. Return `200 OK` immediately.

### 4.2 Bounce

```
POST /webhooks/postmark/bounce

Body:
{
  "Type": "HardBounce",
  "TypeCode": 1,
  "Name": "Hard bounce",
  "Tag": "",
  "MessageID": "...",
  "ServerID": 1234,
  "Description": "The server was unable to deliver your message...",
  "Details": "...",
  "Email": "user@example.com",
  "From": "sender@example.com",
  "BouncedAt": "2024-01-15T09:30:00Z",
  "DumpAvailable": true,
  "Inactive": true,
  "CanActivate": false,
  "Subject": "Hello",
  "Content": "...",
  "MessageStream": "outbound"
}
```

**Processing**:
1. Verify token.
2. Enqueue `bounce:process` job with payload.
3. Return `200 OK`.

### 4.3 Delivery

```
POST /webhooks/postmark/delivery

Body:
{
  "MessageID": "...",
  "Recipient": "user@example.com",
  "Tag": "",
  "DeliveredAt": "2024-01-15T09:30:00Z",
  "Details": "Test message delivery",
  "ServerID": 1234,
  "MessageStream": "outbound",
  "Subject": "Hello"
}
```

**Processing**:
1. Enqueue `delivery:update` job.
2. Update `delivery_logs.status = 'delivered'`.

### 4.4 Spam Complaint

```
POST /webhooks/postmark/spam

Body:
{
  "MessageID": "...",
  "Recipient": "user@example.com",
  "Type": "SpamComplaint",
  "TypeCode": 512,
  "Name": "Spam complaint",
  "Tag": "",
  "Details": "The message was delivered, but was either blocked by the user...",
  "Email": "user@example.com",
  "From": "sender@example.com",
  "BouncedAt": "...",
  "DumpAvailable": false,
  "Inactive": true,
  "CanActivate": false,
  "Subject": "Hello",
  "Content": "",
  "MessageStream": "outbound"
}
```

**Processing**:
1. Enqueue `complaint:process` job.
2. Update `contact_reputation.complaint_count`.
3. Flag `contact_reputation.score` for recalculation.

### 4.5 Open / Click Tracking (Optional)

```
POST /webhooks/postmark/open
POST /webhooks/postmark/click
```
- Store in `delivery_logs.details` JSONB for analytics.

---

## 5. Internal Service API Contracts

### 5.1 `mailstore.Store` Interface

```go
type Store interface {
  // Messages
  CreateMessage(ctx context.Context, msg *Message) error
  GetMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) (*Message, error)
  ListMessages(ctx context.Context, domainID, userID uuid.UUID, labelID *uuid.UUID, opts ListOptions) ([]*Message, int64, error)
  UpdateMessage(ctx context.Context, domainID, userID, messageID uuid.UUID, patch MessagePatch) error
  DeleteMessage(ctx context.Context, domainID, userID, messageID uuid.UUID) error
  MoveToTrash(ctx context.Context, domainID, userID, messageID uuid.UUID) error

  // Labels
  CreateLabel(ctx context.Context, domainID, userID uuid.UUID, label *Label) error
  GetLabels(ctx context.Context, domainID, userID uuid.UUID) ([]*Label, error)
  DeleteLabel(ctx context.Context, domainID, userID, labelID uuid.UUID) error

  // Threading
  GetThread(ctx context.Context, domainID, userID, threadID uuid.UUID) (*Thread, error)
  FindOrCreateThread(ctx context.Context, domainID, userID uuid.UUID, subject, messageID, inReplyTo string) (*Thread, error)

  // Attachments
  CreateAttachment(ctx context.Context, att *Attachment) error
  GetAttachment(ctx context.Context, attachmentID uuid.UUID) (*Attachment, error)

  // Flags
  SetFlag(ctx context.Context, messageID uuid.UUID, flag string) error
  ClearFlag(ctx context.Context, messageID uuid.UUID, flag string) error
  GetFlags(ctx context.Context, messageID uuid.UUID) ([]string, error)

  // Search
  Search(ctx context.Context, domainID, userID uuid.UUID, query string, opts SearchOptions) ([]*Message, int64, error)
  UpdateSearchVector(ctx context.Context, messageID uuid.UUID) error
}
```

### 5.2 `auth.Service` Interface

```go
type Service interface {
  Authenticate(ctx context.Context, email, password string) (*User, error)
  CreateSession(ctx context.Context, userID uuid.UUID, ip, userAgent string, expiry time.Duration) (*Session, string, error)
  ValidateSession(ctx context.Context, token string) (*Session, error)
  ValidateAPIKey(ctx context.Context, key string) (*Session, error)
  Logout(ctx context.Context, token string) error
  CreateUser(ctx context.Context, user *User, password string) error
  UpdatePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error
}
```

### 5.3 `postmark.Client` Interface

```go
type Client interface {
  SendEmail(ctx context.Context, token string, msg *OutboundMessage) (*SendResponse, error)
  SendBatch(ctx context.Context, token string, msgs []*OutboundMessage) (*BatchResponse, error)
  GetDeliveryStatus(ctx context.Context, token, postmarkMessageID string) (*DeliveryStatus, error)
}
```

### 5.4 `reputation.Engine` Interface

```go
type Engine interface {
  EvaluateInbound(ctx context.Context, domainID uuid.UUID, from, to string, senderIP net.IP) (Decision, error)
  UpdateReputation(ctx context.Context, domainID uuid.UUID, email string, eventType EventType) error
  GetReputation(ctx context.Context, domainID uuid.UUID, email string) (*Reputation, error)
}

type Decision string
const (
  DecisionPass      Decision = "pass"
  DecisionGreylist  Decision = "greylist"
  DecisionBlock     Decision = "block"
  DecisionJunk      Decision = "junk"
)
```

### 5.5 `search.Indexer` Interface

```go
type Indexer interface {
  Queue(ctx context.Context, messageID uuid.UUID) error
  ProcessQueue(ctx context.Context, batchSize int) error
  Search(ctx context.Context, domainID, userID uuid.UUID, query string, opts SearchOptions) ([]uuid.UUID, int64, error)
}
```

### 5.6 `contacts.Store` Interface

```go
type Store interface {
  Create(ctx context.Context, contact *Contact) error
  Get(ctx context.Context, domainID, userID, contactID uuid.UUID) (*Contact, error)
  GetByEmail(ctx context.Context, domainID, userID uuid.UUID, email string) (*Contact, error)
  List(ctx context.Context, domainID, userID uuid.UUID, opts ListOptions) ([]*Contact, int64, error)
  Update(ctx context.Context, domainID, userID, contactID uuid.UUID, patch ContactPatch) error
  Delete(ctx context.Context, domainID, userID, contactID uuid.UUID) error
  SyncFromVCard(ctx context.Context, domainID, userID uuid.UUID, vcard string) (*Contact, error)
  ToVCard(ctx context.Context, contactID uuid.UUID) (string, error)
}
```

---

## 6. Error Format

All API errors follow a unified JSON structure:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Request validation failed",
    "details": [
      { "field": "email", "issue": "invalid_format" },
      { "field": "password", "issue": "too_short", "min": 12 }
    ],
    "request_id": "req_abc123"
  }
}
```

**HTTP Status Codes**:
- `200` OK
- `201` Created
- `202` Accepted (async operation queued)
- `400` Bad Request (validation / malformed JSON)
- `401` Unauthorized (missing/invalid auth)
- `403` Forbidden (valid auth, insufficient permissions)
- `404` Not Found
- `409` Conflict (duplicate label, system label deletion)
- `422` Unprocessable Entity (business logic failure)
- `429` Too Many Requests
- `500` Internal Server Error

## 7. Rate Limits

| Endpoint Group | Limit | Window |
|---|---|---|
| Auth (login) | 5 attempts | 1 minute per IP |
| Auth (all) | 100 requests | 1 minute per IP |
| API (read) | 1000 requests | 1 minute per user |
| API (write) | 100 requests | 1 minute per user |
| Webhooks | 10,000 requests | 1 minute per domain |

Rate limit response: `429` with `Retry-After` header.
