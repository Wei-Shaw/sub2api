ALTER TABLE scheduled_test_plans
    ADD COLUMN IF NOT EXISTS include_in_health_samples BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN scheduled_test_plans.include_in_health_samples IS
    'When true, scheduled test outcomes contribute a low-weight signal to high-availability account health';
