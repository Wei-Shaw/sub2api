-- A constant default tags existing rows without rewriting the usage table.
-- New inserts use a full transaction ID, independent of request time and sequence allocation.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS ingestion_txid xid8 NOT NULL DEFAULT '0'::xid8;
ALTER TABLE usage_logs ALTER COLUMN ingestion_txid SET DEFAULT pg_current_xact_id();
