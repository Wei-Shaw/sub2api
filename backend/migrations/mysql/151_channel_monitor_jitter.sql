-- MySQL 方言版本（翻译自 ../151_channel_monitor_jitter.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（按迁移执行器单次执行语义去掉）；
-- INTEGER → INT。

-- Migration: 151_channel_monitor_jitter
-- 渠道监控新增正负随机抖动配置：每次调度在 interval_seconds 基础上
-- ± [0, jitter_seconds] 的均匀随机偏移，避免多个监控固定同步触发。
-- 0（默认）表示固定间隔，与历史行为一致。

ALTER TABLE channel_monitors
    ADD COLUMN jitter_seconds INT NOT NULL DEFAULT 0;
