-- Generalize redeem code usage into a first-class ledger.
-- redeem_codes remains the reward/policy definition table; redeem_code_usages is the source of truth for history.

ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS usage_policy TEXT NOT NULL DEFAULT 'single_use',
    ADD COLUMN IF NOT EXISTS usage_scope TEXT,
    ADD COLUMN IF NOT EXISTS max_total_uses INT DEFAULT 1,
    ADD COLUMN IF NOT EXISTS max_uses_per_user INT,
    ADD COLUMN IF NOT EXISTS used_count INT NOT NULL DEFAULT 0;

UPDATE redeem_codes
SET usage_scope = code
WHERE usage_scope IS NULL OR usage_scope = '';

UPDATE redeem_codes
SET max_total_uses = 1
WHERE max_total_uses IS NULL;

UPDATE redeem_codes
SET used_count = CASE
    WHEN status = 'used' AND used_by IS NOT NULL THEN 1
    ELSE 0
END
WHERE used_count = 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'redeem_codes' AND constraint_name = 'redeem_codes_usage_policy_check'
    ) THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_usage_policy_check
            CHECK (usage_policy IN ('single_use', 'once_per_user'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'redeem_codes' AND constraint_name = 'redeem_codes_used_count_check'
    ) THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_used_count_check
            CHECK (used_count >= 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'redeem_codes' AND constraint_name = 'redeem_codes_max_total_uses_check'
    ) THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_max_total_uses_check
            CHECK (max_total_uses IS NULL OR max_total_uses >= 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'redeem_codes' AND constraint_name = 'redeem_codes_max_uses_per_user_check'
    ) THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_max_uses_per_user_check
            CHECK (max_uses_per_user IS NULL OR max_uses_per_user > 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'redeem_codes' AND constraint_name = 'redeem_codes_once_per_user_policy_check'
    ) THEN
        ALTER TABLE redeem_codes
            ADD CONSTRAINT redeem_codes_once_per_user_policy_check
            CHECK (
                usage_policy <> 'once_per_user'
                OR (
                    usage_scope IS NOT NULL
                    AND usage_scope <> ''
                    AND max_uses_per_user = 1
                )
            );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS redeem_code_usages (
    id BIGSERIAL PRIMARY KEY,
    redeem_code_id BIGINT NOT NULL REFERENCES redeem_codes(id) ON DELETE CASCADE,
    usage_scope TEXT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_snapshot VARCHAR(32) NOT NULL,
    type_snapshot VARCHAR(20) NOT NULL,
    value_snapshot DECIMAL(20,8) NOT NULL DEFAULT 0,
    group_id_snapshot BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    validity_days_snapshot INT NOT NULL DEFAULT 30,
    used_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

INSERT INTO redeem_code_usages (
    redeem_code_id,
    usage_scope,
    user_id,
    code_snapshot,
    type_snapshot,
    value_snapshot,
    group_id_snapshot,
    validity_days_snapshot,
    used_at,
    metadata
)
SELECT
    rc.id,
    COALESCE(NULLIF(rc.usage_scope, ''), rc.code),
    rc.used_by,
    rc.code,
    rc.type,
    rc.value,
    rc.group_id,
    rc.validity_days,
    COALESCE(rc.used_at, rc.created_at, NOW()),
    CASE
        WHEN NULLIF(rc.notes, '') IS NULL THEN '{}'::jsonb
        ELSE jsonb_build_object('notes', rc.notes)
    END
FROM redeem_codes rc
WHERE rc.status = 'used'
  AND rc.used_by IS NOT NULL
ON CONFLICT DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_redeem_codes_usage_scope
    ON redeem_codes (usage_scope);

CREATE INDEX IF NOT EXISTS idx_redeem_codes_usage_policy
    ON redeem_codes (usage_policy);

CREATE INDEX IF NOT EXISTS idx_redeem_code_usages_redeem_code_id
    ON redeem_code_usages (redeem_code_id);

CREATE INDEX IF NOT EXISTS idx_redeem_code_usages_user_id
    ON redeem_code_usages (user_id);

CREATE INDEX IF NOT EXISTS idx_redeem_code_usages_used_at
    ON redeem_code_usages (used_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_redeem_code_usages_scope_user_unique
    ON redeem_code_usages (usage_scope, user_id)
    WHERE usage_scope IS NOT NULL;
