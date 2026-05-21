-- 054_add_ldap_identity.sql
-- Add persisted token_version and LDAP identity profile table.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS token_version BIGINT NOT NULL DEFAULT 0;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS auth_source VARCHAR(20) NOT NULL DEFAULT 'local';

CREATE INDEX IF NOT EXISTS idx_users_auth_source ON users(auth_source) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_token_version ON users(token_version) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS user_ldap_profiles (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    ldap_uid VARCHAR(255) NOT NULL,
    ldap_username VARCHAR(255) NOT NULL,
    ldap_email VARCHAR(255) NOT NULL DEFAULT '',
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    department VARCHAR(255) NOT NULL DEFAULT '',
    groups_hash VARCHAR(128) NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_ldap_profiles_ldap_uid ON user_ldap_profiles(ldap_uid);
CREATE INDEX IF NOT EXISTS idx_user_ldap_profiles_active ON user_ldap_profiles(active);
