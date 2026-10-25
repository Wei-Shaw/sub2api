-- MySQL 方言版本（翻译自 ../126_add_user_rpm_limit.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（迁移执行器保证单次执行）。
--   - integer → INT。
--   - COMMENT ON COLUMN 整句跳过。

-- Add per-user Requests-Per-Minute cap.
-- rpm_limit: 用户全局 RPM 兜底（0 = 不限制）。
-- 仅当所访问分组未设置 rpm_limit 且无 user-group rpm_override 时作为兜底生效。
-- 计数键：rpm:u:{user_id}:{minute}。
ALTER TABLE users ADD COLUMN rpm_limit INT NOT NULL DEFAULT 0;

-- (comment skipped)
