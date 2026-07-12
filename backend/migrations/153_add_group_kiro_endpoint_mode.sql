-- Kiro group-level inference endpoint selector.
-- q   = AWS Q (q.{region}.amazonaws.com)  [default, backfills existing rows]
-- krs = Kiro Runtime Service (runtime.us-east-1.kiro.dev)
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS kiro_endpoint_mode VARCHAR(8) NOT NULL DEFAULT 'q';
