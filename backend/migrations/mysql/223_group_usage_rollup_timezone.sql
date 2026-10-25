-- MySQL 方言版本（翻译自 ../223_group_usage_rollup_timezone.sql）
-- 让 /admin/groups 分组日汇总跟随服务端配置时区。
-- 222 迁移生成的存量日桶均为北京时间，因此新增状态默认标记为 Asia/Shanghai；
-- 服务启动后若当前 TZ 不同，后台同步会检测到不一致并重建日桶。
--
-- 翻译决策：
--   - ADD COLUMN IF NOT EXISTS 去掉 IF NOT EXISTS；TEXT 列在 MySQL 不允许字面量 DEFAULT，
--     timezone_name 改为 VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai'（IANA 名称足够容纳）。
--   - PG 的 current_setting('TimeZone') → @@session.time_zone；会话时区墙钟下
--     DATE(created_at) 即会话时区日期，无需 CONVERT_TZ。
--   - 重建 222 的三个触发器（DROP + CREATE 覆盖），日期计算改为会话时区，
--     且更新水位时同步记录 timezone_name = @@session.time_zone。
--   - 注意（需人工复核）：后台重建日桶的 MySQL 版作业写入 timezone_name 时必须使用同一取值
--     （@@session.time_zone），否则快照有效性校验（对比 timezone_name）会判定失效而退化为全量重算。

ALTER TABLE usage_group_rollup_state
    ADD COLUMN timezone_name VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai';

-- (comment skipped) COMMENT ON COLUMN usage_group_rollup_state.timezone_name
-- (comment skipped) COMMENT ON COLUMN usage_group_rollup_state.closed_before
-- (comment skipped) COMMENT ON COLUMN usage_group_daily_rollups.bucket_date

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_insert;
CREATE TRIGGER usage_logs_group_rollup_invalidate_insert
AFTER INSERT ON usage_logs FOR EACH ROW
    UPDATE usage_group_rollup_state
    SET closed_before = LEAST(closed_before, DATE(NEW.created_at)),
        timezone_name = @@session.time_zone,
        updated_at = NOW(3)
    WHERE id = 1
      AND NEW.group_id IS NOT NULL
      AND DATE(NEW.created_at) < closed_before;

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_delete;
CREATE TRIGGER usage_logs_group_rollup_invalidate_delete
AFTER DELETE ON usage_logs FOR EACH ROW
    UPDATE usage_group_rollup_state
    SET closed_before = LEAST(closed_before, DATE(OLD.created_at)),
        timezone_name = @@session.time_zone,
        updated_at = NOW(3)
    WHERE id = 1
      AND OLD.group_id IS NOT NULL
      AND DATE(OLD.created_at) < closed_before;

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
            SET affected_date = DATE(NEW.created_at);
        ELSEIF NEW.group_id IS NULL THEN
            SET affected_date = DATE(OLD.created_at);
        ELSE
            SET affected_date = LEAST(DATE(OLD.created_at), DATE(NEW.created_at));
        END IF;

        UPDATE usage_group_rollup_state
        SET closed_before = LEAST(closed_before, affected_date),
            timezone_name = @@session.time_zone,
            updated_at = NOW(3)
        WHERE id = 1
          AND affected_date < closed_before;
    END IF;
END;
