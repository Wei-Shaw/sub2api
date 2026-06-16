-- Migration 136: service_quota_limiters 增加 token_components 列
--
-- 让 TPM/TPD 限流器可配置"哪几种 token 计入计数"，对齐 Anthropic 3.7+ / Bedrock 3.7+
-- 主流口径：默认 {input, output, cache_creation}（剔除 cache_read）。
--
-- 业界共识：
--   - input + output 必须计入（行业无一例外）
--   - cache_creation 通常计入（属于"新写"成本）
--   - cache_read 是分歧点：OpenAI/Gemini 计入；Anthropic 3.7+/Bedrock/Groq 不计入（鼓励缓存）
--
-- 用户可在管理界面按 limiter 单独勾选；其他 limiter type（rpm/concurrency/daily_usd）
-- 不使用此字段，service 层会强制清洗为 NULL/空数组以保持语义一致。
--
-- 幂等：通过 information_schema.columns 判断列是否已存在控制重复执行。

DO $migration$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'service_quota_limiters'
          AND column_name = 'token_components'
    ) THEN
        RAISE NOTICE 'Migration 136 already applied, skipping';
        RETURN;
    END IF;

    ALTER TABLE service_quota_limiters
        ADD COLUMN token_components TEXT[] NOT NULL
        DEFAULT ARRAY['input', 'output', 'cache_creation']::TEXT[];

    -- 至少 1 项；只允许 4 个合法值
    ALTER TABLE service_quota_limiters
        ADD CONSTRAINT service_quota_limiters_token_components_min
        CHECK (
            limiter_type NOT IN ('tpm', 'tpd')
            OR (
                array_length(token_components, 1) >= 1
                AND token_components <@ ARRAY['input', 'output', 'cache_creation', 'cache_read']::TEXT[]
            )
        );

    RAISE NOTICE 'Migration 136 applied: service_quota_limiters.token_components added';
END
$migration$;
