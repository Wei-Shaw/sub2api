-- Add error-triggered scheduled tests while preserving existing cron plans.
ALTER TABLE scheduled_test_plans
    ADD COLUMN IF NOT EXISTS trigger_mode VARCHAR(20) NOT NULL DEFAULT 'scheduled',
    ADD COLUMN IF NOT EXISTS retry_interval_minutes INT,
    ADD COLUMN IF NOT EXISTS retry_cron_expression VARCHAR(100);

ALTER TABLE scheduled_test_plans DROP CONSTRAINT IF EXISTS chk_stp_trigger_mode;
ALTER TABLE scheduled_test_plans ADD CONSTRAINT chk_stp_trigger_mode
    CHECK (trigger_mode IN ('scheduled', 'error_recovery'));

ALTER TABLE scheduled_test_plans DROP CONSTRAINT IF EXISTS chk_stp_recovery_schedule;
ALTER TABLE scheduled_test_plans ADD CONSTRAINT chk_stp_recovery_schedule CHECK (
    trigger_mode = 'scheduled' OR
    ((retry_interval_minutes BETWEEN 1 AND 1440 AND retry_cron_expression IS NULL) OR
     (retry_interval_minutes IS NULL AND NULLIF(BTRIM(retry_cron_expression), '') IS NOT NULL))
);

ALTER TABLE scheduled_test_results ADD COLUMN IF NOT EXISTS model_id VARCHAR(100);
UPDATE scheduled_test_results r
SET model_id = p.model_id
FROM scheduled_test_plans p
WHERE p.id = r.plan_id AND r.model_id IS NULL;
ALTER TABLE scheduled_test_results ALTER COLUMN model_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_stp_account_error_recovery
    ON scheduled_test_plans(account_id) WHERE trigger_mode = 'error_recovery';
CREATE INDEX IF NOT EXISTS idx_str_plan_model_created
    ON scheduled_test_results(plan_id, model_id, created_at DESC);
