-- Correct confirmed provider failures that classification v2 attributed to
-- the client. Raw status codes and error messages remain untouched.
-- sub2api-managed-update: reviewed-compatible
-- A recovered upstream security rejection stays recovered, but must expose the
-- provider/security signal instead of being mistaken for a local block.
UPDATE ops_error_logs
SET
    responsibility = 'provider',
    error_category = 'recovered',
    counts_toward_sla = FALSE,
    alert_family = 'security',
    classification_reason = 'final_request_recovered',
    classification_version = 3
WHERE COALESCE(classification_version, 0) < 3
  AND final_outcome = 'recovered'
  AND (
      COALESCE(upstream_status_code, 0) > 0
      OR error_phase IN ('upstream', 'account_auth')
      OR error_source = 'upstream_http'
      OR error_type = 'upstream_error'
  )
  AND (
      error_type LIKE 'cyber_policy%'
      OR COALESCE(error_message, '') ILIKE '%content policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%content policy%'
      OR COALESCE(error_message, '') ILIKE '%security policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%security policy%'
      OR COALESCE(error_message, '') ILIKE '%cyber policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cyber policy%'
      OR COALESCE(error_message, '') ILIKE '%cyber-security policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cyber-security policy%'
      OR COALESCE(error_message, '') ILIKE '%cybersecurity risk%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cybersecurity risk%'
  );

-- Preserve failed upstream security rejections before generic legacy rules see
-- the reused cyber_policy error type as a local client block.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'security_blocked',
    responsibility = 'provider',
    error_category = 'security_policy',
    counts_toward_sla = FALSE,
    alert_family = 'security',
    classification_reason = 'upstream_security_policy_rejection',
    classification_version = 3
WHERE COALESCE(classification_version, 0) < 3
  AND COALESCE(final_outcome, '') <> 'recovered'
  AND (
      COALESCE(upstream_status_code, 0) > 0
      OR error_phase IN ('upstream', 'account_auth')
      OR error_source = 'upstream_http'
      OR error_type = 'upstream_error'
  )
  AND (
      error_type LIKE 'cyber_policy%'
      OR COALESCE(error_message, '') ILIKE '%content policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%content policy%'
      OR COALESCE(error_message, '') ILIKE '%security policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%security policy%'
      OR COALESCE(error_message, '') ILIKE '%cyber policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cyber policy%'
      OR COALESCE(error_message, '') ILIKE '%cyber-security policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cyber-security policy%'
      OR COALESCE(error_message, '') ILIKE '%cybersecurity risk%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cybersecurity risk%'
  );

