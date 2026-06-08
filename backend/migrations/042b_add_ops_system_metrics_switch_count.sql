-- ops_system_metrics 增加账号切换次数统计（按分钟窗口）
ALTER TABLE ops_system_metrics
    ADD COLUMN IF NOT EXISTS account_switch_count BIGINT NOT NULL DEFAULT 0;
