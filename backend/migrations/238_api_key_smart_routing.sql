ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS routing_group_ids JSONB;
COMMENT ON COLUMN api_keys.routing_group_ids IS 'Ordered smart routing groups; empty means fixed group';
