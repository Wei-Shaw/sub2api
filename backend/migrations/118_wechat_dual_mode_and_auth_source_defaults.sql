INSERT INTO settings (key, value)
VALUES
    (
        'wechat_connect_open_enabled',
        CASE
            WHEN COALESCE((SELECT value FROM settings WHERE key = 'wechat_connect_enabled'), 'false') <> 'true' THEN 'false'
            WHEN LOWER(TRIM(COALESCE((SELECT value FROM settings WHERE key = 'wechat_connect_mode'), 'open'))) = 'mp' THEN 'false'
            ELSE 'true'
        END
    ),
    (
        'wechat_connect_mp_enabled',
        CASE
            WHEN COALESCE((SELECT value FROM settings WHERE key = 'wechat_connect_enabled'), 'false') <> 'true' THEN 'false'
            WHEN LOWER(TRIM(COALESCE((SELECT value FROM settings WHERE key = 'wechat_connect_mode'), 'open'))) = 'mp' THEN 'true'
            ELSE 'false'
        END
    ),
    ('auth_source_default_email_grant_on_signup', 'false'),
    ('auth_source_default_linuxdo_grant_on_signup', 'false'),
    ('auth_source_default_oidc_grant_on_signup', 'false'),
    ('auth_source_default_wechat_grant_on_signup', 'false')
ON CONFLICT (key) DO NOTHING;

UPDATE settings
SET value = 'false'
WHERE key IN (
    'auth_source_default_email_grant_on_signup',
    'auth_source_default_linuxdo_grant_on_signup',
    'auth_source_default_oidc_grant_on_signup',
    'auth_source_default_wechat_grant_on_signup'
);
