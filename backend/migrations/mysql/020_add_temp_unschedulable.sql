-- MySQL 方言版本（翻译自 ../020_add_temp_unschedulable.sql）
-- 添加临时不可调度功能相关字段

-- 添加临时不可调度状态解除时间字段
ALTER TABLE accounts ADD COLUMN temp_unschedulable_until DATETIME(3) NULL;

-- 添加临时不可调度原因字段（用于排障和审计）
ALTER TABLE accounts ADD COLUMN temp_unschedulable_reason TEXT NULL;

-- 添加索引以优化调度查询性能
-- （PG 版为部分索引 WHERE deleted_at IS NULL；非唯一部分索引直接去掉 WHERE）
CREATE INDEX idx_accounts_temp_unschedulable_until ON accounts(temp_unschedulable_until);

-- (comment skipped: accounts.temp_unschedulable_until)
-- (comment skipped: accounts.temp_unschedulable_reason)
