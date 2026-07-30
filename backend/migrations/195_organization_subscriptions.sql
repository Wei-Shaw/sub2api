-- Organization (company) subscriptions.
--
-- A company can hold subscription plans (groups) independently from individual
-- users. This table mirrors user_subscriptions but is scoped to an organization
-- instead of a user. Enterprise API keys (later phase) bind to one of these
-- subscriptions; quota limits are read from the referenced group, while the
-- sliding-window usage counters live on each subscription row.
CREATE TABLE IF NOT EXISTS organization_subscriptions (
    id                      BIGSERIAL PRIMARY KEY,
    organization_id         BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    group_id                BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,

    -- Subscription validity window.
    starts_at               TIMESTAMPTZ NOT NULL,
    expires_at              TIMESTAMPTZ NOT NULL,
    status                  VARCHAR(20) NOT NULL DEFAULT 'active',  -- active/expired/cancelled

    -- Sliding-window starts (NULL = not yet activated).
    daily_window_start      TIMESTAMPTZ,
    weekly_window_start     TIMESTAMPTZ,
    monthly_window_start    TIMESTAMPTZ,

    -- Usage consumed in the current window (USD).
    daily_usage_usd         DECIMAL(20, 10) NOT NULL DEFAULT 0,
    weekly_usage_usd        DECIMAL(20, 10) NOT NULL DEFAULT 0,
    monthly_usage_usd       DECIMAL(20, 10) NOT NULL DEFAULT 0,

    -- Who provisioned the subscription.
    assigned_by             BIGINT REFERENCES users(id) ON DELETE SET NULL,
    assigned_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes                   TEXT,

    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_org_subscriptions_org_id ON organization_subscriptions(organization_id);
CREATE INDEX IF NOT EXISTS idx_org_subscriptions_group_id ON organization_subscriptions(group_id);
CREATE INDEX IF NOT EXISTS idx_org_subscriptions_status ON organization_subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_org_subscriptions_expires_at ON organization_subscriptions(expires_at);

-- Each organization can have at most one live (non-deleted) subscription per group.
CREATE UNIQUE INDEX IF NOT EXISTS idx_org_subscriptions_org_group_active
    ON organization_subscriptions(organization_id, group_id)
    WHERE deleted_at IS NULL;
