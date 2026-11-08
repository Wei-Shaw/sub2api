-- 智力检测：按账号逐个发起「鹈鹕骑自行车」题面请求，留下能力判定与作品原文。
-- 全部为新建表，不修改任何现有表；与渠道监控、定时连通性测试（scheduled_test_*）互不相关。
--
-- 依赖说明：
--   intelligence_check_runs.account_id -> accounts.id
-- 账号删除时跑测记录一并清理，故这里保留外键（与 reconciliation_* 刻意不加外键的取舍不同：
-- 智力检测是低频写入的展示型数据，不存在挡住 accounts 写入路径的问题）。

-- 跑测记录：每账号每次跑测一行。
-- 结果写入后不再更新（重试是新增一行、attempt 递增），所以只保留 created_at，不加 updated_at。
-- 作品原文不放在本表，避免列表查询把大字段一起拉出来。
CREATE TABLE IF NOT EXISTS intelligence_check_runs (
    id               BIGSERIAL PRIMARY KEY,
    account_id       BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    batch_id         UUID,
    trigger_source   VARCHAR(20) NOT NULL DEFAULT 'manual',
    model_id         VARCHAR(100) NOT NULL DEFAULT '',
    upstream_model   VARCHAR(100) NOT NULL DEFAULT '',
    reasoning_effort VARCHAR(20) NOT NULL DEFAULT '',
    prompt_variant   VARCHAR(40) NOT NULL DEFAULT 'classic',
    status           VARCHAR(20) NOT NULL DEFAULT 'queued',
    verdict          VARCHAR(20) NOT NULL DEFAULT 'unknown',
    has_html         BOOLEAN NOT NULL DEFAULT false,
    html_bytes       INT NOT NULL DEFAULT 0,
    latency_ms       BIGINT NOT NULL DEFAULT 0,
    attempt          INT NOT NULL DEFAULT 1,
    error_code       VARCHAR(64) NOT NULL DEFAULT '',
    error_message    TEXT NOT NULL DEFAULT '',
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 卡片墙按账号倒序取「最新一次」，这是最主要的查询形态。
CREATE INDEX IF NOT EXISTS idx_intelligence_check_runs_account_created
    ON intelligence_check_runs (account_id, created_at DESC);

-- 列表按状态过滤（进行中 / 失败）。
CREATE INDEX IF NOT EXISTS idx_intelligence_check_runs_status_created
    ON intelligence_check_runs (status, created_at DESC);

-- 手动批量触发后回看整批进度。
CREATE INDEX IF NOT EXISTS idx_intelligence_check_runs_batch
    ON intelligence_check_runs (batch_id)
    WHERE batch_id IS NOT NULL;

-- 作品原文。与跑测记录一对一，删记录即删作品。
CREATE TABLE IF NOT EXISTS intelligence_check_artifacts (
    run_id     BIGINT PRIMARY KEY REFERENCES intelligence_check_runs(id) ON DELETE CASCADE,
    html_text  TEXT NOT NULL,
    byte_len   INT NOT NULL DEFAULT 0,
    sha256     CHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
