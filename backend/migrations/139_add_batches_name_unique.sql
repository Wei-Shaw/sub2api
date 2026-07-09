-- 139_add_batches_name_unique.sql
-- Ensure batch names are unique so UI-side checks and database constraints agree.

CREATE UNIQUE INDEX IF NOT EXISTS idx_batches_name_unique ON batches(name);
