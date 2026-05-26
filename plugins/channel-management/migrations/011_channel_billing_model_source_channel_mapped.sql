-- Change default billing_model_source for new channels to 'channel_mapped'
-- Existing channels keep their current setting (no UPDATE on existing rows)
ALTER TABLE channels ALTER COLUMN billing_model_source SET DEFAULT 'channel_mapped';
