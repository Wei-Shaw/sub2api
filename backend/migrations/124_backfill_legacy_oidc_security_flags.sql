-- Preserve legacy OIDC behavior for upgraded installs that predate the
-- introduction of secure PKCE/id_token defaults. Fresh installs continue to
-- inherit runtime defaults when these rows are absent.

WITH legacy_oidc_install AS (
    SELECT 1
    FROM settings
    WHERE key IN (
        'oidc_connect_enabled',
        'oidc_connect_client_id',
        'oidc_connect_authorize_url',
        'oidc_connect_token_url',
        'oidc_connect_issuer_url',
        'oidc_connect_userinfo_url',
        'oidc_connect_frontend_redirect_url'
    )
    LIMIT 1
)
INSERT INTO settings (key, value)
SELECT defaults.key, 'false'
FROM legacy_oidc_install
CROSS JOIN (
    VALUES
        ('oidc_connect_use_pkce'),
        ('oidc_connect_validate_id_token')
) AS defaults(key)
WHERE NOT EXISTS (
    SELECT 1
    FROM settings existing
    WHERE existing.key = defaults.key
)
ON CONFLICT (key) DO NOTHING;
