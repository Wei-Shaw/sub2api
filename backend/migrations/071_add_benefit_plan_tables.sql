-- Benefit plan MVP:
-- - benefit_packages: reusable package = one subscription group + lease days
-- - benefit_plans: plan made of multiple packages
-- - user_plan_assignments: one active plan per user
-- - user_subscriptions.plan_days_applied: how many days are currently contributed by plan

CREATE TABLE IF NOT EXISTS benefit_packages (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    lease_days INTEGER NOT NULL CHECK (lease_days > 0 AND lease_days <= 36500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_benefit_packages_group_id ON benefit_packages (group_id);

CREATE TABLE IF NOT EXISTS benefit_plans (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS benefit_plan_packages (
    plan_id BIGINT NOT NULL REFERENCES benefit_plans(id) ON DELETE CASCADE,
    package_id BIGINT NOT NULL REFERENCES benefit_packages(id) ON DELETE RESTRICT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (plan_id, package_id)
);

CREATE INDEX IF NOT EXISTS idx_benefit_plan_packages_package_id ON benefit_plan_packages (package_id);

CREATE TABLE IF NOT EXISTS user_plan_assignments (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    plan_id BIGINT NOT NULL REFERENCES benefit_plans(id) ON DELETE RESTRICT,
    version BIGINT NOT NULL DEFAULT 1,
    assigned_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_plan_assignments_plan_id ON user_plan_assignments (plan_id);

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS plan_days_applied INTEGER NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'user_subscriptions_plan_days_applied_non_negative'
    ) THEN
        ALTER TABLE user_subscriptions
            ADD CONSTRAINT user_subscriptions_plan_days_applied_non_negative
            CHECK (plan_days_applied >= 0);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_user_subscriptions_user_plan_days_applied
    ON user_subscriptions (user_id, plan_days_applied)
    WHERE deleted_at IS NULL;
