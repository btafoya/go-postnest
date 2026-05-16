-- System labels are seeded per user by application logic on user creation.
-- This migration inserts them for any pre-existing users.

INSERT INTO labels (domain_id, user_id, name, color, is_system)
SELECT dm.domain_id, dm.user_id, unnest.label, '#4285f4', true
FROM domain_members dm
CROSS JOIN LATERAL unnest(ARRAY['INBOX','SENT','DRAFTS','TRASH','JUNK','IMPORTANT','STARRED','ALL_MAIL']) AS unnest(label)
ON CONFLICT (domain_id, user_id, name) DO NOTHING;
