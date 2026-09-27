-- 学生资格认证记录：一个学校邮箱只能被一个账号认证（email 唯一），
-- 一个用户同时只有一条认证记录（user_id 唯一）。
-- user_id 可空 + ON DELETE SET NULL：删除账号不释放学校邮箱占用，
-- 仅管理员显式删除记录才释放该邮箱。
CREATE TABLE IF NOT EXISTS student_verifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    email TEXT NOT NULL,
    email_raw TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    verified_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    revoked_by BIGINT NULL,
    revoke_reason TEXT NULL,
    rebate_rate_applied DOUBLE PRECISION NULL,
    previous_rebate_rate DOUBLE PRECISION NULL,
    granted_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS student_verifications_email_idx
    ON student_verifications (email);

CREATE UNIQUE INDEX IF NOT EXISTS student_verifications_user_id_idx
    ON student_verifications (user_id);

CREATE INDEX IF NOT EXISTS student_verifications_status_expires_at_idx
    ON student_verifications (status, expires_at);
