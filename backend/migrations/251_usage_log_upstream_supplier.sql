-- 使用记录：记录这笔调用在上游 A6 侧的「商家」与「渠道」归属。
--
-- 为什么需要这四列（2026-10-10 漏计费事件的直接产物）：
--   当晚发现 gpt-5.5 / gpt-6-astra 的 tokens 被记成 0，对账后确认是【供应商级】问题
--   ——18 个供应商里只有 4 个丢（1075 无限token 68.7%、1045 mrkutis 100%、
--   121 优质模型提供商 100%、623 Lietio 1.3%），其余 14 个一笔不丢。
--   但定位过程要靠人工把辰数的 usage_logs 与 A6 账单按 request_id JOIN 起来、
--   再按 marketplace_supplier_id 聚合，耗时数小时，期间一直在漏计费。
--   把商家/渠道固化进库后，这类归因可以直接在页面上看到，也是后续
--   「上游异常自动处置」（见 docs/upstream-guard-automation.md）的数据基础。
--
-- 取数方式（与 246/250 号迁移完全同一条链路，不新增任何采集器）：
--   后台任务已经拿 usage_logs.upstream_request_id 去 A6 精确反查那一条账单，
--   而 A6 账单记录本来就带这几个字段：
--     marketplace_supplier_id        = 商户 ID
--     marketplace_supplier_name      = 商户名
--     channel                        = A6 渠道 ID
--     marketplace_target_channel_id  = 目标渠道 ID
--   此前只从这条账单里取了金额与首字/耗时，现在把这四个字段一并落库。
--
-- ⚠️ 与 upstream_cost_* / upstream_first_token_ms 共享同一套生命周期，因此必须一起理解：
--     - 账单**并非即时落库**（实测 P50≈13s、P90≈66s、P99≈599s），所以这四列会晚于
--       请求出现；刚发出的调用在页面上是空的 —— 这是预期行为，不是 bug；
--     - 反查失败或用尽重试上限时为 NULL，页面应显示「—」而不是 0；
--     - 因此**不能**用 `IS NULL` 判断「商家未知」以外的语义，NULL 一律表示「未知」。
--
-- 幂等：全部使用 IF NOT EXISTS，可重复执行。
-- 不回填、不设默认值：历史行天然为 NULL，含义是「本功能上线前没有这个数据」。
-- 写入侧同样一个字都不用改 —— 这四列与 upstream_cost_* 一样，全部由对账任务
-- 在 UPDATE 时填充，不进 usage_logs 的 INSERT 热点路径。

ALTER TABLE usage_logs
    -- 上游（A6）商户 ID。
    ADD COLUMN IF NOT EXISTS upstream_supplier_id INTEGER,
    -- 上游（A6）商户名，冗余存一份便于页面直接显示、无需再关联。
    ADD COLUMN IF NOT EXISTS upstream_supplier_name TEXT,
    -- 上游（A6）实际计费渠道 ID（账单 channel）。
    ADD COLUMN IF NOT EXISTS upstream_channel_id INTEGER,
    -- 上游（A6）目标渠道 ID（账单 marketplace_target_channel_id）。
    ADD COLUMN IF NOT EXISTS upstream_target_channel_id INTEGER;

COMMENT ON COLUMN usage_logs.upstream_supplier_id IS
    '上游（A6）商户 ID，取自账单 marketplace_supplier_id；NULL 表示尚未对账到';
COMMENT ON COLUMN usage_logs.upstream_supplier_name IS
    '上游（A6）商户名，取自账单 marketplace_supplier_name；NULL 表示尚未对账到';
COMMENT ON COLUMN usage_logs.upstream_channel_id IS
    '上游（A6）实际计费渠道 ID，取自账单 channel；NULL 表示尚未对账到';
COMMENT ON COLUMN usage_logs.upstream_target_channel_id IS
    '上游（A6）目标渠道 ID，取自账单 marketplace_target_channel_id；NULL 表示尚未对账到';

-- 不加新索引：这四列永远与 upstream_cost_* 一起写入，查询也永远跟着
-- upstream_request_id / upstream_cost_fetched_at 走，246 号迁移建的部分索引
-- idx_usage_logs_upstream_cost_pending 已经覆盖了取数路径。
-- 按商户聚合排查用不到索引（行数有限、且是低频管理操作）。
