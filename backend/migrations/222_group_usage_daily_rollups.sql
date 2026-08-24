-- /admin/groups 分组用量日汇总。
-- 迁移创建结构与源表失效触发器，历史数据由后台聚合作业按持久水位回填。

CREATE TABLE IF NOT EXISTS usage_group_daily_rollups (
    bucket_date DATE NOT NULL,
    group_id BIGINT NOT NULL,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_date, group_id)
);

COMMENT ON TABLE usage_group_daily_rollups IS '按北京时间自然日聚合的分组实际费用。';
COMMENT ON COLUMN usage_group_daily_rollups.bucket_date IS '北京时间自然日。';

CREATE TABLE IF NOT EXISTS usage_group_rollup_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    closed_before DATE NOT NULL DEFAULT DATE '1970-01-01',
    retained_from TIMESTAMPTZ NOT NULL DEFAULT TIMESTAMPTZ '1970-01-01 00:00:00+00',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE usage_group_rollup_state IS '分组日汇总的单行发布水位。';
COMMENT ON COLUMN usage_group_rollup_state.closed_before IS '已完整发布日桶的北京时间日期排他上界。';

INSERT INTO usage_group_rollup_state (id, closed_before, retained_from)
VALUES (1, DATE '1970-01-01', TIMESTAMPTZ '1970-01-01 00:00:00+00')
ON CONFLICT (id) DO NOTHING;

-- 已发布范围的源记录发生变化时，必须在同一事务内后退发布水位。
-- DELETE/UPDATE 使用行级触发器，以覆盖外键级联、分区表和直接分区写入。
CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_date DATE;
    published_before DATE;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_date := (OLD.created_at AT TIME ZONE 'Asia/Shanghai')::date;
    ELSE
        IF OLD.group_id IS NULL THEN
            affected_date := (NEW.created_at AT TIME ZONE 'Asia/Shanghai')::date;
        ELSIF NEW.group_id IS NULL THEN
            affected_date := (OLD.created_at AT TIME ZONE 'Asia/Shanghai')::date;
        ELSE
            affected_date := LEAST(
                (OLD.created_at AT TIME ZONE 'Asia/Shanghai')::date,
                (NEW.created_at AT TIME ZONE 'Asia/Shanghai')::date
            );
        END IF;
    END IF;

    -- 即使当前已发布水位尚未越过受影响日期，也必须先锁行。
    -- 否则并发关闭作业可能在本事务之后把水位推进，覆盖本次失效。
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

-- INSERT 是网关高频路径。transition table 让每个批量 INSERT 只锁一次状态行；
-- KEY SHARE 在普通写入之间兼容，但会与关闭作业的 FOR UPDATE 串行化。
CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_date DATE;
    published_before DATE;
BEGIN
    SELECT MIN((created_at AT TIME ZONE 'Asia/Shanghai')::date)
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

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_insert ON usage_logs;
CREATE TRIGGER usage_logs_group_rollup_invalidate_insert
AFTER INSERT ON usage_logs
REFERENCING NEW TABLE AS inserted_usage_logs
FOR EACH STATEMENT
EXECUTE FUNCTION invalidate_group_usage_rollup_state_after_insert();

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_delete ON usage_logs;
CREATE TRIGGER usage_logs_group_rollup_invalidate_delete
AFTER DELETE ON usage_logs
FOR EACH ROW
WHEN (OLD.group_id IS NOT NULL)
EXECUTE FUNCTION invalidate_group_usage_rollup_state();

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_update ON usage_logs;
CREATE TRIGGER usage_logs_group_rollup_invalidate_update
AFTER UPDATE OF created_at, group_id, actual_cost ON usage_logs
FOR EACH ROW
WHEN (
    (
        OLD.created_at IS DISTINCT FROM NEW.created_at
        OR OLD.group_id IS DISTINCT FROM NEW.group_id
        OR OLD.actual_cost IS DISTINCT FROM NEW.actual_cost
    )
    AND (OLD.group_id IS NOT NULL OR NEW.group_id IS NOT NULL)
)
EXECUTE FUNCTION invalidate_group_usage_rollup_state();
