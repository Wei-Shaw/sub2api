-- 使用记录：记录「这笔调用在上游 A6 实际被扣了多少钱」。
--
-- 与 usage_logs 已有的 cost 列是什么关系（改这一列之前必须先分清）：
--   total_cost / actual_cost / input_cost / output_cost … = 按本站价目表算出来的
--   「应收」，是**计费口径**，决定用户被扣多少余额；
--   upstream_cost_* = A6 账单里的**真实扣费**，是**成本口径**。
--   两者的差就是这笔调用的毛利。因此本迁移只**新增**列，绝不改动任何既有 cost 列，
--   历史数字一律不重算。
--
-- 取数方式（决定了下面这些列为什么必须存在）：
--   调用发生时，网关把上游响应头里的请求标识写进 usage_logs.upstream_request_id
--   （见 232 号迁移）。之后由后台任务拿这个 ID 去 A6 **精确反查单条账单**，
--   把金额写回到这条使用记录上。
--
--   为什么不按时间范围批量拉账单再逐条匹配：该 A6 账号与其它系统共用，
--   批量拉取会把别人的扣费一起拉进本库，需要额外维护「不是我们的账单」这类状态；
--   按 ID 反查天然只取本系统自己的那一条，不存在这个问题。
--
-- 时间常量来自实测（2026-10-02，200 条线上账单）：
--   账单并非即时落库 —— P50≈13s、P90≈66s、P95≈111s、P99≈599s、最大 600s。
--   所以「调用一结束就查」有一半以上会查空，必须延迟后再查、查不到要重试。
--
-- 幂等：全部使用 IF NOT EXISTS，可重复执行。
--
-- ⚠️ 设计取舍一：**不设「取数状态」列**（这一条决定了这张表的热写路径与扫描代价）
--   状态完全可以从下面这三个字段推导出来，不需要独立存储：
--     待查     upstream_request_id 非空 且 upstream_cost_fetched_at 为空 且 attempts < 上限
--     已取到   upstream_cost_fetched_at 非空
--     未取到   fetched_at 为空 且 attempts >= 上限
--     不适用   upstream_request_id 为空（本功能上线前的全部存量记录都是这一类）
--   不设这一列换来两件实在的好处：
--     1) **写入侧一个字都不用改**。usage_logs 的插入路径有 9 条 INSERT 语句，
--        若新增一个「必须由写入方填」的列，就要同时改这 9 处——那是全站最热的
--        写入路径，为了一个可以推导的字段去动它，收益为负、风险为正。
--     2) 存量行不需要任何回填。若用「默认值 = 待查」的写法，49 万条历史记录会
--        全部变成「待查」，取数任务每轮都要扫全表去挑出「待查但没有 ID」的行，
--        等于每 30 秒一次全表扫描。现在历史行因为 upstream_request_id 为空而
--        **天然落在队列之外**，一个字节的写入都不需要。

ALTER TABLE usage_logs
    -- A6 账单里的原币金额（A6 为 USD），原样保存不做四舍五入。
    ADD COLUMN IF NOT EXISTS upstream_cost_original NUMERIC(20, 10),
    -- 原币币种。留列而不是硬编码 USD：上游以后若换结算币种，历史行仍能自解释。
    ADD COLUMN IF NOT EXISTS upstream_cost_currency VARCHAR(8),
    -- 换算后的人民币成本，报表口径使用它。
    ADD COLUMN IF NOT EXISTS upstream_cost_cny NUMERIC(20, 10),
    -- 换算时实际使用的汇率。
    -- 必须冻结在行上：汇率以后会变，历史成本不能跟着重算（与下游收入同样的口径）。
    ADD COLUMN IF NOT EXISTS upstream_cost_fx_rate NUMERIC(20, 10),
    -- 已尝试反查的次数。用于退避与「什么时候认输」的判定。
    ADD COLUMN IF NOT EXISTS upstream_cost_attempts SMALLINT NOT NULL DEFAULT 0,
    -- 取到成本的时刻。**这是「已取到」的唯一判据**，为空表示还没取到。
    ADD COLUMN IF NOT EXISTS upstream_cost_fetched_at TIMESTAMPTZ;

COMMENT ON COLUMN usage_logs.upstream_request_id IS
    '上游（A6）为这次调用分配的请求标识，取自上游响应头；用于反查该笔的真实扣费';
COMMENT ON COLUMN usage_logs.upstream_cost_original IS
    'A6 账单中的原币扣费金额（成本口径，不是本站计费）';
COMMENT ON COLUMN usage_logs.upstream_cost_cny IS
    'A6 真实扣费换算后的人民币金额；报表的「上游成本」列取此值';
COMMENT ON COLUMN usage_logs.upstream_cost_fx_rate IS
    '换算上游成本时冻结的汇率，历史行不随汇率变动重算';
COMMENT ON COLUMN usage_logs.upstream_cost_attempts IS
    '上游成本已尝试反查次数；用尽上限后不再查询，在报表上呈现为「未取到」';
COMMENT ON COLUMN usage_logs.upstream_cost_fetched_at IS
    '取到上游成本的时刻；为空表示尚未取到（这也是「是否已取到」的唯一判据）';

-- 取数任务每轮都要按「带请求 ID + 尚未取到」筛一批记录。
--
-- 用部分索引把绝大多数行排除在索引之外。这张表是高频写入的大表，
-- 而本功能上线前的全部存量记录的 upstream_request_id 都为空，
-- 因此这个索引只会覆盖「本功能上线后产生的、还没取到成本的」那一小撮，
-- 体量极小、维护成本极低，也更不会拖慢插入。
CREATE INDEX IF NOT EXISTS idx_usage_logs_upstream_cost_pending
    ON usage_logs (created_at)
    WHERE upstream_request_id IS NOT NULL AND upstream_request_id <> '' AND upstream_cost_fetched_at IS NULL;
