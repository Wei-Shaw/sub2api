-- User daily check-in rewards and admin statistics.

CREATE TABLE IF NOT EXISTS checkin_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    standard_min INTEGER NOT NULL DEFAULT 3 CHECK (standard_min >= 0),
    standard_max INTEGER NOT NULL DEFAULT 10 CHECK (standard_max >= standard_min),
    reduced_threshold DECIMAL(20, 8) NOT NULL DEFAULT 30 CHECK (reduced_threshold >= 0),
    reduced_min INTEGER NOT NULL DEFAULT 1 CHECK (reduced_min >= 0),
    reduced_max INTEGER NOT NULL DEFAULT 3 CHECK (reduced_max >= reduced_min),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO checkin_settings (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS user_checkin_states (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    cycle_reward DECIMAL(20, 8) NOT NULL DEFAULT 0 CHECK (cycle_reward >= 0),
    total_reward DECIMAL(20, 8) NOT NULL DEFAULT 0 CHECK (total_reward >= 0),
    last_checkin_date DATE,
    reset_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_checkins (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_date DATE NOT NULL,
    reward DECIMAL(20, 8) NOT NULL CHECK (reward > 0),
    reward_mode VARCHAR(20) NOT NULL CHECK (reward_mode IN ('standard', 'reduced')),
    cycle_reward_after DECIMAL(20, 8) NOT NULL CHECK (cycle_reward_after >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, checkin_date)
);

CREATE INDEX IF NOT EXISTS idx_user_checkins_date ON user_checkins(checkin_date);
CREATE INDEX IF NOT EXISTS idx_user_checkins_created_at ON user_checkins(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_checkins_user_created ON user_checkins(user_id, created_at DESC);
