-- Per-model rate limits: extend api_keys and groups with JSONB columns.
--
-- api_keys.model_rate_limits:
--   When non-NULL and non-empty, fully overrides the inherited group defaults
--   (replace semantics, not merge).
--   Each element: {"pattern": "...", "limit_5h": <usd>, "limit_1d": <usd>, "limit_7d": <usd>}
--   Pattern follows the same wildcard rules as model routing: exact match or trailing "*".
--   A limit value of 0 (or absent) means the window is unlimited.
--
-- groups.default_model_rate_limits:
--   Default rules inherited by every API key in the group that has no override.
--
-- Backwards compatible: NULL = no per-model limits configured, current behavior preserved.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS model_rate_limits JSONB;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS default_model_rate_limits JSONB;
