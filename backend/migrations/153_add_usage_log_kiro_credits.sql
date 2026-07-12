-- Store Kiro credit consumption per usage log for account usage statistics.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS kiro_credits NUMERIC(20,10);

COMMENT ON COLUMN usage_logs.kiro_credits IS 'Kiro credits consumed by this usage log';
