-- OAuth Provider for native/public clients such as dsh-sub2api-sync.
-- Authorization transactions are short-lived Redis records; these tables hold
-- durable clients, codes, grants and token families.

CREATE TABLE IF NOT EXISTS oauth_clients (
    id BIGSERIAL PRIMARY KEY,
    client_id VARCHAR(160) NOT NULL UNIQUE,
    client_type VARCHAR(20) NOT NULL DEFAULT 'public',
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    logo_url TEXT NOT NULL DEFAULT '',
    redirect_uris JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    pkce_required BOOLEAN NOT NULL DEFAULT TRUE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT oauth_clients_type_check CHECK (client_type IN ('public', 'confidential')),
    CONSTRAINT oauth_clients_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE TABLE IF NOT EXISTS oauth_grants (
    id BIGSERIAL PRIMARY KEY,
    client_id VARCHAR(160) NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    approved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    UNIQUE (client_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_oauth_grants_user_client ON oauth_grants(user_id, client_id);

INSERT INTO oauth_clients (
    client_id, client_type, name, description, redirect_uris, allowed_scopes, pkce_required
)
VALUES (
    'dsh-sub2api-sync',
    'public',
    'DSH Sub2API Sync',
    'Native desktop client for synchronizing Sub2API model configuration',
    -- Loopback receivers with a dynamic port (RFC 8252 §7.3): the *path* is
    -- what is pinned. `/plugins/dsh-sub2api-sync/oauth/callback` is the DSH
    -- harness webserver route the plugin registers; `/oauth/callback` stays
    -- allowed for standalone launchers.
    '["http://127.0.0.1/oauth/callback", "http://[::1]/oauth/callback", "http://127.0.0.1/plugins/dsh-sub2api-sync/oauth/callback", "http://[::1]/plugins/dsh-sub2api-sync/oauth/callback"]'::jsonb,
    '["openid", "profile", "groups:read", "keys:read", "keys:create", "keys:revoke"]'::jsonb,
    TRUE
)
ON CONFLICT (client_id) DO UPDATE SET
    client_type = EXCLUDED.client_type,
    redirect_uris = EXCLUDED.redirect_uris,
    allowed_scopes = EXCLUDED.allowed_scopes,
    pkce_required = EXCLUDED.pkce_required,
    updated_at = NOW();
