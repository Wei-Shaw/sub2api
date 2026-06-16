ALTER TABLE scheduler_outbox
    ADD COLUMN IF NOT EXISTS dedup_key TEXT;
