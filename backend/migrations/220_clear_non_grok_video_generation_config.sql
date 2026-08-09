-- Videos are Grok/xAI-only. Clear stale video pricing from non-Grok groups.
-- Columns match migrations 170/217 (video_price_* / video_model_prices), not a
-- separate allow_video_generation flag which was never applied on this branch.

-- Snapshot before clearing. The UPDATE below is irreversible, and an operator
-- who deliberately priced video on a non-Grok group would otherwise lose that
-- configuration with no way to recover it. CREATE TABLE IF NOT EXISTS ... AS
-- SELECT is a no-op on re-run, so this stays idempotent.
CREATE TABLE IF NOT EXISTS groups_video_price_backup_220 AS
SELECT id AS group_id,
       platform,
       video_price_480p,
       video_price_720p,
       video_price_1080p,
       video_model_prices,
       now() AS backed_up_at
FROM groups
WHERE platform IS DISTINCT FROM 'grok'
  AND platform IS DISTINCT FROM 'composite'
  AND (
      video_price_480p IS NOT NULL
      OR video_price_720p IS NOT NULL
      OR video_price_1080p IS NOT NULL
      OR video_model_prices IS NOT NULL
  );

COMMENT ON TABLE groups_video_price_backup_220 IS
    '迁移 220 清空非 Grok/非 composite 分组视频价前的快照。composite 可能路由到 Grok 账号，予以保留。确认无需回滚后可安全 DROP；回滚方式：UPDATE groups g SET video_price_480p = b.video_price_480p, ... FROM groups_video_price_backup_220 b WHERE g.id = b.group_id';

UPDATE groups
SET video_price_480p = NULL,
    video_price_720p = NULL,
    video_price_1080p = NULL,
    video_model_prices = NULL
WHERE platform IS DISTINCT FROM 'grok'
  AND platform IS DISTINCT FROM 'composite'
  AND (
      video_price_480p IS NOT NULL
      OR video_price_720p IS NOT NULL
      OR video_price_1080p IS NOT NULL
      OR video_model_prices IS NOT NULL
  );
