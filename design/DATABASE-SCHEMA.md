# Database Schema — Go-PostNest Postmark Mail Platform

## 1. Design Principles

- **PostgreSQL 16+** as the sole persistent store.
- **Exact RFC822 source** stored permanently as `BYTEA` in `messages.source`.
- **Attachments** stored as `BYTEA` in `attachments.data`.
- **Gmail-style labels**: many-to-many `message_labels`; `\Inbox`, `\Sent`, `\Drafts`, `\Trash`, `\Junk`, `\Important` are system labels per user.
- **Full-text search**: PostgreSQL `tsvector` with `english` + `simple` dictionaries; updated asynchronously.
- **Threading**: `threads` table groups messages by `Message-ID` / `In-Reply-To` / `References` correlation.
- **Multi-tenancy**: Every mail/contact row is scoped to a `domain_id` + `user_id` pair.

## 2. Core Entities

### 2.1 `domains`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK, default `gen_random_uuid()` | Domain identifier |
| `name` | `VARCHAR(253)` | UNIQUE, NOT NULL | Domain FQDN |
| `postmark_token` | `VARCHAR(255)` |  | Postmark server API token |
| `postmark_stream` | `VARCHAR(64)` | default `'outbound'` | Postmark message stream |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |
| `updated_at` | `TIMESTAMPTZ` | default `now()` | |
| `settings` | `JSONB` | default `'{}'` | Domain-level overrides |

**Indexes**:
- `UNIQUE(name)`

### 2.2 `users`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `email` | `VARCHAR(254)` | UNIQUE, NOT NULL | Primary email / login |
| `password_hash` | `VARCHAR(255)` | NOT NULL | Argon2id hash |
| `display_name` | `VARCHAR(128)` |  | |
| `timezone` | `VARCHAR(64)` | default `'UTC'` | |
| `locale` | `VARCHAR(8)` | default `'en'` | |
| `is_super_admin` | `BOOLEAN` | default `false` | Platform-wide admin |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |
| `updated_at` | `TIMESTAMPTZ` | default `now()` | |
| `settings` | `JSONB` | default `'{}'` | User preferences |

**Indexes**:
- `UNIQUE(email)`

### 2.3 `domain_members`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `user_id` | `UUID` | FK → `users(id)`, ON DELETE CASCADE | |
| `role` | `VARCHAR(16)` | NOT NULL, check `role IN ('admin','user','readonly')` | |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**PK**: `(domain_id, user_id)`

**Indexes**:
- `domain_members_user_id_idx` on `user_id`

### 2.4 `auth_sessions`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `user_id` | `UUID` | FK → `users(id)`, ON DELETE CASCADE | |
| `token_hash` | `VARCHAR(255)` | UNIQUE, NOT NULL | SHA-256 of bearer token |
| `type` | `VARCHAR(16)` | NOT NULL, check `type IN ('session','api_key')` | |
| `expires_at` | `TIMESTAMPTZ` | NOT NULL | |
| `last_used_at` | `TIMESTAMPTZ` |  | |
| `ip_address` | `INET` |  | |
| `user_agent` | `TEXT` |  | |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `auth_sessions_token_hash_idx` on `token_hash`
- `auth_sessions_user_id_idx` on `user_id`

---

## 3. Mail Entities

