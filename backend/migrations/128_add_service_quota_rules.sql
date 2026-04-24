CREATE TABLE IF NOT EXISTS service_quota_rules (
    id BIGSERIAL PRIMARY KEY,
    enabled boolean NOT NULL DEFAULT true,
    scope_level varchar(16) NOT NULL,
    platform varchar(32) NULL,
    group_id bigint NULL REFERENCES groups(id) ON DELETE CASCADE,
    account_id bigint NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model_pattern varchar(255) NULL,
    limiter_type varchar(32) NOT NULL,
    target_mode varchar(32) NOT NULL,
    target_user_id bigint NULL REFERENCES users(id) ON DELETE CASCADE,
    window_mode varchar(16) NOT NULL DEFAULT 'fixed',
    limit_value numeric(20,8) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT service_quota_rules_scope_level_check CHECK (scope_level IN ('global','platform','group','account','model')),
    CONSTRAINT service_quota_rules_limiter_type_check CHECK (limiter_type IN ('rpm','tpm','tpd','daily_usd','concurrency')),
    CONSTRAINT service_quota_rules_target_mode_check CHECK (target_mode IN ('user','per_user','shared','default')),
    CONSTRAINT service_quota_rules_window_mode_check CHECK (window_mode IN ('fixed','rolling')),
    CONSTRAINT service_quota_rules_limit_value_check CHECK (limit_value > 0),
    CONSTRAINT service_quota_rules_user_mode_check CHECK ((target_mode = 'user' AND target_user_id IS NOT NULL) OR (target_mode <> 'user' AND target_user_id IS NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_service_quota_rules_unique_scope_type
ON service_quota_rules (
    scope_level,
    COALESCE(platform, ''),
    COALESCE(group_id, 0),
    COALESCE(account_id, 0),
    COALESCE(model_pattern, ''),
    limiter_type
);

CREATE INDEX IF NOT EXISTS idx_service_quota_rules_enabled ON service_quota_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_service_quota_rules_target_user ON service_quota_rules(target_user_id);

INSERT INTO settings(key, value, updated_at)
VALUES ('service_quota_enabled', 'false', now())
ON CONFLICT (key) DO NOTHING;
