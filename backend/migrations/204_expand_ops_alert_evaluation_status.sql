-- Add the expanded status constraint first so the old application can keep
-- writing during validation. The new statuses are a strict superset of v2.
-- sub2api-managed-update: reviewed-compatible
ALTER TABLE ops_alert_rule_evaluations
    ADD CONSTRAINT ops_alert_rule_evaluations_status_check_v3 CHECK (
        status IN (
            'ok',
            'breached',
            'insufficient_samples',
            'insufficient_bad_count',
            'no_data',
            'stale',
            'error',
            'unsupported',
            'disabled',
            'shadow'
        )
    ) NOT VALID;
