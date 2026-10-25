-- MySQL 方言版本（翻译自 ../239_channel_reasoning_effort_multipliers.sql）
-- 翻译说明：
--   - DO 块（reasoning_effort_multipliers 列不存在时才加列并回填）→ 列由本迁移首次引入，
--     迁移执行器保证单次执行，去掉存在性分支直接执行；仅在引入列的第一次执行时回填，
--     与 PG「重放不得覆盖现有设置（包括已被刻意清空的 map）」语义一致。
--     jsonb_build_object('max', col) → JSON_OBJECT('max', col)。
--   - groups.model_pricing 数组重写（jsonb_array_elements WITH ORDINALITY +
--     jsonb_agg ... ORDER BY ordinal 保序重建）→ MySQL 的 JSON_ARRAYAGG 无法保证
--     元素顺序、GROUP_CONCAT 受 group_concat_max_len 限制，改用临时存储过程
--     逐元素循环：按索引顺序 JSON_ARRAY_APPEND 重建数组保持顺序，仅在确实发生
--     改写时回写（等价 PG 外层 WHERE 的 EXISTS 过滤，无遗留键的行保持原值）。
--   - entry ? 'key' → JSON_CONTAINS_PATH(entry, 'one', '$.key')；
--     jsonb_typeof(x) = 'number' → JSON_TYPE(x) IN ('INTEGER','DOUBLE','DECIMAL','UNSIGNED')
--     （PG 的 jsonb number 覆盖 MySQL 全部 JSON 数值类型）；
--     entry - 'key' → JSON_REMOVE(entry, '$.key')；
--     jsonb || jsonb → JSON_MERGE_PATCH（右侧键与刚移除的键不同名，顶层合并语义等价；
--     右侧为 '{}' 时等价于纯移除遗留键）。
--   - `groups` 为 MySQL 8 保留字，加反引号；COMMENT ON COLUMN 整句跳过。

-- 1) channel_model_pricing：仅迁移显式配置的旧 max 定价，无模型级默认值。
ALTER TABLE channel_model_pricing
    ADD COLUMN reasoning_effort_multipliers JSON NOT NULL DEFAULT ('{}');

UPDATE channel_model_pricing
SET reasoning_effort_multipliers = JSON_OBJECT('max', max_reasoning_effort_multiplier)
WHERE max_reasoning_effort_multiplier IS NOT NULL;

-- 2) channel_account_stats_model_pricing：仅加列，不做回填。
ALTER TABLE channel_account_stats_model_pricing
    ADD COLUMN reasoning_effort_multipliers JSON NOT NULL DEFAULT ('{}');

-- 3) groups.model_pricing：消费数组元素内的遗留键 max_reasoning_effort_multiplier，
--    改写为 reasoning_effort_multipliers；已存在的通用 map（即使为空）优先，
--    重放不会复活遗留键。
DROP PROCEDURE IF EXISTS __migration_239_tmp;

CREATE PROCEDURE __migration_239_tmp()
main: BEGIN
    DECLARE v_gid BIGINT;
    DECLARE v_pricing JSON;
    DECLARE v_entry JSON;
    DECLARE v_legacy JSON;
    DECLARE v_new JSON;
    DECLARE v_had_new_key INT DEFAULT 0;
    DECLARE v_changed INT DEFAULT 0;
    DECLARE v_n INT DEFAULT 0;
    DECLARE v_i INT DEFAULT 0;
    DECLARE v_done INT DEFAULT 0;
    DECLARE cur CURSOR FOR
        SELECT id, model_pricing
          FROM `groups`
         WHERE model_pricing IS NOT NULL
           AND JSON_TYPE(model_pricing) = 'ARRAY';
    DECLARE CONTINUE HANDLER FOR NOT FOUND SET v_done = 1;

    OPEN cur;
    read_loop: LOOP
        FETCH cur INTO v_gid, v_pricing;
        IF v_done = 1 THEN
            LEAVE read_loop;
        END IF;

        SET v_new = JSON_ARRAY();
        SET v_changed = 0;
        SET v_n = JSON_LENGTH(v_pricing);
        SET v_i = 0;

        elem_loop: WHILE v_i < v_n DO
            SET v_entry = JSON_EXTRACT(v_pricing, CONCAT('$[', v_i, ']'));
            IF JSON_TYPE(v_entry) = 'OBJECT'
               AND JSON_CONTAINS_PATH(v_entry, 'one', '$.max_reasoning_effort_multiplier') THEN
                SET v_legacy = JSON_EXTRACT(v_entry, '$.max_reasoning_effort_multiplier');
                SET v_had_new_key = JSON_CONTAINS_PATH(v_entry, 'one', '$.reasoning_effort_multipliers');
                SET v_entry = JSON_REMOVE(v_entry, '$.max_reasoning_effort_multiplier');
                IF v_had_new_key = 0
                   AND JSON_TYPE(v_legacy) IN ('INTEGER', 'DOUBLE', 'DECIMAL', 'UNSIGNED') THEN
                    SET v_entry = JSON_MERGE_PATCH(
                        v_entry,
                        JSON_OBJECT('reasoning_effort_multipliers',
                                    JSON_OBJECT('max', v_legacy))
                    );
                END IF;
                SET v_changed = 1;
            END IF;
            SET v_new = JSON_ARRAY_APPEND(v_new, '$', v_entry);
            SET v_i = v_i + 1;
        END WHILE;

        IF v_changed = 1 THEN
            UPDATE `groups`
               SET model_pricing = v_new
             WHERE id = v_gid;
        END IF;
    END LOOP;
    CLOSE cur;
END;

CALL __migration_239_tmp();
DROP PROCEDURE __migration_239_tmp;

-- (comment skipped) COMMENT ON COLUMN channel_model_pricing.reasoning_effort_multipliers IS
--     'Custom billing multipliers by reasoning effort; omitted efforts use 1x'
-- (comment skipped) COMMENT ON COLUMN channel_account_stats_model_pricing.reasoning_effort_multipliers IS
--     'Custom account statistics billing multipliers by reasoning effort; omitted efforts use 1x'
