-- A complete snapshot per application. Never partially replace a directory.
CREATE TABLE IF NOT EXISTS dingtalk_directory_snapshots (
 app_id VARCHAR(64) PRIMARY KEY,
 synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS dingtalk_departments (
 app_id VARCHAR(64) NOT NULL,
 department_id BIGINT NOT NULL,
 parent_id BIGINT NOT NULL,
 name TEXT NOT NULL,
 PRIMARY KEY (app_id, department_id)
);
CREATE TABLE IF NOT EXISTS dingtalk_members (
 app_id VARCHAR(64) NOT NULL,
 department_id BIGINT NOT NULL,
 union_id TEXT NOT NULL,
 staff_id TEXT NOT NULL,
 name TEXT NOT NULL,
 PRIMARY KEY (app_id, department_id, union_id),
 FOREIGN KEY (app_id, department_id) REFERENCES dingtalk_departments(app_id, department_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_dingtalk_members_union ON dingtalk_members(app_id, union_id);
-- Budget is per platform user across all applications and managed departments.
-- Zero grants no spending authority. Changing departments never resets spending.
CREATE TABLE IF NOT EXISTS dingtalk_manager_budgets (
 user_id BIGINT PRIMARY KEY REFERENCES users(id),
 limit_cents BIGINT NOT NULL DEFAULT 0 CHECK (limit_cents >= 0),
 used_cents BIGINT NOT NULL DEFAULT 0 CHECK (used_cents >= 0),
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS dingtalk_department_managers (
 user_id BIGINT NOT NULL REFERENCES users(id),
 app_id VARCHAR(64) NOT NULL,
 department_id BIGINT NOT NULL,
 PRIMARY KEY(user_id, app_id, department_id)
);
CREATE TABLE IF NOT EXISTS dingtalk_quota_grants (
 id BIGSERIAL PRIMARY KEY,
 actor_id BIGINT NOT NULL REFERENCES users(id),
 target_id BIGINT NOT NULL REFERENCES users(id),
 app_id VARCHAR(64) NOT NULL,
 department_id BIGINT NOT NULL,
 amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
 request_id VARCHAR(64) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(actor_id, request_id)
);
