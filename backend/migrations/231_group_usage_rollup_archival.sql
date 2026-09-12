-- 分组日汇总：确立 retained_from 归档屏障，停止在过期清理时重算历史日桶。
--
-- 背景：保留期清理删除 usage_logs 老数据时，失效触发器会把 closed_before
-- 拉回到最老记录的日期，导致后续同步把整个保留窗口的日桶全部删除并重建，
-- 同时抹掉更早的历史日桶，使分组累计用量随每次清理缩水。
--
-- 新不变量：retained_from 之前的日期视为已归档，其日桶是不可变的历史沉淀。
-- 归档删除必须在同一事务内先推进 retained_from 再删数据，触发器据此跳过失效。
--
-- 触发器仍保持 222 迁移的行级/语句级划分（行级覆盖外键级联与直接分区写入），
-- 只是在取状态行之前先无锁判断归档屏障：retained_from 单调递增，读到旧值只会
-- 让本次退回原有加锁路径，不会漏掉需要的失效。

COMMENT ON COLUMN usage_group_rollup_state.retained_from IS
    '归档屏障：早于该时刻的原始日志已被清理，对应日桶冻结为不可变历史，失效触发器不再回退水位。';

CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    old_date DATE;
    new_date DATE;
    affected_date DATE;
    published_before DATE;
    archived_before DATE;
    configured_timezone TEXT := current_setting('TimeZone');
BEGIN
    IF TG_OP = 'DELETE' THEN
        old_date := (OLD.created_at AT TIME ZONE configured_timezone)::date;
    ELSE
        IF OLD.group_id IS NOT NULL OR OLD.api_key_id IS NOT NULL THEN
            old_date := (OLD.created_at AT TIME ZONE configured_timezone)::date;
        END IF;
        IF NEW.group_id IS NOT NULL OR NEW.api_key_id IS NOT NULL THEN
            new_date := (NEW.created_at AT TIME ZONE configured_timezone)::date;
        END IF;
    END IF;

    -- 归档区间的日桶不可变：原始日志已被清理，重建只会得到残缺结果。
    -- 这里刻意不加锁，让保留期清理的批量删除免于逐行争抢状态行。
    SELECT (retained_from AT TIME ZONE configured_timezone)::date
    INTO archived_before
    FROM usage_group_rollup_state
    WHERE id = 1;

    IF old_date IS NOT NULL
        AND (archived_before IS NULL OR old_date >= archived_before) THEN
        affected_date := old_date;
    END IF;
    IF new_date IS NOT NULL
        AND (archived_before IS NULL OR new_date >= archived_before)
        AND (affected_date IS NULL OR new_date < affected_date) THEN
        affected_date := new_date;
    END IF;

    IF affected_date IS NULL THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;

    -- 即使当前已发布水位尚未越过受影响日期，也必须先锁行。
    -- 否则并发关闭作业可能在本事务之后把水位推进，覆盖本次失效。
    SELECT closed_before, (retained_from AT TIME ZONE configured_timezone)::date
    INTO published_before, archived_before
    FROM usage_group_rollup_state
    WHERE id = 1
    FOR UPDATE;

    -- retained_from may have advanced while the no-lock fast path was running.
    -- Re-evaluate both UPDATE sides against the locked barrier so cleanup cannot
    -- race this trigger into rewinding below the archive boundary.
    affected_date := NULL;
    IF old_date IS NOT NULL
        AND (archived_before IS NULL OR old_date >= archived_before) THEN
        affected_date := old_date;
    END IF;
    IF new_date IS NOT NULL
        AND (archived_before IS NULL OR new_date >= archived_before)
        AND (affected_date IS NULL OR new_date < affected_date) THEN
        affected_date := new_date;
    END IF;

    IF affected_date IS NULL THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;

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
    archived_before DATE;
    configured_timezone TEXT := current_setting('TimeZone');
BEGIN
    SELECT closed_before, (retained_from AT TIME ZONE configured_timezone)::date
    INTO published_before, archived_before
    FROM usage_group_rollup_state
    WHERE id = 1
    FOR KEY SHARE;

    -- 混合批量写入不能因为最老一行已归档就丢掉整批失效。先排除归档行，
    -- 再从仍可重建的行里选择最早日期。
    SELECT MIN((created_at AT TIME ZONE configured_timezone)::date)
    INTO affected_date
    FROM inserted_usage_logs
    WHERE (group_id IS NOT NULL OR api_key_id IS NOT NULL)
        AND (
            archived_before IS NULL
            OR (created_at AT TIME ZONE configured_timezone)::date >= archived_before
        );

    IF affected_date IS NULL THEN
        RETURN NULL;
    END IF;

    IF published_before > affected_date THEN
        UPDATE usage_group_rollup_state
        SET closed_before = LEAST(closed_before, affected_date),
            updated_at = NOW()
        WHERE id = 1;
    END IF;

    RETURN NULL;
END;
$$;

-- group 与 api_key 共用发布水位，任一维度发生历史变更都必须使该水位失效。
DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_delete ON usage_logs;
CREATE TRIGGER usage_logs_group_rollup_invalidate_delete
AFTER DELETE ON usage_logs
FOR EACH ROW
WHEN (OLD.group_id IS NOT NULL OR OLD.api_key_id IS NOT NULL)
EXECUTE FUNCTION invalidate_group_usage_rollup_state();

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_update ON usage_logs;
CREATE TRIGGER usage_logs_group_rollup_invalidate_update
AFTER UPDATE OF created_at, group_id, api_key_id, actual_cost ON usage_logs
FOR EACH ROW
WHEN (
    (
        OLD.created_at IS DISTINCT FROM NEW.created_at
        OR OLD.group_id IS DISTINCT FROM NEW.group_id
        OR OLD.api_key_id IS DISTINCT FROM NEW.api_key_id
        OR OLD.actual_cost IS DISTINCT FROM NEW.actual_cost
    )
    AND (
        OLD.group_id IS NOT NULL OR NEW.group_id IS NOT NULL
        OR OLD.api_key_id IS NOT NULL OR NEW.api_key_id IS NOT NULL
    )
)
EXECUTE FUNCTION invalidate_group_usage_rollup_state();

-- 存量修复：把此前被清理拉坏的水位收回到归档屏障之后，
-- 避免升级后首次同步仍然全量重建保留窗口。
UPDATE usage_group_rollup_state
SET closed_before = (retained_from AT TIME ZONE timezone_name)::date,
    updated_at = NOW()
WHERE id = 1
    AND closed_before < (retained_from AT TIME ZONE timezone_name)::date;
