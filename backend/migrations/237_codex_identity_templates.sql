-- Reusable Codex identity templates. Templates are the control-plane source of
-- truth; account-specific policy/profile/slot rows remain runtime projections.

CREATE TABLE IF NOT EXISTS codex_identity_templates (
    id                    BIGSERIAL PRIMARY KEY,
    name                  VARCHAR(100) NOT NULL,
    description           VARCHAR(500) NOT NULL DEFAULT '',
    revision              BIGINT NOT NULL DEFAULT 1,
    session_policy        JSONB NOT NULL DEFAULT '{"mode":"conversation_isolated"}'::jsonb,
    affinity_ttl_seconds  INTEGER NOT NULL DEFAULT 3600,
    unsupported_policy    VARCHAR(40) NOT NULL DEFAULT 'reject',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT codex_identity_template_name_check
        CHECK (BTRIM(name) <> ''),
    CONSTRAINT codex_identity_template_revision_check
        CHECK (revision > 0),
    CONSTRAINT codex_identity_template_session_policy_check
        CHECK (
            jsonb_typeof(session_policy) = 'object'
            AND COALESCE(session_policy->>'mode', '') IN
                ('conversation_isolated', 'api_key_shared', 'session_pool', 'device_shared')
            AND (
                COALESCE(session_policy->>'mode', '') <> 'device_shared'
                OR (
                    COALESCE((session_policy->>'max_active_conversations_per_slot')::INTEGER, 0) = 1
                    AND COALESCE((session_policy->>'disable_cross_key_continuation')::BOOLEAN, FALSE) = TRUE
                )
            )
        ),
    CONSTRAINT codex_identity_template_affinity_ttl_check
        CHECK (affinity_ttl_seconds BETWEEN 60 AND 86400),
    CONSTRAINT codex_identity_template_unsupported_policy_check
        CHECK (unsupported_policy = 'reject')
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_codex_identity_templates_name_ci
    ON codex_identity_templates (LOWER(BTRIM(name)));

CREATE TABLE IF NOT EXISTS codex_identity_template_profiles (
    id                    BIGSERIAL PRIMARY KEY,
    template_id           BIGINT NOT NULL REFERENCES codex_identity_templates(id) ON DELETE CASCADE,
    os_class              VARCHAR(20) NOT NULL,
    canonical_surface     VARCHAR(20) NOT NULL,
    architecture          VARCHAR(20),
    proxy_mode            VARCHAR(20) NOT NULL DEFAULT 'inherit',
    proxy_id              BIGINT REFERENCES proxies(id) ON DELETE RESTRICT,
    slot_count            INTEGER NOT NULL,
    catalog_version       BIGINT NOT NULL DEFAULT 1,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT codex_identity_template_profile_os_check
        CHECK (os_class IN ('windows', 'macos', 'linux', 'generic')),
    CONSTRAINT codex_identity_template_profile_surface_check
        CHECK (canonical_surface IN ('desktop', 'cli', 'sdk', 'third_party')),
    CONSTRAINT codex_identity_template_profile_arch_check
        CHECK (architecture IS NULL OR architecture IN ('x86_64', 'arm64')),
    CONSTRAINT codex_identity_template_profile_proxy_mode_check
        CHECK (proxy_mode IN ('inherit', 'proxy', 'direct')),
    CONSTRAINT codex_identity_template_profile_proxy_shape_check
        CHECK (
            (proxy_mode = 'proxy' AND proxy_id IS NOT NULL)
            OR (proxy_mode IN ('inherit', 'direct') AND proxy_id IS NULL)
        ),
    CONSTRAINT codex_identity_template_profile_slot_count_check
        CHECK (slot_count BETWEEN 1 AND 3),
    CONSTRAINT codex_identity_template_profile_catalog_version_check
        CHECK (catalog_version = 1),
    CONSTRAINT codex_identity_template_profile_shape_check
        CHECK (
            (os_class = 'generic' AND canonical_surface IN ('sdk', 'third_party') AND architecture IS NULL)
            OR
            (os_class <> 'generic' AND canonical_surface IN ('desktop', 'cli') AND architecture IS NOT NULL)
        ),
    UNIQUE(template_id, os_class, canonical_surface),
    UNIQUE(id, template_id)
);

CREATE TABLE IF NOT EXISTS codex_identity_template_slots (
    id                    BIGSERIAL PRIMARY KEY,
    template_id           BIGINT NOT NULL REFERENCES codex_identity_templates(id) ON DELETE CASCADE,
    profile_id            BIGINT NOT NULL,
    slot_index            INTEGER NOT NULL,
    proxy_mode            VARCHAR(20) NOT NULL DEFAULT 'inherit',
    proxy_id              BIGINT REFERENCES proxies(id) ON DELETE RESTRICT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT codex_identity_template_slot_profile_fk
        FOREIGN KEY(profile_id, template_id)
        REFERENCES codex_identity_template_profiles(id, template_id)
        ON DELETE CASCADE,
    CONSTRAINT codex_identity_template_slot_proxy_mode_check
        CHECK (proxy_mode IN ('inherit', 'proxy', 'direct')),
    CONSTRAINT codex_identity_template_slot_proxy_shape_check
        CHECK (
            (proxy_mode = 'proxy' AND proxy_id IS NOT NULL)
            OR (proxy_mode IN ('inherit', 'direct') AND proxy_id IS NULL)
        ),
    CONSTRAINT codex_identity_template_slot_index_check
        CHECK (slot_index BETWEEN 0 AND 2),
    UNIQUE(profile_id, slot_index)
);

CREATE INDEX IF NOT EXISTS idx_codex_identity_template_profiles_template
    ON codex_identity_template_profiles(template_id, os_class, canonical_surface);
CREATE INDEX IF NOT EXISTS idx_codex_identity_template_profiles_proxy
    ON codex_identity_template_profiles(proxy_id) WHERE proxy_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_codex_identity_template_slots_template
    ON codex_identity_template_slots(template_id, profile_id, slot_index);
CREATE INDEX IF NOT EXISTS idx_codex_identity_template_slots_proxy
    ON codex_identity_template_slots(proxy_id) WHERE proxy_id IS NOT NULL;

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS codex_identity_template_id BIGINT;
ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS codex_identity_template_applied_revision BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'accounts_codex_identity_template_fk'
          AND conrelid = 'accounts'::regclass
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_codex_identity_template_fk
            FOREIGN KEY(codex_identity_template_id)
            REFERENCES codex_identity_templates(id)
            ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'accounts_codex_identity_template_revision_check'
          AND conrelid = 'accounts'::regclass
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_codex_identity_template_revision_check
            CHECK (
                (codex_identity_template_id IS NULL AND codex_identity_template_applied_revision IS NULL)
                OR
                (codex_identity_template_id IS NOT NULL AND codex_identity_template_applied_revision > 0)
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_accounts_codex_identity_template
    ON accounts(codex_identity_template_id)
    WHERE deleted_at IS NULL AND codex_identity_template_id IS NOT NULL;

-- Preserve any account-owned policies created before templates existed. Each
-- enabled account receives one named template; off accounts remain unassigned.
INSERT INTO codex_identity_templates
    (name, description, revision, session_policy, affinity_ttl_seconds, unsupported_policy)
SELECT '迁移账号 ' || accounts.id::TEXT,
       '由升级自动迁移，原账号：' || accounts.name,
       1,
       COALESCE(accounts.codex_identity_policy->'session_policy', '{"mode":"conversation_isolated"}'::jsonb),
       COALESCE((accounts.codex_identity_policy->>'affinity_ttl_seconds')::INTEGER, 3600),
       COALESCE(NULLIF(accounts.codex_identity_policy->>'unsupported_policy', ''), 'reject')
FROM accounts
WHERE accounts.deleted_at IS NULL
  AND COALESCE(accounts.codex_identity_policy->>'mode', 'off') = 'os_profile_device_pool'
  AND accounts.codex_identity_template_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO codex_identity_template_profiles
    (template_id, os_class, canonical_surface, architecture, proxy_mode, proxy_id, slot_count, catalog_version)
SELECT templates.id,
       profile->>'os_class',
       profile->>'canonical_surface',
       NULLIF(profile->>'architecture', ''),
       COALESCE(NULLIF(profile->>'proxy_mode', ''),
                CASE WHEN profile ? 'proxy_id' THEN 'proxy' ELSE 'inherit' END),
       CASE WHEN COALESCE(NULLIF(profile->>'proxy_mode', ''),
                          CASE WHEN profile ? 'proxy_id' THEN 'proxy' ELSE 'inherit' END) = 'proxy'
            THEN (profile->>'proxy_id')::BIGINT ELSE NULL END,
       (profile->>'slot_count')::INTEGER,
       COALESCE((profile->>'catalog_version')::BIGINT, 1)
FROM accounts
JOIN codex_identity_templates AS templates
  ON templates.name = '迁移账号 ' || accounts.id::TEXT
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(accounts.codex_identity_policy->'profiles', '[]'::jsonb)) AS profile
WHERE accounts.deleted_at IS NULL
  AND accounts.codex_identity_template_id IS NULL
ON CONFLICT (template_id, os_class, canonical_surface) DO NOTHING;

INSERT INTO codex_identity_template_slots
    (template_id, profile_id, slot_index, proxy_mode, proxy_id)
SELECT templates.id,
       template_profiles.id,
       (slot->>'index')::INTEGER,
       COALESCE(NULLIF(slot->>'proxy_mode', ''),
                CASE WHEN slot ? 'proxy_id' THEN 'proxy' ELSE 'inherit' END),
       CASE WHEN COALESCE(NULLIF(slot->>'proxy_mode', ''),
                          CASE WHEN slot ? 'proxy_id' THEN 'proxy' ELSE 'inherit' END) = 'proxy'
            THEN (slot->>'proxy_id')::BIGINT ELSE NULL END
FROM accounts
JOIN codex_identity_templates AS templates
  ON templates.name = '迁移账号 ' || accounts.id::TEXT
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(accounts.codex_identity_policy->'profiles', '[]'::jsonb)) AS profile
JOIN codex_identity_template_profiles AS template_profiles
  ON template_profiles.template_id=templates.id
 AND template_profiles.os_class=profile->>'os_class'
 AND template_profiles.canonical_surface=profile->>'canonical_surface'
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(profile->'slots', '[]'::jsonb)) AS slot
WHERE accounts.deleted_at IS NULL
  AND accounts.codex_identity_template_id IS NULL
ON CONFLICT (profile_id, slot_index) DO NOTHING;

UPDATE accounts
SET codex_identity_template_id=templates.id,
    codex_identity_template_applied_revision=templates.revision,
    updated_at=NOW()
FROM codex_identity_templates AS templates
WHERE accounts.deleted_at IS NULL
  AND accounts.codex_identity_template_id IS NULL
  AND COALESCE(accounts.codex_identity_policy->>'mode', 'off') = 'os_profile_device_pool'
  AND templates.name = '迁移账号 ' || accounts.id::TEXT;
