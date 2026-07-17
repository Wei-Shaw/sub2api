ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS routing_mode VARCHAR(32) NOT NULL DEFAULT 'fixed';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS auto_candidate_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE groups
    ADD CONSTRAINT groups_routing_mode_check
    CHECK (routing_mode IN ('fixed', 'auto_lowest_cost'));
