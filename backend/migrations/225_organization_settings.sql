-- Organization-level feature settings.
--
-- One row per organization. Reserved as a growth point for future company-wide
-- toggles; the first switch is `auto_switch_subscription`, which enables
-- transparent fallback of enterprise API keys to another same-platform
-- organization subscription when the currently bound one is exhausted /
-- expired.
CREATE TABLE IF NOT EXISTS organization_settings (
    organization_id             BIGINT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    auto_switch_subscription    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
