-- MySQL 方言版本（翻译自 ../234_channel_max_reasoning_effort_multiplier.sql）
-- 翻译说明：NUMERIC(10,4) → DECIMAL(10,4)（同义类型）；ADD COLUMN IF NOT EXISTS →
--           ADD COLUMN；COMMENT ON COLUMN 整句跳过。
--           PG 的 `DROP CONSTRAINT IF EXISTS` 为防御性语句（本约束在既往迁移中
--           从未创建，IF EXISTS 兜底人工/漂移场景），MySQL 无 `DROP CHECK IF EXISTS`
--           且约束不存在时 DROP CHECK 直接报错 3830 → 按规则文档改写为临时存储过程
--           + information_schema 存在性判断（与 119 号翻译同型处理）。

ALTER TABLE channel_model_pricing
    ADD COLUMN max_reasoning_effort_multiplier DECIMAL(10,4) NULL;

-- (comment skipped)

DROP PROCEDURE IF EXISTS __mig_234_tmp;
CREATE PROCEDURE __mig_234_tmp()
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.table_constraints
        WHERE constraint_schema = DATABASE()
          AND table_name = 'channel_model_pricing'
          AND constraint_name = 'chk_channel_model_pricing_max_reasoning_effort_positive'
          AND constraint_type = 'CHECK'
    ) THEN
        ALTER TABLE channel_model_pricing
            DROP CHECK chk_channel_model_pricing_max_reasoning_effort_positive;
    END IF;
END;

CALL __mig_234_tmp();
DROP PROCEDURE __mig_234_tmp;

ALTER TABLE channel_model_pricing
    ADD CONSTRAINT chk_channel_model_pricing_max_reasoning_effort_positive
    CHECK (max_reasoning_effort_multiplier IS NULL OR max_reasoning_effort_multiplier > 0);
