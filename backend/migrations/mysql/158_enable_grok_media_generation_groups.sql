-- MySQL 方言版本（翻译自 ../158_enable_grok_media_generation_groups.sql）
-- 翻译说明：`groups` 是 MySQL 8 保留字（窗口函数 GROUPS），加反引号；布尔字面量 true/false → TRUE/FALSE。

-- PR 3593 added Grok media routes for image generation, image edits, and video generation.
-- Existing Grok groups were created before the image-generation gate knew about
-- the Grok platform, so backfill them onto the same generation capability gate.
UPDATE `groups`
SET allow_image_generation = TRUE
WHERE platform = 'grok'
  AND allow_image_generation = FALSE;
