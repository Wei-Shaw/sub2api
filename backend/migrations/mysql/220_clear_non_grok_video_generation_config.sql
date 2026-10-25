-- MySQL 方言版本（翻译自 ../220_clear_non_grok_video_generation_config.sql）
-- 翻译说明：
--   - `groups` 为 MySQL 8 保留字需反引号。
--   - CREATE TABLE IF NOT EXISTS ... AS SELECT 保留（MySQL 支持，重跑为 no-op），
--     显式追加 ENGINE=InnoDB / utf8mb4 表选项（CTAS 语法中表选项置于 SELECT 之前）。
--   - `platform IS DISTINCT FROM 'x'` → `NOT (platform <=> 'x')`（NULL 安全比较取反，
--     platform 为 NULL 时两侧行为一致：计入清空范围）。
--   - NOW() → NOW(3)。
--   - COMMENT ON TABLE 整句跳过。

-- Videos are Grok/xAI-only. Clear stale video pricing from non-Grok groups.
-- Columns match migrations 170/217 (video_price_* / video_model_prices), not a
-- separate allow_video_generation flag which was never applied on this branch.

-- Snapshot before clearing. The UPDATE below is irreversible, and an operator
-- who deliberately priced video on a non-Grok group would otherwise lose that
-- configuration with no way to recover it. CREATE TABLE IF NOT EXISTS ... AS
-- SELECT is a no-op on re-run, so this stays idempotent.
CREATE TABLE IF NOT EXISTS groups_video_price_backup_220
ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
AS
SELECT id AS group_id,
       platform,
       video_price_480p,
       video_price_720p,
       video_price_1080p,
       video_model_prices,
       NOW(3) AS backed_up_at
FROM `groups`
WHERE NOT (platform <=> 'grok')
  AND NOT (platform <=> 'composite')
  AND (
      video_price_480p IS NOT NULL
      OR video_price_720p IS NOT NULL
      OR video_price_1080p IS NOT NULL
      OR video_model_prices IS NOT NULL
  );

-- (comment skipped)

UPDATE `groups`
SET video_price_480p = NULL,
    video_price_720p = NULL,
    video_price_1080p = NULL,
    video_model_prices = NULL
WHERE NOT (platform <=> 'grok')
  AND NOT (platform <=> 'composite')
  AND (
      video_price_480p IS NOT NULL
      OR video_price_720p IS NOT NULL
      OR video_price_1080p IS NOT NULL
      OR video_model_prices IS NOT NULL
  );
