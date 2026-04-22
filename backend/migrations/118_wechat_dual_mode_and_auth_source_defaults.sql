INSERT INTO settings (key, value)
VALUES
    (
        'wechat_connect_open_enabled',
        CASE
            WHEN NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wechat_connect_enabled') THEN ''
            WHEN COALESCE((SELECT value FROM settings WHERE key = 'wechat_connect_enabled'), 'false') <> 'true' THEN 'false'
            WHEN LOWER(TRIM(COALESCE((SELECT value FROM settings WHERE key = 'wechat_connect_mode'), 'open'))) = 'mp' THEN 'false'
            ELSE 'true'
        END
    ),
    (
        'wechat_connect_mp_enabled',
        CASE
            WHEN NOT EXISTS (SELECT 1 FROM settings WHERE key = 'wechat_connect_enabled') THEN ''
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
