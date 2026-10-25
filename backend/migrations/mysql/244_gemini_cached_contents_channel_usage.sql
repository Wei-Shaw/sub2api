-- MySQL 方言版本（翻译自 ../244_gemini_cached_contents_channel_usage.sql）
-- 翻译说明：JSONB NOT NULL DEFAULT '{}'::jsonb → JSON NOT NULL DEFAULT ('{}')
-- （8.0.13+ 表达式默认值）；ADD COLUMN IF NOT EXISTS 按迁移执行器单次执行语义去掉修饰词。

-- Gemini 显式缓存创建时的渠道计费口径（渠道 ID / 请求模型 / 计费模型来源），
-- 延长有效期按同一口径计费。
ALTER TABLE gemini_cached_contents
    ADD COLUMN channel_usage JSON NOT NULL DEFAULT ('{}');
