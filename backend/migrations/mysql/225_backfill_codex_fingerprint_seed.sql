-- MySQL 方言版本（翻译自 ../225_backfill_codex_fingerprint_seed.sql）
-- 翻译说明（PG 为单条 UPDATE，无 DO 块，语义直接改写）：
--   - jsonb_set(COALESCE(extra,'{}'::jsonb), '{codex_fingerprint_seed}',
--               to_jsonb(gen_random_uuid()::text), true)
--       → JSON_SET(COALESCE(extra, CAST('{}' AS JSON)), '$.codex_fingerprint_seed', UUID())
--     （gen_random_uuid() → UUID()；UUID() 在 UPDATE 中逐行求值，每行一个新 UUID；
--       JSON_SET 写入字符串值即为 JSON 字符串，与 to_jsonb(text) 等价）。
--   - extra->>'k' → JSON_UNQUOTE(JSON_EXTRACT(extra, '$.k'))；btrim → TRIM。
--   - 正则匹配 `~` → REGEXP_LIKE()（模式无反斜杠字符，无需转义调整）；
--     PG `~` 区分大小写而 MySQL REGEXP_LIKE 默认不区分 → 第三参传 'c' 强制区分，
--     保证大写十六进制的存量 seed 与 PG 行为一致（判为无效并重新回填）。

-- Backfill system-managed Codex fingerprint seeds for enabled OpenAI OAuth accounts.
-- Idempotent: valid canonical seeds are preserved on rerun.
UPDATE accounts
SET extra = JSON_SET(
    COALESCE(extra, CAST('{}' AS JSON)),
    '$.codex_fingerprint_seed',
    UUID()
)
WHERE deleted_at IS NULL
  AND platform = 'openai'
  AND type = 'oauth'
  AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.codex_fingerprint_mode')), '') IN ('device', 'session', 'full')
  AND (
      JSON_UNQUOTE(JSON_EXTRACT(extra, '$.codex_fingerprint_seed')) IS NULL
      OR TRIM(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.codex_fingerprint_seed'))) = ''
      OR NOT (
          REGEXP_LIKE(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.codex_fingerprint_seed')), '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$', 'c')
          AND JSON_UNQUOTE(JSON_EXTRACT(extra, '$.codex_fingerprint_seed')) <> '00000000-0000-0000-0000-000000000000'
      )
  );
