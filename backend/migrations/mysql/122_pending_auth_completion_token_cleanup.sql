-- MySQL 方言版本（翻译自 ../122_pending_auth_completion_token_cleanup.sql）
--
-- 翻译决策：
--   - local_flow_state -> 'completion_response' → JSON_EXTRACT(local_flow_state, '$.completion_response')。
--   - jsonb_typeof(x) = 'object' → JSON_TYPE(x) = 'OBJECT'（MySQL 返回大写）。
--   - jsonb 对象 `- 'key'`（移除顶层键）→ JSON_REMOVE(x, '$.key', ...)（键不存在时为 no-op，语义一致）。
--   - jsonb_set(state, '{completion_response}', new_value, true) → JSON_SET(state,
--     '$.completion_response', new_value)：WHERE 守卫保证 completion_response 已存在且为 object，
--     create_missing 分支不可达，语义一致。
--   - `?` 键存在检查（OR 四项）→ JSON_CONTAINS_PATH(..., 'one', ...)（任一路径存在即为真）。

UPDATE pending_auth_sessions
SET
    local_flow_state = JSON_SET(
        local_flow_state,
        '$.completion_response',
        JSON_REMOVE(
            JSON_EXTRACT(local_flow_state, '$.completion_response'),
            '$.access_token',
            '$.refresh_token',
            '$.expires_in',
            '$.token_type'
        )
    )
WHERE JSON_TYPE(JSON_EXTRACT(local_flow_state, '$.completion_response')) = 'OBJECT'
  AND JSON_CONTAINS_PATH(
      JSON_EXTRACT(local_flow_state, '$.completion_response'),
      'one',
      '$.access_token',
      '$.refresh_token',
      '$.expires_in',
      '$.token_type'
  );
