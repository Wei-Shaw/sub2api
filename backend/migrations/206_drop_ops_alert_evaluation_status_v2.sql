-- The v3 constraint is already validated and enforced before v2 is removed.
-- Old application images only write statuses accepted by the v3 constraint.
-- Keeping this short lock-taking statement in its own transaction avoids
-- retaining ACCESS EXCLUSIVE while historical rows are validated.
-- sub2api-managed-update: reviewed-compatible
ALTER TABLE ops_alert_rule_evaluations
    DROP CONSTRAINT IF EXISTS ops_alert_rule_evaluations_status_check;