### 3.1 `threads`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `user_id` | `UUID` | FK → `users(id)`, ON DELETE CASCADE | |
| `subject_hash` | `VARCHAR(64)` |  | Normalized subject for grouping |
| `message_ids` | `TEXT[]` |  | All `Message-ID` values in thread |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |
| `updated_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `threads_domain_user_idx` on `(domain_id, user_id)`
- `threads_subject_hash_idx` on `(domain_id, user_id, subject_hash)`

### 3.2 `messages`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `user_id` | `UUID` | FK → `users(id)`, ON DELETE CASCADE | |
| `thread_id` | `UUID` | FK → `threads(id)`, ON DELETE SET NULL | |
| `postmark_message_id` | `VARCHAR(64)` |  | Postmark outbound ID |
| `mailbox` | `VARCHAR(255)` | NOT NULL | IMAP mailbox name (INBOX, Sent, etc.) |
| `message_id_header` | `VARCHAR(998)` |  | Original `Message-ID` header |
| `in_reply_to` | `VARCHAR(998)` |  | Original `In-Reply-To` header |
| `references` | `TEXT[]` |  | Original `References` headers |
| `subject` | `TEXT` |  | Decoded subject |
| `from_address` | `VARCHAR(254)` |  | Envelope From |
| `from_name` | `VARCHAR(128)` |  | Display name |
| `to_addresses` | `VARCHAR(254)[]` |  | Envelope To |
| `cc_addresses` | `VARCHAR(254)[]` |  | |
| `bcc_addresses` | `VARCHAR(254)[]` |  | |
| `reply_to` | `VARCHAR(254)` |  | |
| `date` | `TIMESTAMPTZ` |  | Header Date parsed |
| `plain_text` | `TEXT` |  | Extracted plain text body |
| `html_body` | `TEXT` |  | Extracted HTML body |
| `source` | `BYTEA` | NOT NULL | Exact RFC822 source |
| `size_bytes` | `INTEGER` | NOT NULL | Source byte length |
| `is_draft` | `BOOLEAN` | default `false` | |
| `is_outbound` | `BOOLEAN` | default `false` | Sent by this user |
| `is_read` | `BOOLEAN` | default `false` | |
| `is_flagged` | `BOOLEAN` | default `false` | |
| `is_answered` | `BOOLEAN` | default `false` | |
| `search_vector` | `TSVECTOR` |  | Full-text index |
| `created_at` | `TIMESTAMPTZ` | default `now()` | Ingestion time |
| `updated_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `messages_domain_user_mailbox_idx` on `(domain_id, user_id, mailbox, created_at DESC)`
- `messages_domain_user_thread_idx` on `(domain_id, user_id, thread_id, created_at DESC)`
- `messages_postmark_idx` on `(postmark_message_id)` WHERE `postmark_message_id IS NOT NULL`
- `messages_search_vector_idx` using `GIN(search_vector)`
- `messages_message_id_header_idx` on `message_id_header`
- `messages_date_idx` on `(domain_id, user_id, date DESC)`

### 3.3 `labels`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `user_id` | `UUID` | FK → `users(id)`, ON DELETE CASCADE | |
| `name` | `VARCHAR(128)` | NOT NULL | Label name |
| `color` | `VARCHAR(7)` | default `'#4285f4'` | Hex color |
| `is_system` | `BOOLEAN` | default `false` | Built-in label |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `labels_domain_user_name_idx` UNIQUE on `(domain_id, user_id, name)`

**System labels seeded per user**:
- `INBOX`, `SENT`, `DRAFTS`, `TRASH`, `JUNK`, `IMPORTANT`, `STARRED`, `ALL_MAIL`

### 3.4 `message_labels`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `message_id` | `UUID` | FK → `messages(id)`, ON DELETE CASCADE | |
| `label_id` | `UUID` | FK → `labels(id)`, ON DELETE CASCADE | |
| `applied_at` | `TIMESTAMPTZ` | default `now()` | |

**PK**: `(message_id, label_id)`

**Indexes**:
- `message_labels_label_idx` on `label_id`

### 3.5 `attachments`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `message_id` | `UUID` | FK → `messages(id)`, ON DELETE CASCADE | |
| `filename` | `VARCHAR(255)` | NOT NULL | |
| `content_type` | `VARCHAR(255)` | NOT NULL | MIME type |
| `size_bytes` | `INTEGER` | NOT NULL | |
| `data` | `BYTEA` | NOT NULL | File contents |
| `content_id` | `VARCHAR(255)` |  | For inline attachments |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `attachments_message_id_idx` on `message_id`

### 3.6 `message_flags`

