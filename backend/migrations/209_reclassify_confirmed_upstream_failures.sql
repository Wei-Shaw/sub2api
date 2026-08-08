-- Keep writes from the draining v2 image aligned after the one-time backfill.
-- The trigger only touches rows whose writer explicitly uses an older
-- classification version; v3 writers return immediately.
-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION public.normalize_ops_error_log_v3_mixed_writer()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    message_text TEXT := LOWER(
        COALESCE(NEW.error_message, '') || ' ' ||
        COALESCE(NEW.upstream_error_message, '')
    );
    upstream_evidence BOOLEAN :=
        COALESCE(NEW.upstream_status_code, 0) > 0
        OR NEW.error_phase IN ('upstream', 'account_auth')
        OR NEW.error_source = 'upstream_http'
        OR NEW.error_owner = 'provider'
        OR message_text LIKE '%upstream stream disconnected%'
        OR message_text LIKE '%stream read error%'
        OR NEW.error_type = 'upstream_error';
    security_message BOOLEAN;
    platform_capacity_message BOOLEAN;
    capacity_message BOOLEAN;
    capability_message BOOLEAN;
    credential_message BOOLEAN;
    request_semantics BOOLEAN;
    recovered_compatibility BOOLEAN;
    user_business_limit BOOLEAN;
    local_group_model_unavailable BOOLEAN;
    effective_upstream_status INTEGER := COALESCE(NEW.upstream_status_code, 0);
