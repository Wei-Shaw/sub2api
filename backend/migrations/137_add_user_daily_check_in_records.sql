CREATE TABLE IF NOT EXISTS user_daily_check_in_records (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    check_in_date DATE NOT NULL,
    checked_in_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, check_in_date)
);

CREATE INDEX IF NOT EXISTS idx_user_daily_check_in_records_user_date
    ON user_daily_check_in_records(user_id, check_in_date);

INSERT INTO user_daily_check_in_records (
    user_id,
    check_in_date,
    checked_in_at,
    created_at,
    updated_at
)
SELECT
    user_id,
    (last_check_in_at AT TIME ZONE current_setting('TIMEZONE'))::date,
    last_check_in_at,
    created_at,
    updated_at
FROM user_daily_check_ins
WHERE last_check_in_at IS NOT NULL
ON CONFLICT (user_id, check_in_date) DO NOTHING;
