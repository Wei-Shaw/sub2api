-- MySQL 方言版本（翻译自 ../222_group_usage_daily_rollups.sql）
-- /admin/groups 分组用量日汇总。
-- 迁移创建结构与源表失效触发器，历史数据由后台聚合作业按持久水位回填。
--
-- 翻译决策：
--   - PG 的语句级触发器 + transition table → MySQL 行级触发器。插入路径收敛为单条条件
--     UPDATE（「检查水位并回退」原子完成）：正常插入（created_at 日期 >= closed_before）
--     不满足 WHERE 即 no-op、不加锁；命中旧行时 UPDATE 自带行锁，与关闭作业串行化，
--     替代 PG 的 FOR UPDATE / FOR KEY SHARE。
--   - 「北京时间自然日」= DATE(CONVERT_TZ(created_at, @@session.time_zone, '+08:00'))：
--     MySQL DATETIME 存的是会话时区墙钟，先换算到 +08:00 再取日期，与 PG 的
--     AT TIME ZONE 'Asia/Shanghai' 语义一致。
--   - 行级化后 UPDATE 触发器用 `NOT (OLD.col <=> NEW.col ...)` 复刻 PG 的 WHEN 变更条件
--     （MySQL 无 UPDATE OF 列清单，任何列的 UPDATE 都会触发，早退条件保证 no-op）。
--   - CHECK (id = 1) 保留（MySQL 8.0.16+ 强制执行）；COMMENT ON 整句跳过。

CREATE TABLE IF NOT EXISTS usage_group_daily_rollups (
    bucket_date  DATE NOT NULL,
    group_id     BIGINT NOT NULL,
    actual_cost  DECIMAL(20, 10) NOT NULL DEFAULT 0,
    computed_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (bucket_date, group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- (comment skipped) COMMENT ON TABLE usage_group_daily_rollups
-- (comment skipped) COMMENT ON COLUMN usage_group_daily_rollups.bucket_date

CREATE TABLE IF NOT EXISTS usage_group_rollup_state (
    id            SMALLINT NOT NULL PRIMARY KEY,
    closed_before DATE NOT NULL DEFAULT '1970-01-01',
    retained_from DATETIME(3) NOT NULL DEFAULT '1970-01-01 00:00:00',
    updated_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT chk_usage_group_rollup_state_id CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- (comment skipped) COMMENT ON TABLE usage_group_rollup_state
-- (comment skipped) COMMENT ON COLUMN usage_group_rollup_state.closed_before

INSERT INTO usage_group_rollup_state (id, closed_before, retained_from)
VALUES (1, '1970-01-01', '1970-01-01 00:00:00')
ON DUPLICATE KEY UPDATE id = id;

-- 已发布范围的源记录发生变化时，必须在同一事务内后退发布水位。
-- INSERT：仅当新行带 group_id 且其北京时间日期早于当前水位时回退（行级条件 UPDATE 原子完成）。
DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_insert;
CREATE TRIGGER usage_logs_group_rollup_invalidate_insert
AFTER INSERT ON usage_logs FOR EACH ROW
    UPDATE usage_group_rollup_state
    SET closed_before = LEAST(closed_before, DATE(CONVERT_TZ(NEW.created_at, @@session.time_zone, '+08:00'))),
        updated_at = NOW(3)
    WHERE id = 1
      AND NEW.group_id IS NOT NULL
      AND DATE(CONVERT_TZ(NEW.created_at, @@session.time_zone, '+08:00')) < closed_before;

-- DELETE：等价于 PG 行级触发器的 WHEN (OLD.group_id IS NOT NULL)。
DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_delete;
CREATE TRIGGER usage_logs_group_rollup_invalidate_delete
AFTER DELETE ON usage_logs FOR EACH ROW
    UPDATE usage_group_rollup_state
    SET closed_before = LEAST(closed_before, DATE(CONVERT_TZ(OLD.created_at, @@session.time_zone, '+08:00'))),
        updated_at = NOW(3)
    WHERE id = 1
      AND OLD.group_id IS NOT NULL
      AND DATE(CONVERT_TZ(OLD.created_at, @@session.time_zone, '+08:00')) < closed_before;

-- UPDATE：任一受控列（created_at/group_id/actual_cost）值变化且涉及分组时，
-- 按受影响 created_at（变化前后取较早）的北京时间日期失效。
DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_update;
CREATE TRIGGER usage_logs_group_rollup_invalidate_update
AFTER UPDATE ON usage_logs FOR EACH ROW
BEGIN
    DECLARE affected_date DATE;
    IF NOT (OLD.created_at <=> NEW.created_at
            AND OLD.group_id <=> NEW.group_id
            AND OLD.actual_cost <=> NEW.actual_cost)
       AND (OLD.group_id IS NOT NULL OR NEW.group_id IS NOT NULL) THEN
        IF OLD.group_id IS NULL THEN
            SET affected_date = DATE(CONVERT_TZ(NEW.created_at, @@session.time_zone, '+08:00'));
        ELSEIF NEW.group_id IS NULL THEN
            SET affected_date = DATE(CONVERT_TZ(OLD.created_at, @@session.time_zone, '+08:00'));
        ELSE
            SET affected_date = LEAST(
                DATE(CONVERT_TZ(OLD.created_at, @@session.time_zone, '+08:00')),
                DATE(CONVERT_TZ(NEW.created_at, @@session.time_zone, '+08:00'))
            );
        END IF;

        UPDATE usage_group_rollup_state
        SET closed_before = LEAST(closed_before, affected_date),
            updated_at = NOW(3)
        WHERE id = 1
          AND affected_date < closed_before;
    END IF;
END;
