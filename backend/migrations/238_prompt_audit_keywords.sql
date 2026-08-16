-- Persist the independent prompt-audit keyword decision without changing
-- existing AI audit events.
ALTER TABLE prompt_audit_events
    ADD COLUMN IF NOT EXISTS matched_keyword VARCHAR(200) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_prompt_audit_events_matched_keyword
    ON prompt_audit_events(matched_keyword);
