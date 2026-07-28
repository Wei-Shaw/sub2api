-- Additive, versioned error classification for SLA and alerting. Older images
-- ignore these columns; conservative defaults keep rollback-period failures
-- visible instead of silently excluding them.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS final_outcome VARCHAR(32) NOT NULL DEFAULT 'unknown_failed',
    ADD COLUMN IF NOT EXISTS responsibility VARCHAR(32) NOT NULL DEFAULT 'platform',
    ADD COLUMN IF NOT EXISTS error_category VARCHAR(64) NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS counts_toward_sla BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS alert_family VARCHAR(64) NOT NULL DEFAULT 'unknown_failure',
    ADD COLUMN IF NOT EXISTS classification_reason VARCHAR(128) NOT NULL DEFAULT 'legacy_unclassified',
    ADD COLUMN IF NOT EXISTS classification_version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE ops_metrics_hourly
	ADD COLUMN IF NOT EXISTS metric_definition_version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE ops_metrics_daily
	ADD COLUMN IF NOT EXISTS metric_definition_version INTEGER NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_ops_metrics_hourly_v2_bucket
	ON ops_metrics_hourly (bucket_start DESC)
	WHERE metric_definition_version = 2;

CREATE INDEX IF NOT EXISTS idx_ops_metrics_daily_v2_bucket
	ON ops_metrics_daily (bucket_date DESC)
	WHERE metric_definition_version = 2;