IMAP custom flags beyond the boolean columns on `messages`.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `message_id` | `UUID` | FK → `messages(id)`, ON DELETE CASCADE | |
| `flag` | `VARCHAR(64)` | NOT NULL | e.g., `$NotJunk`, `$Phishing` |
| `set_at` | `TIMESTAMPTZ` | default `now()` | |

**PK**: `(message_id, flag)`

### 3.7 `imap_uids`

IMAP sequence-to-UID mapping. Enables reliable `UIDVALIDITY`, `UIDNEXT`, and `EXPUNGE` tracking.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `uid` | `BIGSERIAL` | NOT NULL | Monotonic IMAP UID per user |
| `message_id` | `UUID` | FK → `messages(id)`, ON DELETE CASCADE | |
| `user_id` | `UUID` | FK → `users(id)`, ON DELETE CASCADE | |
| `mailbox` | `VARCHAR(255)` | NOT NULL | IMAP mailbox name |
| `modseq` | `BIGINT` | NOT NULL | CONDSTORE MODSEQ (epoch nanoseconds) |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**PK**: `(user_id, mailbox, uid)`

**Indexes**:
- `imap_uids_message_idx` on `message_id`
- `imap_uids_modseq_idx` on `(user_id, mailbox, modseq)`

---

## 4. Contact & Reputation Entities

### 4.1 `contacts`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `user_id` | `UUID` | FK → `users(id)`, ON DELETE CASCADE | |
| `email` | `VARCHAR(254)` | NOT NULL | |
| `name` | `VARCHAR(128)` |  | Display name |
| `given_name` | `VARCHAR(64)` |  | vCard N given |
| `family_name` | `VARCHAR(64)` |  | vCard N family |
| `organization` | `VARCHAR(128)` |  | |
| `phone` | `VARCHAR(32)` |  | Primary phone |
| `vcard_data` | `TEXT` |  | Raw vCard 4.0 |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |
| `updated_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `contacts_domain_user_email_idx` UNIQUE on `(domain_id, user_id, email)`
- `contacts_email_idx` on `email` for reputation lookups

### 4.2 `contact_reputation`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `contact_id` | `UUID` | FK → `contacts(id)`, ON DELETE CASCADE | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `sent_count` | `INTEGER` | default `0` | Outbound messages to this contact |
| `received_count` | `INTEGER` | default `0` | Inbound messages from this contact |
| `bounce_count` | `INTEGER` | default `0` | Postmark bounces |
| `complaint_count` | `INTEGER` | default `0` | Spam complaints |
| `score` | `INTEGER` | default `50` | 0–100; higher = more trusted |
| `last_interaction_at` | `TIMESTAMPTZ` |  | |
| `updated_at` | `TIMESTAMPTZ` | default `now()` | |

**PK**: `contact_id`

**Indexes**:
- `contact_reputation_domain_score_idx` on `(domain_id, score DESC)`

---

## 5. Spam Entities

### 5.1 `whitelist`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `type` | `VARCHAR(16)` | NOT NULL, check `type IN ('email','domain','ip')` | |
| `value` | `VARCHAR(253)` | NOT NULL | |
| `note` | `TEXT` |  | |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `whitelist_domain_type_value_idx` UNIQUE on `(domain_id, type, value)`

### 5.2 `greylist`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `sender_email` | `VARCHAR(254)` | NOT NULL | |
| `sender_ip` | `INET` |  | |
| `recipient_email` | `VARCHAR(254)` | NOT NULL | |
| `first_seen_at` | `TIMESTAMPTZ` | default `now()` | |
| `passed_at` | `TIMESTAMPTZ` |  | NULL = still greylisted |
| `retry_count` | `INTEGER` | default `1` | |

**Indexes**:
- `greylist_triplet_idx` UNIQUE on `(domain_id, sender_email, sender_ip, recipient_email)`
- `greylist_domain_passed_idx` on `(domain_id, passed_at)`

### 5.3 `blacklist`

Same schema as `whitelist`.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `type` | `VARCHAR(16)` | NOT NULL, check `type IN ('email','domain','ip')` | |
| `value` | `VARCHAR(253)` | NOT NULL | |
| `note` | `TEXT` |  | |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `blacklist_domain_type_value_idx` UNIQUE on `(domain_id, type, value)`

---

## 6. Event & Log Entities

### 6.1 `delivery_logs`

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `message_id` | `UUID` | FK → `messages(id)`, ON DELETE CASCADE | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `recipient` | `VARCHAR(254)` | NOT NULL | |
| `status` | `VARCHAR(16)` | NOT NULL, check `status IN ('pending','sent','delivered','bounced','deferred','complained')` | |
| `postmark_message_id` | `VARCHAR(64)` |  | |
| `details` | `JSONB` | default `'{}'` | Provider response / bounce detail |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |
| `updated_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `delivery_logs_message_id_idx` on `message_id`
- `delivery_logs_domain_status_idx` on `(domain_id, status, created_at DESC)`

