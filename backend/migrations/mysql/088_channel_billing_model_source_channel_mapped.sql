-- MySQL 方言版本（翻译自 ../088_channel_billing_model_source_channel_mapped.sql）
-- Change default billing_model_source for new channels to 'channel_mapped'
-- Existing channels keep their current setting (no UPDATE on existing rows)
ALTER TABLE channels ALTER COLUMN billing_model_source SET DEFAULT 'channel_mapped';
