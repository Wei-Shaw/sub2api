-- Keep validation in its own migration transaction. This releases the
-- ACCESS EXCLUSIVE lock from migration 204 before PostgreSQL scans existing
-- rows under the weaker validation lock.
-- sub2api-managed-update: reviewed-compatible
ALTER TABLE ops_alert_rule_evaluations
    VALIDATE CONSTRAINT ops_alert_rule_evaluations_status_check_v3;
