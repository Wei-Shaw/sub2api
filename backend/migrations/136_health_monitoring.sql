-- 136_health_monitoring.sql
-- 账号健康监控:扩展检测结果表以承载手动测试 + 账号健康快照字段 + 计划唯一约束

-- (A) 扩展 scheduled_test_results 以承载手动测试结果
ALTER TABLE scheduled_test_results ADD COLUMN IF NOT EXISTS account_id BIGINT;
ALTER TABLE scheduled_test_results ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'scheduled';
-- model_id 自包含存储:手动测试 plan_id 为空,无法通过 plan 关联模型;
-- 定时测试也冗余存一份,避免 plan 改模型后历史结果模型misleading。
ALTER TABLE scheduled_test_results ADD COLUMN IF NOT EXISTS model_id VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE scheduled_test_results ALTER COLUMN plan_id DROP NOT NULL;

-- 存量回填 account_id 与 model_id(通过 plan 关联),回填后再给 account_id 加 NOT NULL
UPDATE scheduled_test_results r
SET account_id = p.account_id,
    model_id = COALESCE(NULLIF(r.model_id, ''), p.model_id)
FROM scheduled_test_plans p
WHERE r.plan_id = p.id AND (r.account_id IS NULL OR r.model_id = '');
-- 删除无法回填的孤儿结果(理论上 CASCADE 不会留孤儿,防御性)
DELETE FROM scheduled_test_results WHERE account_id IS NULL;
ALTER TABLE scheduled_test_results ALTER COLUMN account_id SET NOT NULL;

-- source 取值约束(惯例见 108_*.sql 的 CHECK)
ALTER TABLE scheduled_test_results DROP CONSTRAINT IF EXISTS chk_str_source;
ALTER TABLE scheduled_test_results ADD CONSTRAINT chk_str_source
  CHECK (source IN ('manual', 'scheduled'));

-- 按账号查最近结果的索引(服务于聚合,避免 N+1)
CREATE INDEX IF NOT EXISTS idx_str_account_created
  ON scheduled_test_results(account_id, created_at DESC);

-- (B) accounts 表加健康快照字段(Ent 管理,用于正确分页筛选/排序)
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_health_result_status VARCHAR(20);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_health_checked_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_health_latency_ms BIGINT;
CREATE INDEX IF NOT EXISTS idx_accounts_last_health_result_status
  ON accounts(last_health_result_status);
CREATE INDEX IF NOT EXISTS idx_accounts_last_health_checked_at
  ON accounts(last_health_checked_at);

-- (C) 清理重复计划 + 加唯一约束(支持批量 overwrite 的确定性)
-- 破坏性操作:同一 (account_id, model_id) 仅保留最新一条(id 最大者),
-- 其余删除(其结果通过 ON DELETE CASCADE 一并删除)。
-- 上线前请在 staging 核对被删数量;若线上有需保留的重复计划,需人工评估。
DELETE FROM scheduled_test_plans a
USING scheduled_test_plans b
WHERE a.account_id = b.account_id
  AND a.model_id = b.model_id
  AND a.id < b.id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_stp_account_model
  ON scheduled_test_plans(account_id, model_id);
