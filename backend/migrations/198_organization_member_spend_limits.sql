-- Owner-managed company-sponsored spend limits for IAM members.
CREATE TABLE IF NOT EXISTS organization_member_spend_limits (
    id                       BIGSERIAL PRIMARY KEY,
    organization_id          BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- NULL denotes the organization-wide default for all IAM members.
    member_user_id           BIGINT REFERENCES users(id) ON DELETE CASCADE,
    daily_limit_usd          NUMERIC(20, 10),
    monthly_limit_usd        NUMERIC(20, 10),
    alert_enabled            BOOLEAN NOT NULL DEFAULT FALSE,
    alert_threshold_pct      NUMERIC(5, 2) NOT NULL DEFAULT 80,
    additional_recipients    TEXT[] NOT NULL DEFAULT '{}',
    revision                 BIGINT NOT NULL DEFAULT 1,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT organization_spend_limit_amount_required CHECK (
        (daily_limit_usd IS NOT NULL AND daily_limit_usd > 0)
        OR (monthly_limit_usd IS NOT NULL AND monthly_limit_usd > 0)
    ),
    CONSTRAINT organization_spend_limit_daily_positive CHECK (
        daily_limit_usd IS NULL OR daily_limit_usd > 0
    ),
    CONSTRAINT organization_spend_limit_monthly_positive CHECK (
        monthly_limit_usd IS NULL OR monthly_limit_usd > 0
    ),
    CONSTRAINT organization_spend_limit_threshold_range CHECK (
        alert_threshold_pct >= 1 AND alert_threshold_pct <= 100
    ),
    CONSTRAINT organization_spend_limit_revision_positive CHECK (revision > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS organization_spend_limit_all_members_unique
    ON organization_member_spend_limits(organization_id)
    WHERE member_user_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS organization_spend_limit_member_unique
    ON organization_member_spend_limits(organization_id, member_user_id)
    WHERE member_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_organization_spend_limit_member
    ON organization_member_spend_limits(member_user_id)
    WHERE member_user_id IS NOT NULL;
