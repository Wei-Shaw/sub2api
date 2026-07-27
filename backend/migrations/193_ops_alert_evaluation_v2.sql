-- Additive alert-evaluation v2 schema. Older application images ignore these
-- columns and tables, which keeps blue-green rollback compatible.
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS incident_family VARCHAR(64) NOT NULL DEFAULT 'custom',
    ADD COLUMN IF NOT EXISTS minimum_samples INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS minimum_bad_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS recovery_operator VARCHAR(8),
    ADD COLUMN IF NOT EXISTS recovery_threshold DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS recovery_sustained_minutes INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS shadow_mode BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS email_queued BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS ops_alert_rule_evaluations (
    id                  BIGSERIAL PRIMARY KEY,
    rule_id             BIGINT NOT NULL,
    evaluated_at        TIMESTAMPTZ NOT NULL,
    window_start        TIMESTAMPTZ NOT NULL,
    window_end          TIMESTAMPTZ NOT NULL,
    status              VARCHAR(32) NOT NULL,
    breached            BOOLEAN NOT NULL DEFAULT FALSE,
    metric_value        DOUBLE PRECISION,
    threshold_value     DOUBLE PRECISION,
    sample_count        BIGINT NOT NULL DEFAULT 0,
    bad_count           BIGINT NOT NULL DEFAULT 0,
    data_as_of          TIMESTAMPTZ,
    error_code          VARCHAR(64),
    error_message       TEXT,
    evaluator_version   VARCHAR(32) NOT NULL DEFAULT 'v2',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ops_alert_rule_evaluations_status_check CHECK (
        status IN ('ok', 'breached', 'no_data', 'stale', 'error', 'unsupported', 'disabled', 'shadow')
    )
);

CREATE INDEX IF NOT EXISTS idx_ops_alert_rule_evaluations_rule_time
    ON ops_alert_rule_evaluations (rule_id, evaluated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_ops_alert_rule_evaluations_time
    ON ops_alert_rule_evaluations (evaluated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS ops_alert_rule_states (
    rule_id                 BIGINT PRIMARY KEY,
    last_evaluated_at       TIMESTAMPTZ,
    consecutive_breaches   INTEGER NOT NULL DEFAULT 0,
    consecutive_recoveries INTEGER NOT NULL DEFAULT 0,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- These are the exact legacy defaults created by migration 033. They are
-- either duplicate or unsupported by the evaluator and must not remain
-- deceptively enabled after this migration.
UPDATE ops_alert_rules
SET enabled = FALSE,
    description = trim(concat_ws(' ', NULLIF(description, ''), '[disabled: duplicate of error_rate]')),
    updated_at = NOW()
WHERE name = '成功率过低' AND metric_type = 'success_rate'
  AND enabled = TRUE AND operator = '<' AND threshold = 95.0
  AND window_minutes = 5 AND sustained_minutes = 5
  AND severity = 'P0' AND cooldown_minutes = 15
  AND description = '当成功率低于 95% 且持续 5 分钟时触发告警（服务可用性下降）';

UPDATE ops_alert_rules
SET enabled = FALSE,
    description = trim(concat_ws(' ', NULLIF(description, ''), '[disabled: unsupported legacy latency metric]')),
    updated_at = NOW()
WHERE name IN ('P95延迟过高', 'P99延迟过高')
  AND metric_type IN ('p95_latency_ms', 'p99_latency_ms')
  AND enabled = TRUE AND operator = '>' AND window_minutes = 5
  AND sustained_minutes = 10 AND severity = 'P2' AND cooldown_minutes = 30
  AND ((name = 'P95延迟过高' AND metric_type = 'p95_latency_ms' AND threshold = 2000.0)
    OR (name = 'P99延迟过高' AND metric_type = 'p99_latency_ms' AND threshold = 3000.0));

UPDATE ops_alert_rules
SET incident_family = 'availability',
    minimum_samples = 100,
    minimum_bad_count = 10,
    recovery_operator = '<',
    recovery_threshold = 2.5,
    recovery_sustained_minutes = 5,
    updated_at = NOW()
WHERE name = '错误率过高' AND metric_type = 'error_rate'
  AND enabled = TRUE AND operator = '>' AND threshold = 5.0
  AND window_minutes = 5 AND sustained_minutes = 5
  AND severity = 'P1' AND cooldown_minutes = 20;

UPDATE ops_alert_rules
SET incident_family = 'availability',
    minimum_samples = 30,
    minimum_bad_count = 10,
    sustained_minutes = GREATEST(sustained_minutes, 3),
    recovery_operator = '<',
    recovery_threshold = 10,
    recovery_sustained_minutes = 5,
    updated_at = NOW()
WHERE name = '错误率极高' AND metric_type = 'error_rate'
  AND enabled = TRUE AND operator = '>' AND threshold = 20.0
  AND window_minutes = 1 AND sustained_minutes = 1
  AND severity = 'P0' AND cooldown_minutes = 15;

UPDATE ops_alert_rules
SET incident_family = CASE
        WHEN metric_type IN ('cpu_usage_percent', 'memory_usage_percent') THEN 'resource_capacity'
        WHEN metric_type = 'concurrency_queue_depth' THEN 'request_queue'
        ELSE incident_family
    END,
    recovery_operator = CASE
        WHEN metric_type = 'cpu_usage_percent' THEN '<'
        WHEN metric_type = 'memory_usage_percent' THEN '<'
        WHEN metric_type = 'concurrency_queue_depth' THEN '<'
        ELSE recovery_operator
    END,
    recovery_threshold = CASE
        WHEN metric_type = 'cpu_usage_percent' THEN 75
        WHEN metric_type = 'memory_usage_percent' THEN 85
        WHEN metric_type = 'concurrency_queue_depth' THEN 50
        ELSE recovery_threshold
    END,
    recovery_sustained_minutes = CASE
        WHEN metric_type IN ('cpu_usage_percent', 'memory_usage_percent', 'concurrency_queue_depth') THEN 5
        ELSE recovery_sustained_minutes
    END,
    updated_at = NOW()
WHERE enabled = TRUE AND operator = '>' AND (
    (name = 'CPU使用率过高' AND metric_type = 'cpu_usage_percent' AND threshold = 85.0
        AND window_minutes = 5 AND sustained_minutes = 10 AND severity = 'P2' AND cooldown_minutes = 30)
 OR (name = '内存使用率过高' AND metric_type = 'memory_usage_percent' AND threshold = 90.0
        AND window_minutes = 5 AND sustained_minutes = 10 AND severity = 'P1' AND cooldown_minutes = 20)
 OR (name = '并发队列积压' AND metric_type = 'concurrency_queue_depth' AND threshold = 100.0
        AND window_minutes = 5 AND sustained_minutes = 5 AND severity = 'P1' AND cooldown_minutes = 20)
);
