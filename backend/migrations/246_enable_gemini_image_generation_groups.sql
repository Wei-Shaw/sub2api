-- gemini / antigravity 分组的生图开关同时约束 Gemini 系入口，存量分组统一置为开启。
-- composite 分组的同一开关还约束 OpenAI 生图，不在本迁移范围内。
UPDATE groups
SET allow_image_generation = true
WHERE platform IN ('gemini', 'antigravity')
  AND allow_image_generation = false;
