-- MySQL 方言版本（翻译自 ../042b_add_ops_system_metrics_switch_count.sql）

-- ops_system_metrics 增加账号切换次数统计（按分钟窗口）
ALTER TABLE ops_system_metrics
    ADD COLUMN account_switch_count BIGINT NOT NULL DEFAULT 0;