-- Managed credentials, capabilities and platform-owned routing capacity must
-- be resolved before generic provider status handling. Previous upstream
-- attempt metadata can remain on a final account-pool or concurrency failure.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'platform_failed',
    responsibility = 'platform',
    error_category = CASE
        WHEN (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE ANY (ARRAY[
                 '%no available account%',
                 '%concurrency limit exceeded for account%',
                 '%too many pending requests%'
             ])
          OR (
              (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%account pool%'
              AND (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%unavailable%'
          )
          OR COALESCE(error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
          OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
            THEN 'platform_capacity'
        WHEN COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
          OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
            THEN 'product_compatibility'
        ELSE 'platform_credential'
    END,
    counts_toward_sla = CASE
        WHEN COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
          OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
            THEN FALSE
        ELSE TRUE
    END,
    alert_family = CASE
        WHEN (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE ANY (ARRAY[
                 '%no available account%',
                 '%concurrency limit exceeded for account%',
                 '%too many pending requests%'
             ])
          OR (
              (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%account pool%'
              AND (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%unavailable%'
          )
          OR COALESCE(error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
          OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
            THEN 'capacity'
        WHEN COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
          OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
            THEN 'compatibility'
        ELSE 'credential'
    END,
    classification_reason = CASE
        WHEN (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE ANY (ARRAY[
                 '%no available account%',
                 '%concurrency limit exceeded for account%',
                 '%too many pending requests%'
             ])
          OR (
              (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%account pool%'
              AND (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%unavailable%'
          )
            THEN 'platform_capacity_unavailable'
        WHEN COALESCE(error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
          OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
            THEN 'managed_upstream_capacity_rejected'
        WHEN COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
          OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
            THEN 'managed_upstream_capability_not_enabled'
        ELSE 'managed_upstream_credential_rejected'
    END,
    classification_version = 3
WHERE COALESCE(classification_version, 0) < 3
  AND COALESCE(status_code, 0) >= 400
  AND COALESCE(final_outcome, '') <> 'recovered'
  AND (
      COALESCE(upstream_status_code, 0) > 0
      OR error_phase IN ('upstream', 'account_auth')
      OR error_source = 'upstream_http'
      OR error_type = 'upstream_error'
  )
  AND (
      error_phase = 'account_auth'
      OR upstream_status_code = 401
      OR (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE ANY (ARRAY[
             '%no available account%',
             '%concurrency limit exceeded for account%',
             '%too many pending requests%'
         ])
      OR (
          (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%account pool%'
          AND (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%unavailable%'
      )
      OR COALESCE(error_message, '') ILIKE '%insufficient%balance%'
      OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
      OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
      OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
      OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
      OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
      OR COALESCE(error_message, '') ILIKE ANY (ARRAY[
             '%not enabled for this group%',
             '%disabled for this group%',
             '%capability is not enabled%',
             '%permission is not enabled%'
         ])
      OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
             '%not enabled for this group%',
             '%disabled for this group%',
             '%capability is not enabled%',
             '%permission is not enabled%'
         ])
      OR (
          upstream_status_code = 403
          AND (
              COALESCE(error_message, '') ILIKE '%access token%'
              OR COALESCE(upstream_error_message, '') ILIKE '%access token%'
              OR COALESCE(error_message, '') ILIKE '%oauth token%'
              OR COALESCE(upstream_error_message, '') ILIKE '%oauth token%'
              OR COALESCE(error_message, '') ILIKE '%credential%'
              OR COALESCE(upstream_error_message, '') ILIKE '%credential%'
              OR COALESCE(error_message, '') ILIKE '%token has been revoked%'
              OR COALESCE(upstream_error_message, '') ILIKE '%token has been revoked%'
              OR COALESCE(error_message, '') ILIKE '%token expired%'
              OR COALESCE(upstream_error_message, '') ILIKE '%token expired%'
              OR COALESCE(error_message, '') ILIKE '%invalid token%'
              OR COALESCE(upstream_error_message, '') ILIKE '%invalid token%'
              OR COALESCE(error_message, '') ILIKE '%authentication failed%'
              OR COALESCE(upstream_error_message, '') ILIKE '%authentication failed%'
              OR COALESCE(error_message, '') ILIKE '%认证失败%'
              OR COALESCE(upstream_error_message, '') ILIKE '%认证失败%'
              OR COALESCE(error_message, '') ILIKE '%请重新登录%'
              OR COALESCE(upstream_error_message, '') ILIKE '%请重新登录%'
              OR COALESCE(error_message, '') ILIKE '%检查 api key%'
              OR COALESCE(upstream_error_message, '') ILIKE '%检查 api key%'
              OR COALESCE(error_message, '') ILIKE '%insufficient%balance%'
              OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
              OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
              OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
              OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
              OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
              OR COALESCE(error_message, '') ILIKE '%not enabled for this group%'
              OR COALESCE(upstream_error_message, '') ILIKE '%not enabled for this group%'
              OR COALESCE(error_message, '') ILIKE '%disabled for this group%'
              OR COALESCE(upstream_error_message, '') ILIKE '%disabled for this group%'
              OR COALESCE(error_message, '') ILIKE '%capability is not enabled%'
              OR COALESCE(upstream_error_message, '') ILIKE '%capability is not enabled%'
              OR COALESCE(error_message, '') ILIKE '%permission is not enabled%'
              OR COALESCE(upstream_error_message, '') ILIKE '%permission is not enabled%'
          )
      )
  );

-- Local model diagnostics and semantic request failures must be handled before
-- the broad provider update. Stale upstream metadata must not turn either into
-- a provider SLA failure.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = CASE
        WHEN COALESCE(error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(upstream_error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(status_code, 0) < 500
            THEN 'client_rejected'
        ELSE 'platform_failed'
    END,
    responsibility = CASE
        WHEN COALESCE(error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(upstream_error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(status_code, 0) < 500
            THEN 'client'
        ELSE 'platform'
    END,
    error_category = CASE
        WHEN COALESCE(error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(upstream_error_message, '') ILIKE '%not supported by any configured account in this group%'
            THEN 'unsupported_model'
        WHEN COALESCE(status_code, 0) < 500
            THEN 'invalid_request'
        ELSE 'product_compatibility'
    END,
    counts_toward_sla = FALSE,
    alert_family = CASE
        WHEN COALESCE(error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(upstream_error_message, '') ILIKE '%not supported by any configured account in this group%'
            THEN 'compatibility'
        WHEN COALESCE(status_code, 0) < 500
            THEN 'client_quality'
        ELSE 'compatibility'
    END,
    classification_reason = CASE
        WHEN COALESCE(error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(upstream_error_message, '') ILIKE '%not supported by any configured account in this group%'
            THEN 'unsupported_or_unconfigured_model'
        WHEN COALESCE(status_code, 0) < 500
            THEN 'upstream_rejected_request_semantics'
        ELSE 'upstream_request_incompatibility_exposed_as_gateway_failure'
    END,
    classification_version = 3
WHERE COALESCE(classification_version, 0) < 3
  AND COALESCE(final_outcome, '') <> 'recovered'
  AND (
      COALESCE(error_message, '') ILIKE '%not supported by any configured account in this group%'
      OR COALESCE(upstream_error_message, '') ILIKE '%not supported by any configured account in this group%'
      OR (
          error_phase = 'upstream'
          AND COALESCE(responsibility, '') IN ('', 'client', 'provider')
          AND COALESCE(status_code, 0) >= 400
          AND (
              (error_type = 'invalid_request_error' AND upstream_status_code IN (400, 422))
              OR COALESCE(error_message, '') ILIKE '%context_length_exceeded%'
              OR COALESCE(upstream_error_message, '') ILIKE '%context_length_exceeded%'
              OR COALESCE(error_message, '') ILIKE '%exceeds the context window%'
              OR COALESCE(upstream_error_message, '') ILIKE '%exceeds the context window%'
              OR COALESCE(error_message, '') ILIKE '%maximum context length%'
              OR COALESCE(upstream_error_message, '') ILIKE '%maximum context length%'
              OR COALESCE(error_message, '') ILIKE '%invalid ''%'
              OR COALESCE(upstream_error_message, '') ILIKE '%invalid ''%'
              OR COALESCE(error_message, '') ILIKE '%invalid request%'
              OR COALESCE(upstream_error_message, '') ILIKE '%invalid request%'
              OR COALESCE(error_message, '') ILIKE '%invalid_request_error%'
              OR COALESCE(upstream_error_message, '') ILIKE '%invalid_request_error%'
              OR COALESCE(error_message, '') ILIKE '%invalid parameter%'
              OR COALESCE(upstream_error_message, '') ILIKE '%invalid parameter%'
              OR COALESCE(error_message, '') ILIKE '%unknown parameter%'
              OR COALESCE(upstream_error_message, '') ILIKE '%unknown parameter%'
              OR COALESCE(error_message, '') ILIKE '%unsupported parameter%'
              OR COALESCE(upstream_error_message, '') ILIKE '%unsupported parameter%'
              OR COALESCE(error_message, '') ILIKE '%malformed request%'
              OR COALESCE(upstream_error_message, '') ILIKE '%malformed request%'
              OR COALESCE(error_message, '') ILIKE '%empty input messages%'
              OR COALESCE(upstream_error_message, '') ILIKE '%empty input messages%'
              OR COALESCE(error_message, '') ILIKE '%user messages must have non-empty content%'
              OR COALESCE(upstream_error_message, '') ILIKE '%user messages must have non-empty content%'
              OR (COALESCE(error_message, '') ILIKE '%invalid `signature`%' AND COALESCE(error_message, '') ILIKE '%`thinking` block%')
              OR (COALESCE(upstream_error_message, '') ILIKE '%invalid `signature`%' AND COALESCE(upstream_error_message, '') ILIKE '%`thinking` block%')
              OR COALESCE(error_message, '') ILIKE '%invalid schema for response_format%'
              OR COALESCE(upstream_error_message, '') ILIKE '%invalid schema for response_format%'
              OR COALESCE(error_message, '') ILIKE '%''required'' must include every key in properties%'
              OR COALESCE(upstream_error_message, '') ILIKE '%''required'' must include every key in properties%'
              OR COALESCE(error_message, '') ILIKE '%`temperature` is deprecated for this model%'
              OR COALESCE(upstream_error_message, '') ILIKE '%`temperature` is deprecated for this model%'
              OR COALESCE(error_message, '') ILIKE '%`temperature` and `top_p` cannot both be specified%'
              OR COALESCE(upstream_error_message, '') ILIKE '%`temperature` and `top_p` cannot both be specified%'
              OR (COALESCE(error_message, '') ILIKE '%cache_control.ttl%' AND COALESCE(error_message, '') ILIKE '%must not come after%')
              OR (COALESCE(upstream_error_message, '') ILIKE '%cache_control.ttl%' AND COALESCE(upstream_error_message, '') ILIKE '%must not come after%')
              OR COALESCE(error_message, '') ILIKE '%does not support the effort parameter%'
              OR COALESCE(upstream_error_message, '') ILIKE '%does not support the effort parameter%'
          )
      )
  );

-- Classification v2 treated HTTP 2xx in-band business errors as recovered.
-- Restore local user quota, subscription and concurrency semantics before the
-- recovered and upstream rules inspect the same rows.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'business_limited',
    responsibility = 'client',
    error_category = CASE
        WHEN error_type = 'subscription_error'
             OR COALESCE(error_message, '') ILIKE '%subscription%'
            THEN 'user_subscription'
        WHEN COALESCE(error_message, '') ILIKE '%concurrency limit exceeded for user%'
            THEN 'user_concurrency'
        ELSE 'user_quota'
    END,
    counts_toward_sla = FALSE,
    alert_family = 'client_quality',
    classification_reason = 'user_plan_quota_or_concurrency_limit',
    classification_version = 3
WHERE classification_version = 2
  AND final_outcome = 'recovered'
  AND COALESCE(status_code, 0) > 0
  AND COALESCE(status_code, 0) < 400
  AND COALESCE(is_business_limited, FALSE)
  AND error_phase IN ('request', 'auth')
  AND COALESCE(upstream_status_code, 0) = 0
  AND COALESCE(error_source, '') <> 'upstream_http'
  AND COALESCE(error_type, '') <> 'upstream_error'
  AND (
      error_type IN ('billing_error', 'subscription_error')
      OR COALESCE(error_message, '') ILIKE '%insufficient balance%'
      OR COALESCE(error_message, '') ILIKE '%quota exhausted%'
      OR COALESCE(error_message, '') ILIKE '%usage limit exceeded%'
      OR COALESCE(error_message, '') ILIKE '%concurrency limit exceeded for user%'
  );

-- Classification v2 treated every HTTP 2xx log as recovered and could also
-- mistake an upstream broken pipe for a client cancellation. Run this before
-- the broad provider update so stale upstream status metadata cannot override
-- explicit transport evidence.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'platform_failed',
    responsibility = 'platform',
    error_category = 'network_transport',
    counts_toward_sla = TRUE,
    alert_family = 'provider_health',
    classification_reason = 'upstream_transport_or_unclassified_failure',
    classification_version = 3
WHERE classification_version = 2
  AND (
      (
          final_outcome = 'recovered'
          AND COALESCE(status_code, 0) > 0
          AND COALESCE(status_code, 0) < 400
          AND error_type = 'upstream_error'
          AND upstream_status_code IS NULL
          AND COALESCE(error_message, '') NOT ILIKE 'Recovered upstream error%'
          AND COALESCE(error_message, '') NOT ILIKE 'Recovered account authentication failure%'
      )
      OR (
          (
              COALESCE(error_message, '') ILIKE '%upstream stream disconnected%'
              OR COALESCE(upstream_error_message, '') ILIKE '%upstream stream disconnected%'
              OR COALESCE(error_message, '') ILIKE '%stream read error%'
              OR COALESCE(upstream_error_message, '') ILIKE '%stream read error%'
              OR COALESCE(error_message, '') ILIKE '%broken pipe%'
              OR COALESCE(upstream_error_message, '') ILIKE '%broken pipe%'
          )
          AND (
              error_phase IN ('upstream', 'network')
              OR error_source = 'upstream_http'
              OR error_type = 'upstream_error'
          )
          AND NOT (
              final_outcome = 'recovered'
              AND (
                  COALESCE(error_message, '') ILIKE 'Recovered upstream error%'
                  OR COALESCE(error_message, '') ILIKE 'Recovered account authentication failure%'
              )
          )
      )
  );

-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'provider_failed',
    responsibility = 'provider',
    error_category = CASE
        WHEN upstream_status_code = 429 THEN 'provider_rate_limit'
        WHEN upstream_status_code = 529 THEN 'provider_overloaded'
        ELSE 'provider_server'
    END,
    counts_toward_sla = TRUE,
    alert_family = 'provider_health',
    classification_reason = CASE
        WHEN upstream_status_code = 429 THEN 'upstream_capacity_or_rate_limit'
        WHEN upstream_status_code = 529 THEN 'upstream_overloaded'
        WHEN upstream_status_code >= 500 THEN 'upstream_server_error'
        ELSE 'upstream_provider_rejected_request'
    END,
    classification_version = 3
WHERE classification_version = 2
  AND error_phase = 'upstream'
  AND responsibility = 'client'
  AND COALESCE(status_code, 0) >= 400
  AND (
      upstream_status_code BETWEEN 400 AND 499
      OR upstream_status_code >= 500
  )
  AND NOT (
      (error_type = 'invalid_request_error' AND upstream_status_code IN (400, 422))
      OR COALESCE(error_message, '') ILIKE '%context_length_exceeded%'
      OR COALESCE(upstream_error_message, '') ILIKE '%context_length_exceeded%'
      OR COALESCE(error_message, '') ILIKE '%exceeds the context window%'
      OR COALESCE(upstream_error_message, '') ILIKE '%exceeds the context window%'
      OR COALESCE(error_message, '') ILIKE '%maximum context length%'
      OR COALESCE(upstream_error_message, '') ILIKE '%maximum context length%'
      OR COALESCE(error_message, '') ILIKE '%invalid ''%'
      OR COALESCE(upstream_error_message, '') ILIKE '%invalid ''%'
      OR COALESCE(error_message, '') ILIKE '%invalid request%'
      OR COALESCE(upstream_error_message, '') ILIKE '%invalid request%'
      OR COALESCE(error_message, '') ILIKE '%invalid_request_error%'
      OR COALESCE(upstream_error_message, '') ILIKE '%invalid_request_error%'
      OR COALESCE(error_message, '') ILIKE '%invalid parameter%'
      OR COALESCE(upstream_error_message, '') ILIKE '%invalid parameter%'
      OR COALESCE(error_message, '') ILIKE '%unknown parameter%'
      OR COALESCE(upstream_error_message, '') ILIKE '%unknown parameter%'
      OR COALESCE(error_message, '') ILIKE '%unsupported parameter%'
      OR COALESCE(upstream_error_message, '') ILIKE '%unsupported parameter%'
      OR COALESCE(error_message, '') ILIKE '%malformed request%'
      OR COALESCE(upstream_error_message, '') ILIKE '%malformed request%'
      OR COALESCE(error_message, '') ILIKE '%empty input messages%'
      OR COALESCE(upstream_error_message, '') ILIKE '%empty input messages%'
      OR COALESCE(error_message, '') ILIKE '%user messages must have non-empty content%'
      OR COALESCE(upstream_error_message, '') ILIKE '%user messages must have non-empty content%'
      OR (COALESCE(error_message, '') ILIKE '%invalid `signature`%' AND COALESCE(error_message, '') ILIKE '%`thinking` block%')
      OR (COALESCE(upstream_error_message, '') ILIKE '%invalid `signature`%' AND COALESCE(upstream_error_message, '') ILIKE '%`thinking` block%')
      OR COALESCE(error_message, '') ILIKE '%invalid schema for response_format%'
      OR COALESCE(upstream_error_message, '') ILIKE '%invalid schema for response_format%'
      OR COALESCE(error_message, '') ILIKE '%''required'' must include every key in properties%'
      OR COALESCE(upstream_error_message, '') ILIKE '%''required'' must include every key in properties%'
      OR COALESCE(error_message, '') ILIKE '%`temperature` is deprecated for this model%'
      OR COALESCE(upstream_error_message, '') ILIKE '%`temperature` is deprecated for this model%'
      OR COALESCE(error_message, '') ILIKE '%`temperature` and `top_p` cannot both be specified%'
      OR COALESCE(upstream_error_message, '') ILIKE '%`temperature` and `top_p` cannot both be specified%'
      OR (COALESCE(error_message, '') ILIKE '%cache_control.ttl%' AND COALESCE(error_message, '') ILIKE '%must not come after%')
      OR (COALESCE(upstream_error_message, '') ILIKE '%cache_control.ttl%' AND COALESCE(upstream_error_message, '') ILIKE '%must not come after%')
      OR COALESCE(error_message, '') ILIKE '%does not support the effort parameter%'
      OR COALESCE(upstream_error_message, '') ILIKE '%does not support the effort parameter%'
      OR COALESCE(error_message, '') ILIKE '%content policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%content policy%'
      OR COALESCE(error_message, '') ILIKE '%security policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%security policy%'
      OR COALESCE(error_message, '') ILIKE '%cyber policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cyber policy%'
      OR COALESCE(error_message, '') ILIKE '%cyber-security policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cyber-security policy%'
      OR COALESCE(error_message, '') ILIKE '%cybersecurity risk%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cybersecurity risk%'
  );

-- A provider 400/422 rewritten by the gateway to 5xx is a compatibility
-- failure in the platform, not a client rejection.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'platform_failed',
    responsibility = 'platform',
    error_category = 'product_compatibility',
    counts_toward_sla = FALSE,
    alert_family = 'compatibility',
    classification_reason = 'upstream_request_incompatibility_exposed_as_gateway_failure',
    classification_version = 3
WHERE classification_version = 2
  AND error_phase = 'upstream'
  AND responsibility = 'client'
  AND status_code >= 500
  AND upstream_status_code IN (400, 422)
  AND (
      error_type = 'invalid_request_error'
      OR COALESCE(upstream_error_message, '') ILIKE '%invalid_request_error%'
      OR COALESCE(upstream_error_message, '') ILIKE '%invalid request%'
      OR COALESCE(error_message, '') ILIKE '%invalid_request_error%'
      OR COALESCE(error_message, '') ILIKE '%invalid request%'
  );

-- If the final request succeeded, a failed upstream attempt cannot be a
-- client semantic error. Keep it excluded from SLA while preserving provider
-- health or compatibility visibility.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    responsibility = CASE
        WHEN COALESCE(error_message, '') ILIKE '%context canceled%'
          OR COALESCE(upstream_error_message, '') ILIKE '%context canceled%'
          OR COALESCE(error_message, '') ILIKE '%client disconnected%'
          OR COALESCE(upstream_error_message, '') ILIKE '%client disconnected%'
            THEN 'client'
        WHEN COALESCE(error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(upstream_error_message, '') ILIKE '%not supported by any configured account in this group%'
            THEN 'client'
        WHEN (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE ANY (ARRAY[
                 '%no available account%',
                 '%concurrency limit exceeded for account%',
                 '%too many pending requests%'
             ])
          OR (
              (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%account pool%'
              AND (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%unavailable%'
          )
          OR COALESCE(error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
          OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
            THEN 'platform'
        WHEN COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
          OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
            THEN 'platform'
        WHEN upstream_status_code = 401
          OR error_phase = 'account_auth'
          OR (
              upstream_status_code = 403
              AND (
                  COALESCE(error_message, '') ILIKE ANY (ARRAY[
                      '%access token%', '%oauth token%', '%credential%',
                      '%token has been revoked%', '%token expired%', '%invalid token%',
                      '%authentication failed%', '%认证失败%', '%请重新登录%', '%检查 api key%'
                  ])
                  OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                      '%access token%', '%oauth token%', '%credential%',
                      '%token has been revoked%', '%token expired%', '%invalid token%',
                      '%authentication failed%', '%认证失败%', '%请重新登录%', '%检查 api key%'
                  ])
              )
          )
            THEN 'platform'
        WHEN (error_type = 'invalid_request_error' AND upstream_status_code IN (400, 422))
          OR COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%context_length_exceeded%', '%exceeds the context window%',
                 '%maximum context length%', '%invalid ''%', '%invalid request%',
                 '%invalid_request_error%', '%invalid parameter%', '%unknown parameter%',
                 '%unsupported parameter%', '%malformed request%', '%empty input messages%',
                 '%user messages must have non-empty content%', '%invalid schema for response_format%',
                 '%''required'' must include every key in properties%',
                 '%`temperature` is deprecated for this model%',
                 '%`temperature` and `top_p` cannot both be specified%',
                 '%does not support the effort parameter%'
             ])
          OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%context_length_exceeded%', '%exceeds the context window%',
                 '%maximum context length%', '%invalid ''%', '%invalid request%',
                 '%invalid_request_error%', '%invalid parameter%', '%unknown parameter%',
                 '%unsupported parameter%', '%malformed request%', '%empty input messages%',
                 '%user messages must have non-empty content%', '%invalid schema for response_format%',
                 '%''required'' must include every key in properties%',
                 '%`temperature` is deprecated for this model%',
                 '%`temperature` and `top_p` cannot both be specified%',
                 '%does not support the effort parameter%'
             ])
          OR (COALESCE(error_message, '') ILIKE '%invalid `signature`%' AND COALESCE(error_message, '') ILIKE '%`thinking` block%')
          OR (COALESCE(upstream_error_message, '') ILIKE '%invalid `signature`%' AND COALESCE(upstream_error_message, '') ILIKE '%`thinking` block%')
          OR (COALESCE(error_message, '') ILIKE '%cache_control.ttl%' AND COALESCE(error_message, '') ILIKE '%must not come after%')
          OR (COALESCE(upstream_error_message, '') ILIKE '%cache_control.ttl%' AND COALESCE(upstream_error_message, '') ILIKE '%must not come after%')
            THEN 'platform'
        ELSE 'provider'
    END,
    error_category = 'recovered',
    counts_toward_sla = FALSE,
    alert_family = CASE
        WHEN COALESCE(error_message, '') ILIKE '%context canceled%'
          OR COALESCE(upstream_error_message, '') ILIKE '%context canceled%'
          OR COALESCE(error_message, '') ILIKE '%client disconnected%'
          OR COALESCE(upstream_error_message, '') ILIKE '%client disconnected%'
            THEN 'client_quality'
        WHEN COALESCE(error_message, '') ILIKE '%not supported by any configured account in this group%'
          OR COALESCE(upstream_error_message, '') ILIKE '%not supported by any configured account in this group%'
            THEN 'compatibility'
        WHEN (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE ANY (ARRAY[
                 '%no available account%',
                 '%concurrency limit exceeded for account%',
                 '%too many pending requests%'
             ])
          OR (
              (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%account pool%'
              AND (COALESCE(error_message, '') || ' ' || COALESCE(upstream_error_message, '')) ILIKE '%unavailable%'
          )
          OR COALESCE(error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
          OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
          OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
          OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
            THEN 'capacity'
        WHEN COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
          OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
            THEN 'compatibility'
        WHEN upstream_status_code = 401
          OR error_phase = 'account_auth'
          OR (
              upstream_status_code = 403
              AND (
                  COALESCE(error_message, '') ILIKE ANY (ARRAY[
                      '%access token%', '%oauth token%', '%credential%',
                      '%token has been revoked%', '%token expired%', '%invalid token%',
                      '%authentication failed%', '%认证失败%', '%请重新登录%', '%检查 api key%'
                  ])
                  OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                      '%access token%', '%oauth token%', '%credential%',
                      '%token has been revoked%', '%token expired%', '%invalid token%',
                      '%authentication failed%', '%认证失败%', '%请重新登录%', '%检查 api key%'
                  ])
              )
          )
            THEN 'credential'
        WHEN (error_type = 'invalid_request_error' AND upstream_status_code IN (400, 422))
          OR COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%context_length_exceeded%', '%exceeds the context window%',
                 '%maximum context length%', '%invalid ''%', '%invalid request%',
                 '%invalid_request_error%', '%invalid parameter%', '%unknown parameter%',
                 '%unsupported parameter%', '%malformed request%', '%model%not supported%',
                 '%model%not in whitelist%', '%model%not configured%', '%invalid%',
                 '%empty input messages%', '%user messages must have non-empty content%',
                 '%''required'' must include every key in properties%',
                 '%`temperature` is deprecated for this model%',
                 '%`temperature` and `top_p` cannot both be specified%',
                 '%does not support the effort parameter%'
             ])
          OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%context_length_exceeded%', '%exceeds the context window%',
                 '%maximum context length%', '%invalid ''%', '%invalid request%',
                 '%invalid_request_error%', '%invalid parameter%', '%unknown parameter%',
                 '%unsupported parameter%', '%malformed request%', '%model%not supported%',
                 '%model%not in whitelist%', '%model%not configured%', '%invalid%',
                 '%empty input messages%', '%user messages must have non-empty content%',
                 '%''required'' must include every key in properties%',
                 '%`temperature` is deprecated for this model%',
                 '%`temperature` and `top_p` cannot both be specified%',
                 '%does not support the effort parameter%'
             ])
          OR (COALESCE(error_message, '') ILIKE '%cache_control.ttl%' AND COALESCE(error_message, '') ILIKE '%must not come after%')
          OR (COALESCE(upstream_error_message, '') ILIKE '%cache_control.ttl%' AND COALESCE(upstream_error_message, '') ILIKE '%must not come after%')
            THEN 'compatibility'
        ELSE 'provider_health'
    END,
    classification_reason = 'final_request_recovered',
    classification_version = 3
WHERE classification_version = 2
  AND final_outcome = 'recovered'
  AND COALESCE(status_code, 0) < 400
  AND (
      (
          error_phase = 'upstream'
          AND (
              upstream_status_code >= 400
              OR COALESCE(error_message, '') ILIKE 'Recovered upstream error%'
          )
      )
      OR (
          error_phase = 'account_auth'
          AND (
              upstream_status_code >= 400
              OR COALESCE(error_message, '') ILIKE 'Recovered account authentication failure%'
          )
      )
  );

-- Legacy callers marked some routing failures as business-limited. A gateway
-- 5xx is still a platform availability failure regardless of that broad flag.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'platform_failed',
    responsibility = 'platform',
    error_category = 'platform_internal',
    counts_toward_sla = TRUE,
    alert_family = 'availability',
    classification_reason = 'platform_internal_or_routing_failure',
    classification_version = 3
WHERE classification_version = 2
  AND error_phase IN ('routing', 'internal')
  AND error_owner = 'platform'
  AND COALESCE(status_code, 0) >= 500
  AND final_outcome = 'client_rejected'
  AND responsibility = 'client';

-- Some older logger paths retained phase=request/source=client_request even
-- though error_type explicitly said upstream_error. Preserve that evidence.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'platform_failed',
    responsibility = 'platform',
    error_category = 'network_transport',
    counts_toward_sla = TRUE,
    alert_family = 'provider_health',
    classification_reason = 'upstream_transport_or_unclassified_failure',
    classification_version = 3
WHERE classification_version = 2
  AND error_type = 'upstream_error'
  AND COALESCE(status_code, 0) >= 500
  AND responsibility = 'client'
  AND (
      error_phase = 'request'
      OR error_source = 'client_request'
  )
  AND NOT (
      error_message ILIKE '%upstream%403%insufficient%balance%'
      OR error_message ILIKE '%upstream%401%insufficient%balance%'
  );

-- Some Codex manifest failures did not persist upstream_status_code, but the
-- sanitized message still contains an explicit upstream 401/403 balance error.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'platform_failed',
    responsibility = 'platform',
    error_category = 'platform_capacity',
    counts_toward_sla = TRUE,
    alert_family = 'capacity',
    classification_reason = 'managed_upstream_capacity_rejected',
    classification_version = 3
WHERE classification_version = 2
  AND error_type = 'upstream_error'
  AND COALESCE(status_code, 0) >= 500
  AND responsibility = 'client'
  AND (
      error_message ILIKE '%upstream%403%insufficient%balance%'
      OR error_message ILIKE '%upstream%401%insufficient%balance%'
  );

-- Upstream safety-policy rejections must keep provider ownership. Do not let
-- message text alone turn an upstream rejection into a local client block.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'security_blocked',
    responsibility = 'provider',
    error_category = 'security_policy',
    counts_toward_sla = FALSE,
    alert_family = 'security',
    classification_reason = 'upstream_security_policy_rejection',
    classification_version = 3
WHERE COALESCE(classification_version, 0) < 3
  AND error_phase = 'upstream'
  AND COALESCE(final_outcome, '') <> 'recovered'
  AND (
      COALESCE(error_message, '') ILIKE '%content policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%content policy%'
      OR COALESCE(error_message, '') ILIKE '%security policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%security policy%'
      OR COALESCE(error_message, '') ILIKE '%cyber policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cyber policy%'
      OR COALESCE(error_message, '') ILIKE '%cyber-security policy%'
      OR COALESCE(upstream_error_message, '') ILIKE '%cyber-security policy%'
  );

-- Session-level cyber policy blocks use a more specific error type than the
-- original classifier recognized, but remain client security rejections.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    final_outcome = 'security_blocked',
    responsibility = 'client',
    error_category = 'security_policy',
    counts_toward_sla = FALSE,
    alert_family = 'security',
    classification_reason = 'security_policy_rejection',
    classification_version = 3
WHERE classification_version = 2
  AND error_type LIKE 'cyber_policy%'
  AND final_outcome IN ('client_rejected', 'recovered')
  AND COALESCE(upstream_status_code, 0) = 0
  AND error_phase NOT IN ('upstream', 'account_auth')
  AND COALESCE(error_source, '') <> 'upstream_http'
  AND COALESCE(error_type, '') <> 'upstream_error';

-- A recovered 401/403 came from a rejected managed upstream credential. The
-- request recovered, but the operational signal belongs to managed credentials.
-- sub2api-managed-update: reviewed-compatible
UPDATE ops_error_logs
SET
    responsibility = CASE
        WHEN error_phase = 'account_auth'
             OR upstream_status_code = 401
             OR COALESCE(error_message, '') ILIKE ANY (ARRAY[
                    '%access token%',
                    '%oauth token%',
                    '%credential%',
                    '%token has been revoked%',
                    '%token expired%',
                    '%invalid token%',
                    '%authentication failed%'
                ])
             OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                    '%access token%',
                    '%oauth token%',
                    '%credential%',
                    '%token has been revoked%',
                    '%token expired%',
                    '%invalid token%',
                    '%authentication failed%'
                ])
             OR COALESCE(error_message, '') ILIKE '%认证失败%'
             OR COALESCE(upstream_error_message, '') ILIKE '%认证失败%'
             OR COALESCE(error_message, '') ILIKE '%请重新登录%'
             OR COALESCE(upstream_error_message, '') ILIKE '%请重新登录%'
             OR COALESCE(error_message, '') ILIKE '%检查 api key%'
             OR COALESCE(upstream_error_message, '') ILIKE '%检查 api key%'
            THEN 'platform'
        WHEN COALESCE(error_message, '') ILIKE '%insufficient%balance%'
             OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
             OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
             OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
             OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
             OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
            THEN 'platform'
        WHEN COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
             OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
            THEN 'platform'
        ELSE 'provider'
    END,
    alert_family = CASE
        WHEN COALESCE(error_message, '') ILIKE '%insufficient%balance%'
             OR COALESCE(upstream_error_message, '') ILIKE '%insufficient%balance%'
             OR COALESCE(error_message, '') ILIKE '%预扣费额度失败%'
             OR COALESCE(upstream_error_message, '') ILIKE '%预扣费额度失败%'
             OR COALESCE(error_message, '') ILIKE '%用户剩余额度%'
             OR COALESCE(upstream_error_message, '') ILIKE '%用户剩余额度%'
            THEN 'capacity'
        WHEN COALESCE(error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
             OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                 '%not enabled for this group%',
                 '%disabled for this group%',
                 '%capability is not enabled%',
                 '%permission is not enabled%'
             ])
            THEN 'compatibility'
        WHEN error_phase = 'account_auth'
             OR upstream_status_code = 401
             OR COALESCE(error_message, '') ILIKE ANY (ARRAY[
                    '%access token%',
                    '%oauth token%',
                    '%credential%',
                    '%token has been revoked%',
                    '%token expired%',
                    '%invalid token%',
                    '%authentication failed%'
                ])
             OR COALESCE(upstream_error_message, '') ILIKE ANY (ARRAY[
                    '%access token%',
                    '%oauth token%',
                    '%credential%',
                    '%token has been revoked%',
                    '%token expired%',
                    '%invalid token%',
                    '%authentication failed%'
                ])
             OR COALESCE(error_message, '') ILIKE '%认证失败%'
             OR COALESCE(upstream_error_message, '') ILIKE '%认证失败%'
             OR COALESCE(error_message, '') ILIKE '%请重新登录%'
             OR COALESCE(upstream_error_message, '') ILIKE '%请重新登录%'
             OR COALESCE(error_message, '') ILIKE '%检查 api key%'
             OR COALESCE(upstream_error_message, '') ILIKE '%检查 api key%'
            THEN 'credential'
        ELSE 'provider_health'
    END,
    classification_version = 3
WHERE classification_version = 2
  AND final_outcome = 'recovered'
  AND (
      error_phase = 'account_auth'
      OR upstream_status_code IN (401, 403)
  );