### 6.2 `webhook_events`

Raw webhook payload archive for audit and replay.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `provider` | `VARCHAR(16)` | default `'postmark'` | |
| `event_type` | `VARCHAR(32)` | NOT NULL | `Inbound`, `Bounce`, `Delivery`, `Open`, `Click`, `SpamComplaint` |
| `payload` | `JSONB` | NOT NULL | Raw webhook body |
| `processed_at` | `TIMESTAMPTZ` |  | NULL = unprocessed |
| `error` | `TEXT` |  | Processing failure reason |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `webhook_events_domain_type_idx` on `(domain_id, event_type, created_at DESC)`
- `webhook_events_unprocessed_idx` on `(domain_id, processed_at)` WHERE `processed_at IS NULL`

### 6.3 `bounce_events`

Denormalized bounce summary for fast reporting.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `UUID` | PK | |
| `delivery_log_id` | `UUID` | FK → `delivery_logs(id)`, ON DELETE CASCADE | |
| `domain_id` | `UUID` | FK → `domains(id)`, ON DELETE CASCADE | |
| `bounce_type` | `VARCHAR(16)` | NOT NULL | `HardBounce`, `SoftBounce`, `SpamComplaint`, etc. |
| `bounce_description` | `TEXT` |  | Human-readable reason |
| `diagnostic_code` | `VARCHAR(255)` |  | SMTP response code / detail |
| `created_at` | `TIMESTAMPTZ` | default `now()` | |

**Indexes**:
- `bounce_events_domain_type_idx` on `(domain_id, bounce_type, created_at DESC)`

---

## 7. Full-Text Search Design

### 7.1 `tsvector` Generation

The `messages.search_vector` column is populated with:

```sql
-- Trigger function (executed asynchronously by worker for performance)
CREATE OR REPLACE FUNCTION messages_update_search_vector()
RETURNS TRIGGER AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('english', coalesce(NEW.subject, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(NEW.from_address, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(NEW.from_name, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(NEW.plain_text, '')), 'C') ||
    setweight(to_tsvector('simple', coalesce(NEW.to_addresses::text, '')), 'D');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

### 7.2 Query Pattern

```sql
SELECT id, subject, from_address, ts_rank_cd(search_vector, query, 32) AS rank
FROM messages
WHERE domain_id = $1 AND user_id = $2
  AND search_vector @@ plainto_tsquery('english', $3)
ORDER BY rank DESC, date DESC
LIMIT 50;
```

### 7.3 Async Updates

Instead of synchronous triggers, the `search updater` worker processes a Redis queue of `message_id`s to refresh. This avoids write latency on high-volume inbound ingestion.

---

## 8. Migration Strategy

1. **V1__init.sql**: Create all tables, indexes, and constraints.
2. **V2__fts.sql**: Install `pg_trgm` and create `tsvector` generation function.
3. **V3__seed_labels.sql**: Insert system labels (`INBOX`, `SENT`, etc.) for existing users.
4. **V4__indexes.sql**: Add partial indexes and performance tuning after initial data load.

Use **golang-migrate** or **golang-migrate/v4** for versioned migrations.
