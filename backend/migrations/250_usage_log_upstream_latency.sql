-- 使用记录：记录「这笔调用在上游 A6 侧的首字耗时与总耗时」。
--
-- 为什么要有这两列（与本站已有的延迟列是什么关系）：
--   usage_logs.first_token_ms / duration_ms = **本站观测**，从网关开始处理请求算起，
--   包含中转自身的开销、网络往返、以及上游的全部处理时间；
--   upstream_first_token_ms / upstream_duration_ms = **上游自报**，只覆盖上游内部那一段。
--   两者相减就是「中转开销」，这正是排障与对比时最需要的数字，但此前只能靠外部
--   脚本 JOIN 上游账单才能算出来。本迁移把它固化进库，让管理员在页面上直接对比。
--
-- 取数方式（与 246 号迁移的 upstream_cost_* 完全同一条链路，不新增任何采集器）：
--   后台任务已经拿 usage_logs.upstream_request_id 去 A6 精确反查那一条账单，
--   而 A6 的账单记录里本来就带这两个字段：
--     other.frt  = 上游首字耗时，**毫秒**；
--     use_time   = 上游总耗时，**整秒**（上游只给整数，故本列精度只到秒）。
--   此前只从这条账单里取了金额，现在把这两个字段一并落库。
--
-- ⚠️ 与 upstream_cost_* 共享同一套生命周期，因此必须一起理解：
--     - 调用发生时上游响应头里带回 request_id，写在 usage_request_id 上；
--     - 账单**并非即时落库**（实测 P50≈13s、P90≈66s、P99≈599s），所以这两列
--       会晚于请求出现，刚发出的调用在页面上是空的 —— 这是预期行为，不是 bug；
--     - 反查失败或用尽重试上限时为 NULL，页面应显示为「—」而不是 0；
--     - 因此**不能用 `IS NULL` 判断「上游很快」**，NULL 一律表示「未知」。
--
-- ⚠️ 精度声明（展示时必须遵守）：
--     upstream_first_token_ms 是毫秒精度；
--     upstream_duration_ms 由上游整秒值换算而来（×1000），**有效精度只有秒级**，
--     不能拿它去和本站 duration_ms 做几十毫秒级的比较。
--
-- 幂等：全部使用 IF NOT EXISTS，可重复执行。
--
-- 不回填、不设默认值：历史行天然为 NULL，含义是「本功能上线前没有这个数据」，
-- 与「查了但没查到」在展示上都走「—」，无需区分，也就不需要任何回填写入。
-- 写入侧同样一个字都不用改 —— 这两列与本功能上线前的 upstream_cost_* 一样，
-- 全部由对账任务在 UPDATE 时填入，不在 usage_logs 的 9 条 INSERT 热点路径上。

ALTER TABLE usage_logs
    -- 上游（A6）自报的首字耗时，单位毫秒。
    ADD COLUMN IF NOT EXISTS upstream_first_token_ms INTEGER,
    -- 上游（A6）自报的总耗时，单位毫秒（由上游整秒值 ×1000，精度只到秒）。
    ADD COLUMN IF NOT EXISTS upstream_duration_ms INTEGER;

COMMENT ON COLUMN usage_logs.upstream_first_token_ms IS
    '上游（A6）账单里的首字耗时（毫秒，精度毫秒）；NULL 表示尚未对账到或上游未回传';
COMMENT ON COLUMN usage_logs.upstream_duration_ms IS
    '上游（A6）账单里的总耗时（毫秒，由上游整秒值换算，精度只到秒）；NULL 表示尚未对账到或上游未回传';

-- 不加新索引：这两列永远与 upstream_cost_* 一起写入，查询也永远跟着
-- upstream_request_id / upstream_cost_fetched_at 走，246 号迁移建的部分索引
-- idx_usage_logs_upstream_cost_pending 已经覆盖了取数路径。
