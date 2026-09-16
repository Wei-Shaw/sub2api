-- Migration: 230_channel_monitor_group_binding
-- 渠道监控配额模式支持「绑定分组」：
--   1. group_id 关联分组，聚合组内全部 active 账号（含被手动暂停的）的额度
--      作为渠道级配额视图；分组删除时置空，监控保留并报「分组未关联」
--   2. group_id 与 account_id 互斥（配额模式二选一）：单账号视图 or 组级聚合视图
--
-- 背景：account_id 是单值 FK，一个账号额度耗尽不代表整个渠道不可用。
-- 账号很多的分组（如多个 kiro 账号轮换）需要聚合口径才能反映真实渠道状态。

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_id ON channel_monitors(group_id);

-- 互斥约束：两者同时非空说明数据源歧义，直接在库层拦掉。
-- 用 DO 块保证灾备重放/重复执行不会因约束已存在而失败。
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint c
          JOIN pg_class t ON t.oid = c.conrelid
         WHERE t.relname = 'channel_monitors'
           AND c.conname = 'channel_monitors_quota_target_check'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_quota_target_check
            CHECK (account_id IS NULL OR group_id IS NULL);
    END IF;
END $$;

COMMENT ON COLUMN channel_monitors.group_id IS
    '配额模式关联的分组 ID（聚合组内全部 active 账号额度）；分组删除时置空；与 account_id 互斥';
