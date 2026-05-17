CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(253) UNIQUE NOT NULL,
    postmark_token VARCHAR(255),
    postmark_stream VARCHAR(64) DEFAULT 'outbound',
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    settings JSONB DEFAULT '{}'
);

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(254) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(128),
    timezone VARCHAR(64) DEFAULT 'UTC',
    locale VARCHAR(8) DEFAULT 'en',
    is_super_admin BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    settings JSONB DEFAULT '{}'
);

CREATE TABLE domain_members (
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(16) NOT NULL CHECK (role IN ('admin','user','readonly')),
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (domain_id, user_id)
);
CREATE INDEX domain_members_user_id_idx ON domain_members(user_id);

CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    type VARCHAR(16) NOT NULL CHECK (type IN ('session','api_key')),
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX auth_sessions_token_hash_idx ON auth_sessions(token_hash);
CREATE INDEX auth_sessions_user_id_idx ON auth_sessions(user_id);

CREATE TABLE threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subject_hash VARCHAR(64),
    message_ids TEXT[],
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX threads_domain_user_idx ON threads(domain_id, user_id);
CREATE INDEX threads_subject_hash_idx ON threads(domain_id, user_id, subject_hash);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES threads(id) ON DELETE SET NULL,
    postmark_message_id VARCHAR(64),
    mailbox VARCHAR(255) NOT NULL,
    message_id_header VARCHAR(998),
    in_reply_to VARCHAR(998),
	    "references" TEXT[],
    subject TEXT,
    from_address VARCHAR(254),
    from_name VARCHAR(128),
    to_addresses VARCHAR(254)[],
    cc_addresses VARCHAR(254)[],
    bcc_addresses VARCHAR(254)[],
    reply_to VARCHAR(254),
    date TIMESTAMPTZ,
    plain_text TEXT,
    html_body TEXT,
    source BYTEA NOT NULL,
    size_bytes INTEGER NOT NULL,
    is_draft BOOLEAN DEFAULT false,
    is_outbound BOOLEAN DEFAULT false,
    is_read BOOLEAN DEFAULT false,
    is_flagged BOOLEAN DEFAULT false,
    is_answered BOOLEAN DEFAULT false,
    search_vector TSVECTOR,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX messages_domain_user_mailbox_idx ON messages(domain_id, user_id, mailbox, created_at DESC);
CREATE INDEX messages_domain_user_thread_idx ON messages(domain_id, user_id, thread_id, created_at DESC);
CREATE INDEX messages_postmark_idx ON messages(postmark_message_id) WHERE postmark_message_id IS NOT NULL;
CREATE INDEX messages_search_vector_idx ON messages USING GIN(search_vector);
CREATE INDEX messages_message_id_header_idx ON messages(message_id_header);
CREATE INDEX messages_date_idx ON messages(domain_id, user_id, date DESC);

CREATE TABLE labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    color VARCHAR(7) DEFAULT '#4285f4',
    is_system BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX labels_domain_user_name_idx ON labels(domain_id, user_id, name);

CREATE TABLE message_labels (
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    applied_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (message_id, label_id)
);
CREATE INDEX message_labels_label_idx ON message_labels(label_id);

CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(255) NOT NULL,
    size_bytes INTEGER NOT NULL,
    data BYTEA NOT NULL,
    content_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX attachments_message_id_idx ON attachments(message_id);

CREATE TABLE message_flags (
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    flag VARCHAR(64) NOT NULL,
    set_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (message_id, flag)
);

CREATE TABLE imap_uids (
    uid BIGSERIAL NOT NULL,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mailbox VARCHAR(255) NOT NULL,
    modseq BIGINT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (user_id, mailbox, uid)
);
CREATE INDEX imap_uids_message_idx ON imap_uids(message_id);
CREATE INDEX imap_uids_modseq_idx ON imap_uids(user_id, mailbox, modseq);

CREATE TABLE contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email VARCHAR(254) NOT NULL,
    name VARCHAR(128),
    given_name VARCHAR(64),
    family_name VARCHAR(64),
    organization VARCHAR(128),
    phone VARCHAR(32),
    vcard_data TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX contacts_domain_user_email_idx ON contacts(domain_id, user_id, email);
CREATE INDEX contacts_email_idx ON contacts(email);

CREATE TABLE contact_reputation (
    contact_id UUID PRIMARY KEY REFERENCES contacts(id) ON DELETE CASCADE,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    sent_count INTEGER DEFAULT 0,
    received_count INTEGER DEFAULT 0,
    bounce_count INTEGER DEFAULT 0,
    complaint_count INTEGER DEFAULT 0,
    score INTEGER DEFAULT 50,
    last_interaction_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX contact_reputation_domain_score_idx ON contact_reputation(domain_id, score DESC);

CREATE TABLE whitelist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    type VARCHAR(16) NOT NULL CHECK (type IN ('email','domain','ip')),
    value VARCHAR(253) NOT NULL,
    note TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX whitelist_domain_type_value_idx ON whitelist(domain_id, type, value);

CREATE TABLE greylist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    sender_email VARCHAR(254) NOT NULL,
    sender_ip INET,
    recipient_email VARCHAR(254) NOT NULL,
    first_seen_at TIMESTAMPTZ DEFAULT now(),
    passed_at TIMESTAMPTZ,
    retry_count INTEGER DEFAULT 1
);
CREATE UNIQUE INDEX greylist_triplet_idx ON greylist(domain_id, sender_email, sender_ip, recipient_email);
CREATE INDEX greylist_domain_passed_idx ON greylist(domain_id, passed_at);

CREATE TABLE blacklist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    type VARCHAR(16) NOT NULL CHECK (type IN ('email','domain','ip')),
    value VARCHAR(253) NOT NULL,
    note TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE UNIQUE INDEX blacklist_domain_type_value_idx ON blacklist(domain_id, type, value);

CREATE TABLE delivery_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    recipient VARCHAR(254) NOT NULL,
    status VARCHAR(16) NOT NULL CHECK (status IN ('pending','sent','delivered','bounced','deferred','complained')),
    postmark_message_id VARCHAR(64),
    details JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX delivery_logs_message_id_idx ON delivery_logs(message_id);
CREATE INDEX delivery_logs_domain_status_idx ON delivery_logs(domain_id, status, created_at DESC);

CREATE TABLE webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    provider VARCHAR(16) DEFAULT 'postmark',
    event_type VARCHAR(32) NOT NULL,
    payload JSONB NOT NULL,
    processed_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX webhook_events_domain_type_idx ON webhook_events(domain_id, event_type, created_at DESC);
CREATE INDEX webhook_events_unprocessed_idx ON webhook_events(domain_id, processed_at) WHERE processed_at IS NULL;

CREATE TABLE bounce_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_log_id UUID NOT NULL REFERENCES delivery_logs(id) ON DELETE CASCADE,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    bounce_type VARCHAR(16) NOT NULL,
    bounce_description TEXT,
    diagnostic_code VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX bounce_events_domain_type_idx ON bounce_events(domain_id, bounce_type, created_at DESC);
