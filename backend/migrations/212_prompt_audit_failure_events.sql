ALTER TABLE prompt_audit_events DROP CONSTRAINT IF EXISTS chk_prompt_audit_events_decision;
ALTER TABLE prompt_audit_events ADD CONSTRAINT chk_prompt_audit_events_decision
    CHECK (decision IN ('pass', 'flag', 'critical', 'failed'));

ALTER TABLE prompt_audit_events DROP CONSTRAINT IF EXISTS chk_prompt_audit_events_risk_level;
ALTER TABLE prompt_audit_events ADD CONSTRAINT chk_prompt_audit_events_risk_level
    CHECK (risk_level IN ('low', 'medium', 'high', 'critical', 'unknown'));

ALTER TABLE prompt_audit_events DROP CONSTRAINT IF EXISTS chk_prompt_audit_events_action;
ALTER TABLE prompt_audit_events ADD CONSTRAINT chk_prompt_audit_events_action
    CHECK (action IN ('Allow', 'Warn', 'Block', 'Error'));
