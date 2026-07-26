CREATE TABLE IF NOT EXISTS custom_domains (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    domain VARCHAR(253) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_dns',
    all_users BOOLEAN NOT NULL DEFAULT FALSE,
    verification_token VARCHAR(128) NOT NULL,
    verification_txt_name VARCHAR(253) NOT NULL,
    verification_txt_value VARCHAR(256) NOT NULL,
    cname_target VARCHAR(253),
    verified_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    last_error TEXT,
    disabled_at TIMESTAMPTZ,
    disabled_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT custom_domains_status_check CHECK (
        status IN ('pending_dns', 'active', 'disabled')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS custom_domains_domain_unique_active
    ON custom_domains (lower(domain))
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS custom_domains_user_id_idx
    ON custom_domains (user_id);

CREATE INDEX IF NOT EXISTS custom_domains_status_idx
    ON custom_domains (status);

CREATE INDEX IF NOT EXISTS custom_domains_all_users_idx
    ON custom_domains (all_users)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS custom_domain_users (
    custom_domain_id BIGINT NOT NULL REFERENCES custom_domains(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (custom_domain_id, user_id)
);

CREATE INDEX IF NOT EXISTS custom_domain_users_user_id_idx
    ON custom_domain_users (user_id);

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS custom_domain_id BIGINT NULL,
    ADD COLUMN IF NOT EXISTS custom_domain VARCHAR(253) NULL;

CREATE INDEX IF NOT EXISTS usage_logs_custom_domain_id_created_at_idx
    ON usage_logs (custom_domain_id, created_at)
    WHERE custom_domain_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS usage_logs_custom_domain_created_at_idx
    ON usage_logs (custom_domain, created_at)
    WHERE custom_domain IS NOT NULL;

INSERT INTO settings (key, value)
VALUES ('custom_domains_enabled', 'false')
ON CONFLICT (key) DO NOTHING;
