-- Scheduler outbox: durable claim columns.
--
-- FOR UPDATE only holds row locks for the duration of one statement, so the
-- previous design could not distinguish "consumed" from "never observed":
-- a row whose id was allocated before the watermark advanced but committed
-- after it stayed invisible to `id > watermark` forever, and was then deleted
-- by cleanup as if it had been consumed.
--
-- Mirror the durable invalidation queue (auth_cache_invalidation_outbox):
-- ownership is recorded in the row, and an abandoned claim becomes available
-- again after its lease expires. All columns are nullable so the ALTER never
-- rewrites the table on a hot queue.
ALTER TABLE scheduler_outbox
    ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS claimed_by TEXT,
    ADD COLUMN IF NOT EXISTS available_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_scheduler_outbox_claimable
    ON scheduler_outbox (available_at, id);
