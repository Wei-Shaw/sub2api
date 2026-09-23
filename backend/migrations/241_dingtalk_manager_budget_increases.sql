CREATE TABLE IF NOT EXISTS dingtalk_manager_budget_increases (
 id BIGSERIAL PRIMARY KEY,
 actor_id BIGINT NOT NULL REFERENCES users(id),
 manager_id BIGINT NOT NULL REFERENCES dingtalk_manager_budgets(user_id),
 amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
 limit_cents_after BIGINT NOT NULL,
 request_id VARCHAR(64) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(actor_id, request_id)
);
CREATE INDEX IF NOT EXISTS idx_dingtalk_budget_increases_manager ON dingtalk_manager_budget_increases(manager_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_dingtalk_quota_grants_actor_id ON dingtalk_quota_grants(actor_id, id DESC);
