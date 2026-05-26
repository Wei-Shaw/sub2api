-- Add per-user Requests-Per-Minute cap.
-- rpm_limit: 用户全局 RPM 兜底（0 = 不限制）。
-- 仅当所访问分组未设置 rpm_limit 且无 user-group rpm_override 时作为兜底生效。
-- 计数键：rpm:u:{user_id}:{minute}。
ALTER TABLE users ADD COLUMN IF NOT EXISTS rpm_limit integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN users.rpm_limit IS '用户级 RPM 兜底上限；0 表示不限制；仅当分组未设置 rpm_limit 时生效。';
