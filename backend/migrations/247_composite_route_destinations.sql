-- Explicit upstream resource grants. Never SET NULL on deletion: that would
-- silently convert a restricted route into the legacy composite platform pool.
ALTER TABLE composite_model_routes
    ADD COLUMN target_group_id BIGINT NULL,
    ADD COLUMN target_account_id BIGINT NULL,
    ADD CONSTRAINT composite_route_destination_exclusive CHECK (target_group_id IS NULL OR target_account_id IS NULL),
    ADD CONSTRAINT composite_route_target_group_positive CHECK (target_group_id IS NULL OR target_group_id > 0),
    ADD CONSTRAINT composite_route_target_account_positive CHECK (target_account_id IS NULL OR target_account_id > 0),
    ADD CONSTRAINT composite_route_target_not_self CHECK (target_group_id IS NULL OR target_group_id <> group_id),
    ADD CONSTRAINT composite_route_target_group_fk FOREIGN KEY (target_group_id) REFERENCES groups(id) ON DELETE RESTRICT,
    ADD CONSTRAINT composite_route_target_account_fk FOREIGN KEY (target_account_id) REFERENCES accounts(id) ON DELETE RESTRICT;

CREATE INDEX composite_route_target_group_idx ON composite_model_routes(target_group_id) WHERE target_group_id IS NOT NULL;
CREATE INDEX composite_route_target_account_idx ON composite_model_routes(target_account_id) WHERE target_account_id IS NOT NULL;
