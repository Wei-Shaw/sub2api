CREATE TABLE IF NOT EXISTS installation_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    bootstrapped_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
