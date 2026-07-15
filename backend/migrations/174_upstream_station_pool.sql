-- 上游中转站资源池。该模块使用独立表，不改变现有手工账号语义。
CREATE TABLE IF NOT EXISTS upstream_stations (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE,
    site_type VARCHAR(32) NOT NULL DEFAULT 'auto',
    base_url VARCHAR(1024) NOT NULL,
    credential_mode VARCHAR(32) NOT NULL DEFAULT 'password',
    credential_cipher TEXT NOT NULL DEFAULT '',
    recharge_multiplier NUMERIC(20, 8) NOT NULL DEFAULT 1,
    recharge_source VARCHAR(16) NOT NULL DEFAULT 'manual',
    balance NUMERIC(20, 8),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    auto_sync BOOLEAN NOT NULL DEFAULT TRUE,
    health_status VARCHAR(20) NOT NULL DEFAULT 'unknown',
    last_error TEXT NOT NULL DEFAULT '',
    last_sync_at TIMESTAMPTZ,
    last_test_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_stations_site_type_check CHECK (site_type IN ('auto', 'newapi', 'sub2api')),
    CONSTRAINT upstream_stations_credential_mode_check CHECK (credential_mode IN ('password', 'token', 'api_key')),
    CONSTRAINT upstream_stations_recharge_multiplier_check CHECK (recharge_multiplier > 0),
    CONSTRAINT upstream_stations_recharge_source_check CHECK (recharge_source IN ('manual', 'auto'))
);

CREATE TABLE IF NOT EXISTS upstream_routes (
    id BIGSERIAL PRIMARY KEY,
    station_id BIGINT NOT NULL REFERENCES upstream_stations(id) ON DELETE CASCADE,
    remote_group_key VARCHAR(256) NOT NULL,
    remote_group_name VARCHAR(256) NOT NULL,
    platform VARCHAR(50) NOT NULL,
    models JSONB NOT NULL DEFAULT '[]'::jsonb,
    group_rate NUMERIC(20, 8) NOT NULL DEFAULT 1,
    recharge_multiplier NUMERIC(20, 8) NOT NULL DEFAULT 1,
    effective_rate NUMERIC(20, 8) NOT NULL DEFAULT 1,
    fixed_route BOOLEAN NOT NULL DEFAULT FALSE,
    remote_api_key_id VARCHAR(128) NOT NULL DEFAULT '',
    api_key_cipher TEXT NOT NULL DEFAULT '',
    managed_account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    schedulable BOOLEAN NOT NULL DEFAULT TRUE,
    health_status VARCHAR(20) NOT NULL DEFAULT 'unknown',
    last_error TEXT NOT NULL DEFAULT '',
    last_test_at TIMESTAMPTZ,
    last_sync_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_routes_rate_check CHECK (group_rate >= 0),
    CONSTRAINT upstream_routes_recharge_multiplier_check CHECK (recharge_multiplier > 0),
    CONSTRAINT upstream_routes_effective_rate_check CHECK (effective_rate >= 0),
    CONSTRAINT upstream_routes_station_group_platform_unique UNIQUE (station_id, remote_group_key, platform)
);

CREATE INDEX IF NOT EXISTS upstream_routes_station_rate_idx
    ON upstream_routes (station_id, effective_rate, id);
CREATE INDEX IF NOT EXISTS upstream_routes_managed_account_idx
    ON upstream_routes (managed_account_id) WHERE managed_account_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS upstream_rate_snapshots (
    id BIGSERIAL PRIMARY KEY,
    route_id BIGINT NOT NULL REFERENCES upstream_routes(id) ON DELETE CASCADE,
    group_rate NUMERIC(20, 8) NOT NULL,
    recharge_multiplier NUMERIC(20, 8) NOT NULL,
    effective_rate NUMERIC(20, 8) NOT NULL,
    sampled_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS upstream_rate_snapshots_route_time_idx
    ON upstream_rate_snapshots (route_id, sampled_at DESC);

CREATE TABLE IF NOT EXISTS upstream_sync_logs (
    id BIGSERIAL PRIMARY KEY,
    station_id BIGINT NOT NULL REFERENCES upstream_stations(id) ON DELETE CASCADE,
    action VARCHAR(64) NOT NULL,
    success BOOLEAN NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS upstream_sync_logs_station_time_idx
    ON upstream_sync_logs (station_id, created_at DESC);
