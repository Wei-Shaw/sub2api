-- Extract composite model routes from per-group ownership into reusable schemes.
-- Existing group-scoped routes are migrated into one scheme per source group
-- and rebound so runtime resolution stays unchanged.

CREATE TABLE IF NOT EXISTS composite_route_schemes (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_composite_route_schemes_name_active
    ON composite_route_schemes (name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_composite_route_schemes_deleted_at
    ON composite_route_schemes (deleted_at);

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS composite_route_scheme_id BIGINT NULL;

ALTER TABLE composite_model_routes
    ADD COLUMN IF NOT EXISTS scheme_id BIGINT NULL;

DO $$
DECLARE
    rec RECORD;
    new_id BIGINT;
    scheme_name TEXT;
BEGIN
    FOR rec IN
        SELECT DISTINCT r.group_id,
               COALESCE(NULLIF(BTRIM(g.name), ''), 'group-' || r.group_id::text) AS group_name
        FROM composite_model_routes r
        LEFT JOIN groups g ON g.id = r.group_id
        WHERE r.scheme_id IS NULL
    LOOP
        scheme_name := rec.group_name || ' (#' || rec.group_id::text || ')';

        INSERT INTO composite_route_schemes (name, description, created_at, updated_at)
        VALUES (
            scheme_name,
            '由分组「' || rec.group_name || '」的 Composite 路由迁移而来',
            NOW(),
            NOW()
        )
        RETURNING id INTO new_id;

        UPDATE composite_model_routes
        SET scheme_id = new_id
        WHERE group_id = rec.group_id
          AND scheme_id IS NULL;

        UPDATE groups
        SET composite_route_scheme_id = new_id
        WHERE id = rec.group_id
          AND composite_route_scheme_id IS NULL;
    END LOOP;
END $$;

DELETE FROM composite_model_routes WHERE scheme_id IS NULL;

ALTER TABLE composite_model_routes
    ALTER COLUMN scheme_id SET NOT NULL;

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_groups_group;

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_group_id_fkey;

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_scheme_id_fkey;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_scheme_id_fkey
    FOREIGN KEY (scheme_id) REFERENCES composite_route_schemes(id) ON DELETE CASCADE;

ALTER TABLE groups
    DROP CONSTRAINT IF EXISTS groups_composite_route_scheme_id_fkey;

ALTER TABLE groups
    ADD CONSTRAINT groups_composite_route_scheme_id_fkey
    FOREIGN KEY (composite_route_scheme_id) REFERENCES composite_route_schemes(id) ON DELETE SET NULL;

DROP INDEX IF EXISTS idx_composite_model_routes_unique_active;
CREATE UNIQUE INDEX IF NOT EXISTS idx_composite_model_routes_unique_active
    ON composite_model_routes (scheme_id, endpoint, match_type, public_model)
    WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS idx_composite_model_routes_group_enabled;
CREATE INDEX IF NOT EXISTS idx_composite_model_routes_scheme_enabled
    ON composite_model_routes (scheme_id, enabled)
    WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS idx_composite_model_routes_group_priority;
CREATE INDEX IF NOT EXISTS idx_composite_model_routes_scheme_priority
    ON composite_model_routes (scheme_id, priority, id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_composite_model_routes_scheme_id
    ON composite_model_routes (scheme_id);

CREATE INDEX IF NOT EXISTS idx_groups_composite_route_scheme_id
    ON groups (composite_route_scheme_id)
    WHERE composite_route_scheme_id IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE composite_model_routes
    DROP COLUMN IF EXISTS group_id;
