-- Repair databases where the custom migration runner executed both the Goose
-- Up and Down sections from legacy migrations 019, 024, and 037.

CREATE TABLE IF NOT EXISTS ops_alert_silences (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL,
    platform VARCHAR(64) NOT NULL,
    group_id BIGINT,
    region VARCHAR(64),
    until TIMESTAMPTZ NOT NULL,
    reason TEXT,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_alert_silences_lookup
    ON ops_alert_silences (rule_id, platform, group_id, region, until);

UPDATE accounts
SET credentials = jsonb_set(credentials, '{tier_id}', '"LEGACY"', true)
WHERE platform = 'gemini'
  AND type = 'oauth'
  AND jsonb_typeof(credentials) = 'object'
  AND credentials->>'tier_id' IS NULL
  AND (
      credentials->>'oauth_type' = 'code_assist'
      OR (credentials->>'oauth_type' IS NULL AND credentials->>'project_id' IS NOT NULL)
  );

INSERT INTO user_attribute_definitions
    (key, name, description, type, options, required, validation, placeholder, display_order, enabled, created_at, updated_at)
SELECT 'wechat', '微信', '用户微信号', 'text', '[]'::jsonb, false, '{}'::jsonb, '请输入微信号', 0, true, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM user_attribute_definitions WHERE key = 'wechat' AND deleted_at IS NULL
);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'users'
          AND column_name = 'wechat'
    ) THEN
        INSERT INTO user_attribute_values (user_id, attribute_id, value, created_at, updated_at)
        SELECT
            u.id,
            (SELECT id FROM user_attribute_definitions WHERE key = 'wechat' AND deleted_at IS NULL LIMIT 1),
            u.wechat,
            NOW(),
            NOW()
        FROM users u
        WHERE u.wechat IS NOT NULL
          AND u.wechat <> ''
          AND u.deleted_at IS NULL
          AND NOT EXISTS (
              SELECT 1
              FROM user_attribute_values uav
              WHERE uav.user_id = u.id
                AND uav.attribute_id = (
                    SELECT id FROM user_attribute_definitions
                    WHERE key = 'wechat' AND deleted_at IS NULL
                    LIMIT 1
                )
          );

        ALTER TABLE users DROP COLUMN wechat;
    END IF;
END $$;
