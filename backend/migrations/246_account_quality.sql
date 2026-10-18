-- Local account quality observations. No credentials or changes to account priority.
CREATE TABLE account_quality_policies (
 group_id BIGINT PRIMARY KEY REFERENCES groups(id), config JSONB NOT NULL DEFAULT '{}',
 next_run_at TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE account_quality_runs (
 id BIGSERIAL PRIMARY KEY, group_id BIGINT NOT NULL REFERENCES groups(id),
 spec JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'queued',
 planned INT NOT NULL DEFAULT 0, completed INT NOT NULL DEFAULT 0,
 lease_until TIMESTAMPTZ NOT NULL DEFAULT NOW()+interval '3 minutes',
 cancel_requested BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), finished_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX account_quality_one_active_group ON account_quality_runs(group_id) WHERE status IN ('queued','running');
CREATE UNIQUE INDEX account_quality_one_active_round ON account_quality_runs((true)) WHERE status IN ('queued','running');
CREATE TABLE account_quality_results (
 id BIGSERIAL PRIMARY KEY, run_id BIGINT NOT NULL REFERENCES account_quality_runs(id),
 group_id BIGINT NOT NULL, account_id BIGINT NOT NULL, model TEXT NOT NULL,
 effort TEXT NOT NULL, service_tier TEXT NOT NULL DEFAULT '', benchmark TEXT NOT NULL,
 verdict TEXT NOT NULL, answer TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '',
 duration_ms BIGINT NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX account_quality_result_scope ON account_quality_results(group_id, account_id, created_at DESC);
CREATE INDEX account_quality_result_budget ON account_quality_results(group_id, created_at);
