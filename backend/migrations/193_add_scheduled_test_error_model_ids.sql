-- 193: Persist the selected failed-model allowlist for error recovery plans.
ALTER TABLE scheduled_test_plans
    ADD COLUMN IF NOT EXISTS model_ids TEXT[] NOT NULL DEFAULT '{}';