BEGIN
    IF COALESCE(NEW.classification_version, 0) >= 3 THEN
        RETURN NEW;
    END IF;

    security_message :=
        message_text LIKE '%content policy%'
        OR message_text LIKE '%security policy%'
        OR message_text LIKE '%cyber policy%'
        OR message_text LIKE '%cyber-security policy%'
        OR message_text LIKE '%cybersecurity risk%';
    platform_capacity_message :=
        message_text LIKE '%no available account%'
        OR message_text LIKE '%concurrency limit exceeded for account%'
        OR message_text LIKE '%too many pending requests%'
        OR (message_text LIKE '%account pool%' AND message_text LIKE '%unavailable%');
    capacity_message :=
        message_text LIKE '%insufficient balance%'
        OR message_text LIKE '%insufficient_account_balance%'
        OR message_text LIKE '%insufficient_balance%'
        OR message_text LIKE '%预扣费额度失败%'
        OR message_text LIKE '%用户剩余额度%';
    capability_message :=
        message_text LIKE '%not enabled for this group%'
        OR message_text LIKE '%disabled for this group%'
        OR message_text LIKE '%capability is not enabled%'
        OR message_text LIKE '%permission is not enabled%';
    credential_message :=
        message_text LIKE '%access token%'
        OR message_text LIKE '%oauth token%'
        OR message_text LIKE '%credential%'
        OR message_text LIKE '%token has been revoked%'
        OR message_text LIKE '%token expired%'
        OR message_text LIKE '%invalid token%'
        OR message_text LIKE '%authentication failed%'
        OR message_text LIKE '%认证失败%'
        OR message_text LIKE '%请重新登录%'
        OR message_text LIKE '%检查 api key%';
    request_semantics :=
        (
            NEW.error_type = 'invalid_request_error'
            AND NEW.upstream_status_code IN (400, 422)
        )
        OR message_text LIKE '%context_length_exceeded%'
        OR message_text LIKE '%exceeds the context window%'
        OR message_text LIKE '%maximum context length%'
        OR message_text LIKE '%invalid ''%'
        OR message_text LIKE '%invalid request%'
        OR message_text LIKE '%invalid_request_error%'
        OR message_text LIKE '%invalid parameter%'
        OR message_text LIKE '%unknown parameter%'
        OR message_text LIKE '%unsupported parameter%'
        OR message_text LIKE '%malformed request%'
        OR message_text LIKE '%empty input messages%'
        OR message_text LIKE '%user messages must have non-empty content%'
        OR (message_text LIKE '%invalid `signature`%' AND message_text LIKE '%`thinking` block%')
        OR message_text LIKE '%invalid schema for response_format%'
        OR message_text LIKE '%''required'' must include every key in properties%'
        OR message_text LIKE '%`temperature` is deprecated for this model%'
        OR message_text LIKE '%`temperature` and `top_p` cannot both be specified%'
        OR (message_text LIKE '%cache_control.ttl%' AND message_text LIKE '%must not come after%')
        OR message_text LIKE '%does not support the effort parameter%';
    recovered_compatibility :=
        request_semantics
        OR message_text LIKE '%invalid%'
        OR (
            message_text LIKE '%model%'
            AND (
                message_text LIKE '%not supported%'
                OR message_text LIKE '%not in whitelist%'
                OR message_text LIKE '%not configured%'
            )
        );
    user_business_limit :=
        COALESCE(NEW.is_business_limited, FALSE)
        AND NOT upstream_evidence
        AND NEW.error_phase IN ('request', 'auth')
        AND (
            NEW.error_type IN ('billing_error', 'subscription_error')
            OR message_text LIKE '%insufficient balance%'
            OR message_text LIKE '%quota exhausted%'
            OR message_text LIKE '%usage limit exceeded%'
            OR message_text LIKE '%concurrency limit exceeded for user%'
        );
    local_group_model_unavailable :=
        message_text LIKE '%not supported by any configured account in this group%';

    IF NOT upstream_evidence AND NEW.error_type LIKE 'cyber_policy%' THEN
        NEW.final_outcome := 'security_blocked';
        NEW.responsibility := 'client';
        NEW.error_category := 'security_policy';
        NEW.counts_toward_sla := FALSE;
        NEW.alert_family := 'security';
        NEW.classification_reason := 'security_policy_rejection';
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF user_business_limit THEN
        NEW.final_outcome := 'business_limited';
        NEW.responsibility := 'client';
        NEW.error_category := CASE
            WHEN NEW.error_type = 'subscription_error' OR message_text LIKE '%subscription%'
                THEN 'user_subscription'
            WHEN message_text LIKE '%concurrency limit exceeded for user%'
                THEN 'user_concurrency'
            ELSE 'user_quota'
        END;
        NEW.counts_toward_sla := FALSE;
        NEW.alert_family := 'client_quality';
        NEW.classification_reason := 'user_plan_quota_or_concurrency_limit';
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF NEW.final_outcome = 'recovered'
       AND NOT (
           COALESCE(NEW.status_code, 0) > 0
           AND COALESCE(NEW.status_code, 0) < 400
           AND NEW.error_type = 'upstream_error'
           AND NEW.upstream_status_code IS NULL
           AND COALESCE(NEW.error_message, '') NOT ILIKE 'Recovered upstream error%'
           AND COALESCE(NEW.error_message, '') NOT ILIKE 'Recovered account authentication failure%'
       ) THEN
        NEW.error_category := 'recovered';
        NEW.counts_toward_sla := FALSE;
        NEW.classification_reason := 'final_request_recovered';
        IF message_text LIKE '%context canceled%'
           OR message_text LIKE '%client disconnected%' THEN
            NEW.responsibility := 'client';
            NEW.alert_family := 'client_quality';
        ELSIF local_group_model_unavailable THEN
            NEW.responsibility := 'client';
            NEW.alert_family := 'compatibility';
        ELSIF security_message AND upstream_evidence THEN
            NEW.responsibility := 'provider';
            NEW.alert_family := 'security';
        ELSIF platform_capacity_message OR capacity_message THEN
            NEW.responsibility := 'platform';
            NEW.alert_family := 'capacity';
        ELSIF capability_message THEN
            NEW.responsibility := 'platform';
            NEW.alert_family := 'compatibility';
        ELSIF NEW.error_phase = 'account_auth'
              OR NEW.upstream_status_code = 401
              OR (NEW.upstream_status_code = 403 AND credential_message) THEN
            NEW.responsibility := 'platform';
            NEW.alert_family := 'credential';
        ELSIF request_semantics THEN
            NEW.responsibility := 'platform';
            NEW.alert_family := 'compatibility';
        ELSIF recovered_compatibility THEN
            NEW.responsibility := 'provider';
            NEW.alert_family := 'compatibility';
        ELSIF COALESCE(NEW.upstream_status_code, 0) >= 400
              OR NEW.error_phase IN ('upstream', 'account_auth') THEN
            NEW.responsibility := 'provider';
            NEW.alert_family := 'provider_health';
        ELSE
            NEW.responsibility := COALESCE(NULLIF(NEW.responsibility, ''), 'unknown');
            NEW.alert_family := 'none';
        END IF;
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF local_group_model_unavailable THEN
        NEW.final_outcome := 'client_rejected';
        NEW.responsibility := 'client';
        NEW.error_category := 'unsupported_model';
        NEW.counts_toward_sla := FALSE;
        NEW.alert_family := 'compatibility';
        NEW.classification_reason := 'unsupported_or_unconfigured_model';
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF platform_capacity_message THEN
        NEW.final_outcome := 'platform_failed';
        NEW.responsibility := 'platform';
        NEW.error_category := 'platform_capacity';
        NEW.counts_toward_sla := TRUE;
        NEW.alert_family := 'capacity';
        NEW.classification_reason := 'platform_capacity_unavailable';
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF COALESCE(NEW.status_code, 0) >= 500
       AND (
           NEW.error_phase IN ('routing', 'internal')
           OR (NEW.error_owner = 'platform' AND NEW.error_source = 'gateway')
       ) THEN
        NEW.final_outcome := 'platform_failed';
        NEW.responsibility := 'platform';
        NEW.error_category := 'platform_internal';
        NEW.counts_toward_sla := TRUE;
        NEW.alert_family := 'availability';
        NEW.classification_reason := 'platform_internal_or_routing_failure';
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF COALESCE(NEW.upstream_status_code, 0) = 0
       AND message_text NOT LIKE '%upstream stream disconnected%'
       AND message_text NOT LIKE '%stream read error%'
       AND NOT (upstream_evidence AND message_text LIKE '%broken pipe%')
       AND (
           NEW.status_code IN (408, 499)
           OR message_text LIKE '%context canceled%'
           OR message_text LIKE '%client disconnected%'
           OR message_text LIKE '%client closed%'
           OR message_text LIKE '%request canceled%'
       ) THEN
        NEW.final_outcome := 'cancelled';
        NEW.responsibility := 'client';
        NEW.error_category := 'client_cancelled';
        NEW.counts_toward_sla := FALSE;
        NEW.alert_family := 'client_quality';
        NEW.classification_reason := 'client_cancel_or_disconnect';
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF NOT upstream_evidence AND security_message THEN
        NEW.final_outcome := 'security_blocked';
        NEW.responsibility := 'client';
        NEW.error_category := 'security_policy';
        NEW.counts_toward_sla := FALSE;
        NEW.alert_family := 'security';
        NEW.classification_reason := 'security_policy_rejection';
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF upstream_evidence THEN
        IF security_message THEN
            NEW.final_outcome := 'security_blocked';
            NEW.responsibility := 'provider';
            NEW.error_category := 'security_policy';
            NEW.counts_toward_sla := FALSE;
            NEW.alert_family := 'security';
            NEW.classification_reason := 'upstream_security_policy_rejection';
        ELSIF capacity_message THEN
            NEW.final_outcome := 'platform_failed';
            NEW.responsibility := 'platform';
            NEW.error_category := 'platform_capacity';
            NEW.counts_toward_sla := TRUE;
            NEW.alert_family := 'capacity';
            NEW.classification_reason := 'managed_upstream_capacity_rejected';
        ELSIF capability_message THEN
            NEW.final_outcome := 'platform_failed';
            NEW.responsibility := 'platform';
            NEW.error_category := 'product_compatibility';
            NEW.counts_toward_sla := FALSE;
            NEW.alert_family := 'compatibility';
            NEW.classification_reason := 'managed_upstream_capability_not_enabled';
        ELSIF NEW.error_phase = 'account_auth'
              OR NEW.upstream_status_code = 401
              OR (NEW.upstream_status_code = 403 AND credential_message) THEN
            NEW.final_outcome := 'platform_failed';
            NEW.responsibility := 'platform';
            NEW.error_category := 'platform_credential';
            NEW.counts_toward_sla := TRUE;
            NEW.alert_family := 'credential';
            NEW.classification_reason := 'managed_upstream_credential_rejected';
        ELSIF request_semantics THEN
            NEW.final_outcome := CASE
                WHEN COALESCE(NEW.status_code, 0) >= 500 OR COALESCE(NEW.status_code, 0) = 0
                    THEN 'platform_failed'
                ELSE 'client_rejected'
            END;
            NEW.responsibility := CASE
                WHEN COALESCE(NEW.status_code, 0) >= 500 OR COALESCE(NEW.status_code, 0) = 0
                    THEN 'platform'
                ELSE 'client'
            END;
            NEW.error_category := CASE
                WHEN COALESCE(NEW.status_code, 0) >= 500 OR COALESCE(NEW.status_code, 0) = 0
                    THEN 'product_compatibility'
                ELSE 'invalid_request'
            END;
            NEW.counts_toward_sla := FALSE;
            NEW.alert_family := CASE
                WHEN COALESCE(NEW.status_code, 0) >= 500 OR COALESCE(NEW.status_code, 0) = 0
                    THEN 'compatibility'
                ELSE 'client_quality'
            END;
            NEW.classification_reason := CASE
                WHEN COALESCE(NEW.status_code, 0) >= 500 OR COALESCE(NEW.status_code, 0) = 0
                    THEN 'upstream_request_incompatibility_exposed_as_gateway_failure'
                ELSE 'upstream_rejected_request_semantics'
            END;
        ELSE
            IF effective_upstream_status = 0
               AND message_text NOT LIKE '%upstream stream disconnected%'
               AND message_text NOT LIKE '%stream read error%'
               AND message_text NOT LIKE '%broken pipe%'
               AND ((NEW.status_code >= 400 AND NEW.status_code < 500) OR NEW.status_code = 529) THEN
                effective_upstream_status := NEW.status_code;
            END IF;
            IF effective_upstream_status >= 400 AND effective_upstream_status < 500
               AND effective_upstream_status <> 429 THEN
                NEW.final_outcome := 'provider_failed';
                NEW.responsibility := 'provider';
                NEW.error_category := 'provider_server';
                NEW.counts_toward_sla := TRUE;
                NEW.alert_family := 'provider_health';
                NEW.classification_reason := 'upstream_provider_rejected_request';
            ELSIF effective_upstream_status = 429 THEN
                NEW.final_outcome := 'provider_failed';
                NEW.responsibility := 'provider';
                NEW.error_category := 'provider_rate_limit';
                NEW.counts_toward_sla := TRUE;
                NEW.alert_family := 'provider_health';
                NEW.classification_reason := 'upstream_capacity_or_rate_limit';
            ELSIF effective_upstream_status = 529
                  OR message_text LIKE '%overloaded%'
                  OR NEW.error_type = 'overloaded_error' THEN
                NEW.final_outcome := 'provider_failed';
                NEW.responsibility := 'provider';
                NEW.error_category := 'provider_overloaded';
                NEW.counts_toward_sla := TRUE;
                NEW.alert_family := 'provider_health';
                NEW.classification_reason := 'upstream_overloaded';
            ELSIF effective_upstream_status >= 500 THEN
                NEW.final_outcome := 'provider_failed';
                NEW.responsibility := 'provider';
                NEW.error_category := 'provider_server';
                NEW.counts_toward_sla := TRUE;
                NEW.alert_family := 'provider_health';
                NEW.classification_reason := 'upstream_server_error';
            ELSE
                NEW.final_outcome := 'platform_failed';
                NEW.responsibility := 'platform';
                NEW.error_category := 'network_transport';
                NEW.counts_toward_sla := TRUE;
                NEW.alert_family := 'provider_health';
                NEW.classification_reason := 'upstream_transport_or_unclassified_failure';
            END IF;
        END IF;
        NEW.classification_version := 3;
        RETURN NEW;
    END IF;

    IF COALESCE(NEW.status_code, 0) >= 500
       AND NEW.error_phase IN ('routing', 'internal') THEN
        NEW.final_outcome := 'platform_failed';
        NEW.responsibility := 'platform';
        NEW.error_category := 'platform_internal';
        NEW.counts_toward_sla := TRUE;
        NEW.alert_family := 'availability';
        NEW.classification_reason := 'platform_internal_or_routing_failure';
        NEW.classification_version := 3;
    END IF;
    RETURN NEW;
END;
$$;

-- sub2api-managed-update: reviewed-compatible
CREATE TRIGGER ops_error_logs_normalize_v3_mixed_writer
BEFORE INSERT ON ops_error_logs
FOR EACH ROW
WHEN (COALESCE(NEW.classification_version, 0) < 3)
EXECUTE FUNCTION public.normalize_ops_error_log_v3_mixed_writer();
