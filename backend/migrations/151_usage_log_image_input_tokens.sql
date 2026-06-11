ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS image_input_tokens INTEGER;
ALTER TABLE usage_logs ALTER COLUMN image_input_tokens SET DEFAULT 0;

ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS image_input_cost DECIMAL(20, 10);
ALTER TABLE usage_logs ALTER COLUMN image_input_cost SET DEFAULT 0;