UPDATE ops_error_logs
SET final_outcome = CASE
        WHEN COALESCE(status_code, 0) > 0 AND status_code < 400 THEN 'recovered'
		WHEN status_code IN (408, 499) OR lower(COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ~
            '(context canceled|client disconnected|client closed|request canceled|broken pipe)' THEN 'cancelled'
		WHEN error_type = 'cyber_policy' OR lower(COALESCE(error_message, '')) ~
			'(cyber policy|content policy|security policy|turnstile verification)' THEN 'security_blocked'
        WHEN lower(COALESCE(error_message, '')) ~
            '(no available accounts|concurrency limit exceeded for account|too many pending requests)' THEN 'platform_failed'
		WHEN lower(COALESCE(error_message, '')) ~ '(model.*(not supported|not in whitelist|not configured))' THEN 'client_rejected'
        WHEN COALESCE(upstream_status_code, 0) IN (401, 403) OR error_phase = 'account_auth' THEN 'platform_failed'
        WHEN COALESCE(upstream_status_code, 0) BETWEEN 400 AND 499 AND upstream_status_code <> 429 THEN 'client_rejected'
        WHEN COALESCE(upstream_status_code, 0) = 429 OR COALESCE(upstream_status_code, 0) >= 500 THEN 'provider_failed'
        WHEN COALESCE(is_business_limited, FALSE) THEN 'business_limited'
        WHEN COALESCE(status_code, 0) BETWEEN 400 AND 499 THEN 'client_rejected'
        WHEN COALESCE(status_code, 0) >= 500 THEN 'platform_failed'
        ELSE 'unknown_failed'
    END,
    responsibility = CASE
        WHEN COALESCE(status_code, 0) > 0 AND status_code < 400 THEN
            CASE
				WHEN lower(COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ~
					'(context canceled|client disconnected|client closed|request canceled|broken pipe)' THEN 'client'
                WHEN COALESCE(upstream_status_code, 0) IN (401, 403) OR error_phase = 'account_auth' THEN 'platform'
                WHEN COALESCE(upstream_status_code, 0) = 429 OR COALESCE(upstream_status_code, 0) >= 500 THEN 'provider'
                WHEN COALESCE(upstream_status_code, 0) BETWEEN 400 AND 499 THEN 'client'
                ELSE 'unknown'
            END
		WHEN status_code IN (408, 499) OR lower(COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ~
            '(context canceled|client disconnected|client closed|request canceled|broken pipe)' THEN 'client'
		WHEN error_type = 'cyber_policy' OR lower(COALESCE(error_message, '')) ~
			'(cyber policy|content policy|security policy|turnstile verification)' THEN 'client'
        WHEN lower(COALESCE(error_message, '')) ~
            '(no available accounts|concurrency limit exceeded for account|too many pending requests)' THEN 'platform'
        WHEN COALESCE(upstream_status_code, 0) IN (401, 403) OR error_phase = 'account_auth' THEN 'platform'
        WHEN COALESCE(upstream_status_code, 0) BETWEEN 400 AND 499 AND upstream_status_code <> 429 THEN 'client'
        WHEN COALESCE(upstream_status_code, 0) = 429 OR COALESCE(upstream_status_code, 0) >= 500 THEN 'provider'
        WHEN COALESCE(is_business_limited, FALSE) OR COALESCE(status_code, 0) BETWEEN 400 AND 499 THEN 'client'
        WHEN COALESCE(status_code, 0) >= 500 THEN 'platform'
        ELSE 'unknown'
    END,
    error_category = CASE
        WHEN COALESCE(status_code, 0) > 0 AND status_code < 400 THEN 'recovered'
		WHEN status_code IN (408, 499) OR lower(COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ~
            '(context canceled|client disconnected|client closed|request canceled|broken pipe)' THEN 'client_cancelled'
		WHEN error_type = 'cyber_policy' OR lower(COALESCE(error_message, '')) ~
			'(cyber policy|content policy|security policy|turnstile verification)' THEN 'security_policy'
        WHEN lower(COALESCE(error_message, '')) LIKE '%no available accounts%' THEN 'platform_capacity'
        WHEN lower(COALESCE(error_message, '')) ~
            '(concurrency limit exceeded for account|too many pending requests)' THEN 'platform_capacity'
		WHEN lower(COALESCE(error_message, '')) ~ '(model.*(not supported|not in whitelist|not configured))' THEN 'unsupported_model'
        WHEN COALESCE(upstream_status_code, 0) IN (401, 403) OR error_phase = 'account_auth' THEN 'platform_credential'
        WHEN COALESCE(upstream_status_code, 0) = 429 THEN 'provider_rate_limit'
        WHEN COALESCE(upstream_status_code, 0) = 529 OR lower(COALESCE(error_message, '')) LIKE '%overload%' THEN 'provider_overloaded'
        WHEN COALESCE(upstream_status_code, 0) >= 500 THEN 'provider_server'
        WHEN COALESCE(upstream_status_code, 0) BETWEEN 400 AND 499 AND upstream_status_code <> 429 AND status_code >= 500 THEN 'product_compatibility'
        WHEN COALESCE(upstream_status_code, 0) BETWEEN 400 AND 499 AND upstream_status_code <> 429 THEN 'invalid_request'
        WHEN COALESCE(is_business_limited, FALSE) THEN
            CASE WHEN lower(COALESCE(error_message, '')) LIKE '%concurrency limit exceeded for user%'
                THEN 'user_concurrency' ELSE 'user_quota' END
        WHEN error_phase = 'auth' OR status_code IN (401, 403) THEN 'client_auth'
        WHEN COALESCE(status_code, 0) BETWEEN 400 AND 499 THEN 'invalid_request'
        WHEN error_phase = 'network' THEN 'network_transport'
        WHEN COALESCE(status_code, 0) >= 500 THEN 'platform_internal'
        ELSE 'unknown'
    END,
    counts_toward_sla = CASE
        WHEN COALESCE(status_code, 0) > 0 AND status_code < 400 THEN FALSE
		WHEN status_code IN (408, 499) OR lower(COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ~
            '(context canceled|client disconnected|client closed|request canceled|broken pipe)' THEN FALSE
		WHEN error_type = 'cyber_policy' OR lower(COALESCE(error_message, '')) ~
			'(cyber policy|content policy|security policy|turnstile verification)' THEN FALSE
        WHEN lower(COALESCE(error_message, '')) ~
            '(no available accounts|concurrency limit exceeded for account|too many pending requests)' THEN TRUE
		WHEN lower(COALESCE(error_message, '')) ~ '(model.*(not supported|not in whitelist|not configured))' THEN FALSE
        WHEN COALESCE(upstream_status_code, 0) IN (401, 403) OR error_phase = 'account_auth' THEN TRUE
        WHEN COALESCE(upstream_status_code, 0) BETWEEN 400 AND 499 AND upstream_status_code <> 429 THEN FALSE
        WHEN COALESCE(upstream_status_code, 0) = 429 OR COALESCE(upstream_status_code, 0) >= 500 THEN TRUE
        WHEN COALESCE(is_business_limited, FALSE) THEN FALSE
        WHEN COALESCE(status_code, 0) BETWEEN 400 AND 499 THEN FALSE
        WHEN COALESCE(status_code, 0) >= 500 THEN TRUE
        ELSE TRUE
    END,
    alert_family = CASE
		WHEN COALESCE(status_code, 0) > 0 AND status_code < 400 AND
			lower(COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ~
			'(context canceled|client disconnected|client closed|request canceled|broken pipe)' THEN 'client_quality'
		WHEN COALESCE(status_code, 0) > 0 AND status_code < 400 AND
            (COALESCE(upstream_status_code, 0) = 429 OR COALESCE(upstream_status_code, 0) >= 500 OR error_phase IN ('upstream', 'account_auth'))
            THEN 'provider_health'
        WHEN COALESCE(status_code, 0) > 0 AND status_code < 400 THEN 'none'
		WHEN status_code IN (408, 499) OR lower(COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ~
            '(context canceled|client disconnected|client closed|request canceled|broken pipe)' THEN 'client_quality'
		WHEN error_type = 'cyber_policy' OR lower(COALESCE(error_message, '')) ~
			'(cyber policy|content policy|security policy|turnstile verification)' THEN 'security'
        WHEN lower(COALESCE(error_message, '')) ~
            '(no available accounts|concurrency limit exceeded for account|too many pending requests)' THEN 'capacity'
		WHEN lower(COALESCE(error_message, '')) ~ '(model.*(not supported|not in whitelist|not configured))' THEN 'compatibility'
        WHEN COALESCE(upstream_status_code, 0) IN (401, 403) OR error_phase = 'account_auth' THEN 'capacity'
        WHEN COALESCE(upstream_status_code, 0) BETWEEN 400 AND 499 AND upstream_status_code <> 429 AND status_code >= 500 THEN 'compatibility'
        WHEN COALESCE(upstream_status_code, 0) BETWEEN 400 AND 499 AND upstream_status_code <> 429 THEN 'client_quality'
        WHEN COALESCE(upstream_status_code, 0) = 429 OR COALESCE(upstream_status_code, 0) >= 500 THEN 'provider_health'
        WHEN COALESCE(is_business_limited, FALSE) OR COALESCE(status_code, 0) BETWEEN 400 AND 499 THEN 'client_quality'
        WHEN COALESCE(status_code, 0) >= 500 THEN 'availability'
        ELSE 'unknown_failure'
    END,
    classification_reason = 'migration_v2_backfill',
    classification_version = 2;

-- The legacy business flag intentionally grouped quota, authentication and
-- local access/feature policies so all three stayed out of the old SLA. Split
-- those meanings for V2 without changing their SLA eligibility.
UPDATE ops_error_logs
SET final_outcome = CASE
        WHEN error_type = 'authentication_error' OR status_code = 401 OR
            lower(COALESCE(error_message, '')) ~
                '(invalid api key|api key is required|api key is expired|api key is disabled|user associated with api key not found|user account is not active|api key 所属分组已删除|api key 所属分组已停用|api key is not assigned to any group)'
            THEN 'client_rejected'
        WHEN error_type IN ('billing_error', 'subscription_error') OR status_code = 429 OR
            lower(COALESCE(error_message, '')) ~
                '(insufficient.*balance|quota exhausted|usage limit exceeded|subscription|concurrency limit exceeded for user|requests-per-minute limit exceeded)'
            THEN 'business_limited'
        ELSE 'client_rejected'
    END,
    responsibility = 'client',
    error_category = CASE
        WHEN error_type = 'authentication_error' OR status_code = 401 OR
            lower(COALESCE(error_message, '')) ~
                '(invalid api key|api key is required|api key is expired|api key is disabled|user associated with api key not found|user account is not active|api key 所属分组已删除|api key 所属分组已停用|api key is not assigned to any group)'
            THEN 'client_auth'
        WHEN error_type = 'subscription_error' OR lower(COALESCE(error_message, '')) LIKE '%subscription%'
            THEN 'user_subscription'
        WHEN lower(COALESCE(error_message, '')) LIKE '%concurrency limit exceeded for user%'
            THEN 'user_concurrency'
        WHEN error_type = 'billing_error' OR status_code = 429 OR
            lower(COALESCE(error_message, '')) ~
                '(insufficient.*balance|quota exhausted|usage limit exceeded|requests-per-minute limit exceeded)'
            THEN 'user_quota'
        ELSE 'client_policy'
    END,
    counts_toward_sla = FALSE,
    alert_family = 'client_quality',
    classification_reason = CASE
        WHEN error_type = 'authentication_error' OR status_code = 401 OR
            lower(COALESCE(error_message, '')) ~
                '(invalid api key|api key is required|api key is expired|api key is disabled|user associated with api key not found|user account is not active|api key 所属分组已删除|api key 所属分组已停用|api key is not assigned to any group)'
            THEN 'migration_v2_local_auth'
        WHEN error_type IN ('billing_error', 'subscription_error') OR status_code = 429 OR
            lower(COALESCE(error_message, '')) ~
                '(insufficient.*balance|quota exhausted|usage limit exceeded|subscription|concurrency limit exceeded for user|requests-per-minute limit exceeded)'
            THEN 'migration_v2_user_business_limit'
        ELSE 'migration_v2_local_policy'
    END
WHERE classification_version = 2
  AND COALESCE(is_business_limited, FALSE)
  AND COALESCE(status_code, 0) >= 400
  AND COALESCE(error_phase, '') NOT IN ('upstream', 'account_auth')
  AND error_category NOT IN ('platform_capacity', 'security_policy', 'client_cancelled', 'unsupported_model');

-- Some older upstream paths preserved the upstream HTTP status as the client
-- status but did not populate upstream_status_code. Recover only unambiguous
-- 4xx/529 semantics; synthetic 5xx values remain conservative platform/network
-- failures because their provider origin cannot be proven.
UPDATE ops_error_logs
SET final_outcome = CASE
        WHEN status_code IN (401, 403) THEN 'platform_failed'
        WHEN status_code IN (429, 529) THEN 'provider_failed'
        ELSE 'client_rejected'
    END,
    responsibility = CASE
        WHEN status_code IN (401, 403) THEN 'platform'
        WHEN status_code IN (429, 529) THEN 'provider'
        ELSE 'client'
    END,
    error_category = CASE
        WHEN status_code IN (401, 403) THEN 'platform_credential'
        WHEN status_code = 429 THEN 'provider_rate_limit'
        WHEN status_code = 529 THEN 'provider_overloaded'
        ELSE 'invalid_request'
    END,
    counts_toward_sla = status_code IN (401, 403, 429, 529),
    alert_family = CASE
        WHEN status_code IN (401, 403) THEN 'capacity'
        WHEN status_code IN (429, 529) THEN 'provider_health'
        ELSE 'client_quality'
    END,
    classification_reason = 'migration_v2_implicit_upstream_status'
WHERE classification_version = 2
  AND error_phase = 'upstream'
  AND COALESCE(upstream_status_code, 0) = 0
  AND (status_code BETWEEN 400 AND 499 OR status_code = 529)
  AND status_code NOT IN (408, 499)
  AND error_category NOT IN ('security_policy', 'client_cancelled');

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_sla_outcome_time
    ON ops_error_logs (counts_toward_sla, final_outcome, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_alert_family_time
    ON ops_error_logs (alert_family, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_category_time
    ON ops_error_logs (error_category, created_at DESC);

-- Replace the exact legacy availability defaults with the classified metrics.
-- These predicates intentionally avoid changing operator-created rules.
UPDATE ops_alert_rules
SET metric_type = 'availability_failure_rate',
	operator = '>=',
    name = '基础设施可用性缓慢下降',
    description = '30 分钟 SLA 合格请求失败率达到 5%，失败至少 10 次且样本至少 100；持续 10 分钟后触发',
    window_minutes = 30,
    sustained_minutes = 10,
    incident_family = 'availability',
    minimum_samples = 100,
    minimum_bad_count = 10,
    recovery_operator = '<',
    recovery_threshold = 2.5,
    recovery_sustained_minutes = 10,
    updated_at = NOW()
WHERE name = '错误率过高' AND metric_type = 'error_rate'
  AND operator = '>' AND threshold = 5.0
  AND window_minutes = 5 AND sustained_minutes = 5
  AND severity = 'P1' AND cooldown_minutes = 20;

UPDATE ops_alert_rules
SET metric_type = 'availability_failure_rate',
	operator = '>=',
    name = '基础设施可用性快速下降',
    description = '5 分钟 SLA 合格请求失败率达到 20%，失败至少 10 次且样本至少 30；持续 3 分钟后触发',
    window_minutes = 5,
    sustained_minutes = 3,
    incident_family = 'availability',
    minimum_samples = 30,
    minimum_bad_count = 10,
    recovery_operator = '<',
    recovery_threshold = 10,
    recovery_sustained_minutes = 5,
    updated_at = NOW()
WHERE name = '错误率极高' AND metric_type = 'error_rate'
  AND operator = '>' AND threshold = 20.0
  AND window_minutes = 1 AND severity = 'P0' AND cooldown_minutes = 15;

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters, created_at, updated_at
)
SELECT
    '未知责任失败率', '未知责任最终失败达到 1%，至少 5 次且样本至少 50；影子运行用于发现分类缺口',
	TRUE, 'P1', 'unknown_failure_rate', '>=', 1,
    15, 5, 30, 'unknown_failure', 50, 5, '<', 0.2, 10, TRUE, TRUE, NULL, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM ops_alert_rules WHERE name = '未知责任失败率' AND metric_type = 'unknown_failure_rate'
);

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters, created_at, updated_at
)
SELECT
    '产品兼容性错误突发', '同一窗口的协议映射或错误状态改写达到 20 次；不进入基础设施 SLA',
	TRUE, 'P2', 'compatibility_error_count', '>=', 20,
    15, 5, 60, 'compatibility', 20, 20, '<', 5, 10, TRUE, FALSE, NULL, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM ops_alert_rules WHERE name = '产品兼容性错误突发' AND metric_type = 'compatibility_error_count'
);

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters, created_at, updated_at
)
SELECT
    '已恢复上游异常增加', '请求最终成功但经历上游限流、5xx 或切换达到 20 次；用于发现隐性供应商退化',
	TRUE, 'P2', 'recovered_provider_error_count', '>=', 20,
    15, 5, 60, 'provider_health', 20, 20, '<', 5, 10, TRUE, FALSE, NULL, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM ops_alert_rules WHERE name = '已恢复上游异常增加' AND metric_type = 'recovered_provider_error_count'
);

INSERT INTO ops_alert_rules (
    name, description, enabled, severity, metric_type, operator, threshold,
    window_minutes, sustained_minutes, cooldown_minutes, incident_family,
    minimum_samples, minimum_bad_count, recovery_operator, recovery_threshold,
    recovery_sustained_minutes, shadow_mode, notify_email, filters, created_at, updated_at
)
SELECT
    '安全策略拒绝突发', '安全或风控策略拒绝达到 100 次；进入安全摘要，不触发基础设施 paging',
	TRUE, 'P3', 'security_blocked_count', '>=', 100,
    15, 5, 60, 'security', 100, 100, '<', 20, 10, TRUE, FALSE, NULL, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM ops_alert_rules WHERE name = '安全策略拒绝突发' AND metric_type = 'security_blocked_count'
);
