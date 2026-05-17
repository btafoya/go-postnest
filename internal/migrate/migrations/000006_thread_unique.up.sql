-- Add unique constraint to prevent duplicate threads for the same subject per user.
-- This eliminates the race condition in FindOrCreateThread.
CREATE UNIQUE INDEX threads_unique_subject ON threads(domain_id, user_id, subject_hash);
