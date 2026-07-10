-- Phase 3.8-Lite demo account states for the video gateway adapter.
-- These rows contain only safe demonstration metadata and masked placeholders.

INSERT INTO video_provider_accounts (
    provider,
    display_name,
    enabled,
    encrypted_api_key,
    masked_key,
    base_url,
    default_model,
    rate_limit_per_minute,
    metadata_json
) VALUES
    (
        'mock',
        '演示通道 A - 正常可用',
        TRUE,
        '',
        '',
        'mock://video-gateway/a',
        'mock-video-v1',
        120,
        '{
          "mode":"local-demo",
          "priority":10,
          "key_status":"normal",
          "health_status":"healthy",
          "last_test_at":"2026-05-17T02:00:00Z"
        }'::jsonb
    ),
    (
        'seedance',
        'Seedance 2.0 - 未配置 Key',
        TRUE,
        '',
        '',
        'demo://seedance/2.0',
        'seedance-2-0-pro',
        60,
        '{
          "priority":20,
          "key_status":"missing",
          "health_status":"needs_key",
          "diagnostic_type":"Key 未配置",
          "suggested_action":"请配置 API Key 后启用真实调用",
          "last_test_at":"2026-05-17T02:03:00Z"
        }'::jsonb
    ),
    (
        'kling',
        'Kling - 停用账号',
        FALSE,
        '',
        '',
        'demo://kling/v1',
        'kling-v1',
        60,
        '{
          "priority":30,
          "key_status":"disabled",
          "health_status":"disabled",
          "suggested_action":"确认业务需要后再启用该 API 通道",
          "last_test_at":"2026-05-17T02:06:00Z"
        }'::jsonb
    ),
    (
        'seedance',
        'Seedance 2.0 - 鉴权失败账号',
        TRUE,
        '',
        'sdnc***demo',
        'demo://seedance/2.0',
        'seedance-2-0-pro',
        60,
        '{
          "priority":40,
          "key_status":"auth_failed",
          "health_status":"auth_failed",
          "diagnostic_type":"鉴权失败",
          "suggested_action":"请检查 Key 是否过期或填错",
          "last_error":"上游返回鉴权失败",
          "last_test_at":"2026-05-17T02:09:00Z"
        }'::jsonb
    ),
    (
        'kling',
        'Kling - 触发限流账号',
        TRUE,
        '',
        'klng***demo',
        'demo://kling/v1',
        'kling-v1',
        30,
        '{
          "priority":50,
          "key_status":"rate_limited",
          "health_status":"rate_limited",
          "diagnostic_type":"触发限流",
          "suggested_action":"请降低并发或增加账号",
          "last_error":"上游返回限流",
          "last_test_at":"2026-05-17T02:12:00Z"
        }'::jsonb
    )
ON CONFLICT (provider, display_name) DO UPDATE
SET enabled = EXCLUDED.enabled,
    encrypted_api_key = EXCLUDED.encrypted_api_key,
    masked_key = EXCLUDED.masked_key,
    base_url = EXCLUDED.base_url,
    default_model = EXCLUDED.default_model,
    rate_limit_per_minute = EXCLUDED.rate_limit_per_minute,
    metadata_json = EXCLUDED.metadata_json,
    updated_at = NOW();
