-- D3-b 保留期清理(NULL-OUT)：超过保留天数的采集内容把 prompt_redacted/response_redacted 置空，
-- 保留行与计数(护城河看板全时段指标持续累计)。
--
-- 该 partial index 的谓词与清理 query 谓词一致：行被清空(两列均为 '')后即离开索引，
-- 使每轮清理只扫「尚有内容」的行，避免随历史无界增长而退化(扫描成本恒定有界)。
CREATE INDEX IF NOT EXISTS idx_ai_generation_content_unpurged_created_at
    ON ai_generation_content (created_at)
    WHERE prompt_redacted <> '' OR response_redacted <> '';
