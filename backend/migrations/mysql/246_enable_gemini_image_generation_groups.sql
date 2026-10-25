-- MySQL 方言版本（翻译自 ../246_enable_gemini_image_generation_groups.sql）
-- 翻译说明：`groups` 是 MySQL 8 保留字（窗口函数 GROUPS），加反引号；
-- 布尔字面量 true/false → TRUE/FALSE（MySQL BOOLEAN 实为 TINYINT(1)）。

-- gemini / antigravity 分组的生图开关同时约束 Gemini 系入口，存量分组统一置为开启。
-- composite 分组的同一开关还约束 OpenAI 生图，不在本迁移范围内。
UPDATE `groups`
SET allow_image_generation = TRUE
WHERE platform IN ('gemini', 'antigravity')
  AND allow_image_generation = FALSE;
