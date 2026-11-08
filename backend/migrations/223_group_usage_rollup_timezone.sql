-- 让 /admin/groups 分组日汇总跟随服务端配置时区。
-- 222 迁移生成的存量日桶均为北京时间，因此新增状态默认标记为 Asia/Shanghai；
-- 服务启动后若当前 TZ 不同，后台同步会检测到不一致并重建日桶。

ALTER TABLE usage_group_rollup_state
    ADD COLUMN IF NOT EXISTS timezone_name TEXT NOT NULL DEFAULT 'Asia/Shanghai';

COMMENT ON COLUMN usage_group_rollup_state.timezone_name IS '当前分组日桶采用的 IANA 时区名称。';
COMMENT ON COLUMN usage_group_rollup_state.closed_before IS '已完整发布日桶的配置时区日期排他上界。';
COMMENT ON COLUMN usage_group_daily_rollups.bucket_date IS 'timezone_name 对应时区的自然日。';

CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_date DATE;
    published_before DATE;
    configured_timezone TEXT := current_setting('TimeZone');
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_date := (OLD.created_at AT TIME ZONE configured_timezone)::date;
    ELSE
        IF OLD.group_id IS NULL THEN
            affected_date := (NEW.created_at AT TIME ZONE configured_timezone)::date;
        ELSIF NEW.group_id IS NULL THEN
            affected_date := (OLD.created_at AT TIME ZONE configured_timezone)::date;
        ELSE
            affected_date := LEAST(
                (OLD.created_at AT TIME ZONE configured_timezone)::date,
                (NEW.created_at AT TIME ZONE configured_timezone)::date
            );
        END IF;
    END IF;

    SELECT closed_before
    INTO published_before
    FROM usage_group_rollup_state
    WHERE id = 1
    FOR UPDATE;

    IF published_before > affected_date THEN
        UPDATE usage_group_rollup_state
        SET closed_before = LEAST(closed_before, affected_date),
            updated_at = NOW()
        WHERE id = 1;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_date DATE;
    published_before DATE;
    configured_timezone TEXT := current_setting('TimeZone');
BEGIN
    SELECT MIN((created_at AT TIME ZONE configured_timezone)::date)
    INTO affected_date
    FROM inserted_usage_logs
    WHERE group_id IS NOT NULL;

    IF affected_date IS NULL THEN
        RETURN NULL;
    END IF;

    SELECT closed_before
    INTO published_before
    FROM usage_group_rollup_state
    WHERE id = 1
    FOR KEY SHARE;

    IF published_before > affected_date THEN
        UPDATE usage_group_rollup_state
        SET closed_before = LEAST(closed_before, affected_date),
            updated_at = NOW()
        WHERE id = 1;
    END IF;

    RETURN NULL;
END;
$$;
