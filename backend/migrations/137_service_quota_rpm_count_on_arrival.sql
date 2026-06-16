-- Migration 137: service_quota_limiters 增加 count_on_arrival 列
--
-- 让 RPM 限流器可配置"何时计数"：
--   - count_on_arrival = true：请求到达即 +1（保留旧语义；现存 RPM 行迁移为该值，确保不破坏生产行为）
--   - count_on_arrival = false：仅在请求成功落账（Record 阶段）后 +1，让被拒/失败请求不消耗配额
--
-- 默认值 false：新建 RPM 限流器更符合"成功才计"的直觉，更接近 OpenAI/Anthropic 官方计费口径。
-- 其他 limiter type（tpm/tpd/concurrency/daily_usd）忽略该字段：
--   - tpm/tpd/daily_usd 本身就是后置 Record 阶段计数（无 PreCheck 写入）
--   - concurrency 走 ZSET acquire/release，不存在"成功后才记一笔"的概念
--
-- 幂等：通过 information_schema.columns 判断列是否已存在控制重复执行。

DO $migration$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'service_quota_limiters'
          AND column_name = 'count_on_arrival'
    ) THEN
        RAISE NOTICE 'Migration 137 already applied, skipping';
        RETURN;
    END IF;

    ALTER TABLE service_quota_limiters
        ADD COLUMN count_on_arrival BOOLEAN NOT NULL DEFAULT false;

    -- 现存 RPM 行必须沿用旧"到达即计"语义，否则升级后历史规则的判超限节奏会突变。
    UPDATE service_quota_limiters
       SET count_on_arrival = true
     WHERE limiter_type = 'rpm';

    RAISE NOTICE 'Migration 137 applied: service_quota_limiters.count_on_arrival added';
END
$migration$;
