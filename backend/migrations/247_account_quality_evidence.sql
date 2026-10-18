ALTER TABLE account_quality_runs ADD COLUMN benchmark TEXT NOT NULL DEFAULT 'candy';
ALTER TABLE account_quality_runs ADD COLUMN scheduled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_quality_results ADD COLUMN evidence JSONB NOT NULL DEFAULT '{}';
ALTER TABLE account_quality_results ADD COLUMN endpoint TEXT NOT NULL DEFAULT 'responses';
ALTER TABLE account_quality_results ADD COLUMN requests_used INT NOT NULL DEFAULT 1 CHECK(requests_used>=0 AND requests_used<=3);
CREATE INDEX account_quality_result_identity ON account_quality_results(account_id,model,effort,service_tier,benchmark,created_at DESC);
CREATE TABLE account_quality_weights (
 group_id BIGINT NOT NULL REFERENCES groups(id), account_id BIGINT NOT NULL REFERENCES accounts(id),
 scope_key TEXT NOT NULL, multiplier DOUBLE PRECISION NOT NULL DEFAULT 1 CHECK(multiplier>0 AND multiplier<=4),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), PRIMARY KEY(group_id,account_id,scope_key)
);
