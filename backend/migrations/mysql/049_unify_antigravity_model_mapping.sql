-- MySQL 方言版本（翻译自 ../049_unify_antigravity_model_mapping.sql）
-- Force set default Antigravity model_mapping.
--
-- Notes:
-- - Applies to both Antigravity OAuth and Upstream accounts.
-- - Overwrites existing credentials.model_mapping.
-- - Removes legacy credentials.model_whitelist.
--
-- 翻译说明：PG 的 `jsonb - 'key'`（删除顶层键）→ JSON_REMOVE；
--           PG 的 `jsonb || jsonb`（对象浅合并，右侧优先）→ JSON_MERGE_PATCH。
--           右侧仅含 model_mapping 键且左侧已移除同名键，两种合并语义结果一致。

UPDATE accounts
SET credentials = JSON_MERGE_PATCH(
    JSON_REMOVE(COALESCE(credentials, CAST('{}' AS JSON)), '$.model_whitelist', '$.model_mapping'),
    CAST('{
  "model_mapping": {
    "claude-opus-4-6": "claude-opus-4-6",
    "claude-opus-4-5-thinking": "claude-opus-4-5-thinking",
    "claude-opus-4-5-20251101": "claude-opus-4-5-thinking",
    "claude-sonnet-4-5": "claude-sonnet-4-5",
    "claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
    "claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
    "claude-haiku-4-5": "claude-sonnet-4-5",
    "claude-haiku-4-5-20251001": "claude-sonnet-4-5",
    "gemini-2.5-flash": "gemini-2.5-flash",
    "gemini-2.5-flash-lite": "gemini-2.5-flash-lite",
    "gemini-2.5-flash-thinking": "gemini-2.5-flash-thinking",
    "gemini-2.5-pro": "gemini-2.5-pro",
    "gemini-3-flash": "gemini-3-flash",
    "gemini-3-flash-preview": "gemini-3-flash",
    "gemini-3-pro-high": "gemini-3-pro-high",
    "gemini-3-pro-low": "gemini-3-pro-low",
    "gemini-3-pro-image": "gemini-3-pro-image",
    "gemini-3-pro-preview": "gemini-3-pro-high",
    "gemini-3-pro-image-preview": "gemini-3-pro-image",
    "gpt-oss-120b-medium": "gpt-oss-120b-medium",
    "tab_flash_lite_preview": "tab_flash_lite_preview"
  }
}' AS JSON)
)
WHERE platform = 'antigravity'
  AND deleted_at IS NULL;
