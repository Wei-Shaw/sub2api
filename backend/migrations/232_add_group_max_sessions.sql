ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS max_sessions INTEGER NOT NULL DEFAULT 0;

ALTER TABLE groups
    DROP CONSTRAINT IF EXISTS groups_max_sessions_non_negative;

ALTER TABLE groups
    ADD CONSTRAINT groups_max_sessions_non_negative
    CHECK (max_sessions >= 0);
