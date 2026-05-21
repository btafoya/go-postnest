-- Backfill imap_uids for existing messages with sequential UIDs.
INSERT INTO imap_uids (message_id, user_id, mailbox, uid, modseq)
SELECT m.id, m.user_id, COALESCE(l.name, m.mailbox), nextval('imap_uids_uid_seq'), 1
FROM messages m
LEFT JOIN message_labels ml ON ml.message_id = m.id
LEFT JOIN labels l ON ml.label_id = l.id
ORDER BY m.user_id, COALESCE(l.name, m.mailbox), m.created_at;
