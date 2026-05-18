CREATE TABLE IF NOT EXISTS user_daily_check_ins (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    check_in_days INTEGER NOT NULL DEFAULT 0,
    last_check_in_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_daily_check_ins_last_check_in_at
    ON user_daily_check_ins(last_check_in_at);
