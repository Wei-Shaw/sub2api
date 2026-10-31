-- Explicit upstream resource grants. Never SET NULL on deletion: that would
-- silently convert a restricted route into the legacy composite platform pool.
--
-- Idempotent: early builds shipped this schema as 246_composite_route_destinations.sql,
-- which later collided with an upstream migration number. Databases that already
-- applied that file re-run this one as a no-op.
ALTER TABLE composite_model_routes
    ADD COLUMN IF NOT EXISTS target_group_id BIGINT NULL,
    ADD COLUMN IF NOT EXISTS target_account_id BIGINT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'composite_model_routes'::regclass AND conname = 'composite_route_destination_exclusive') THEN
        ALTER TABLE composite_model_routes ADD CONSTRAINT composite_route_destination_exclusive CHECK (target_group_id IS NULL OR target_account_id IS NULL);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'composite_model_routes'::regclass AND conname = 'composite_route_target_group_positive') THEN
        ALTER TABLE composite_model_routes ADD CONSTRAINT composite_route_target_group_positive CHECK (target_group_id IS NULL OR target_group_id > 0);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'composite_model_routes'::regclass AND conname = 'composite_route_target_account_positive') THEN
        ALTER TABLE composite_model_routes ADD CONSTRAINT composite_route_target_account_positive CHECK (target_account_id IS NULL OR target_account_id > 0);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'composite_model_routes'::regclass AND conname = 'composite_route_target_not_self') THEN
        ALTER TABLE composite_model_routes ADD CONSTRAINT composite_route_target_not_self CHECK (target_group_id IS NULL OR target_group_id <> group_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'composite_model_routes'::regclass AND conname = 'composite_route_target_group_fk') THEN
        ALTER TABLE composite_model_routes ADD CONSTRAINT composite_route_target_group_fk FOREIGN KEY (target_group_id) REFERENCES groups(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'composite_model_routes'::regclass AND conname = 'composite_route_target_account_fk') THEN
        ALTER TABLE composite_model_routes ADD CONSTRAINT composite_route_target_account_fk FOREIGN KEY (target_account_id) REFERENCES accounts(id) ON DELETE RESTRICT;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS composite_route_target_group_idx ON composite_model_routes(target_group_id) WHERE target_group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS composite_route_target_account_idx ON composite_model_routes(target_account_id) WHERE target_account_id IS NOT NULL;
