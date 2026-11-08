-- 247_drop_reconciliation_module.sql
--
-- 背景：原「经营对账」（Companion）模块已整体下线。
--
-- 它的做法是把 A6 一段时间的账单整段拉回来，再逐条与本站使用记录做匹配，
-- 因此需要四张表来存放：匹配规则（account_rules）、拉回来的上游账单
-- （upstream_bills）、逐条调用的补充快照（usage_extras）、以及同步游标
-- （sync_state）。
--
-- 新的做法不再做匹配：网关已经把上游声明的请求 ID 记在 usage_logs.upstream_request_id
-- 上（见 246 号迁移），后台任务拿这个 ID 直接向 A6 精确反查那一条账单，把真实扣费
-- 写回同一条使用记录。查不到就退避重试，重试用尽就记「未取到」。
-- 也就是说「账单」不再需要在本站存一份，「规则」与「快照」也不再需要。
--
-- 于是本迁移删掉三张专属于旧做法的表。
--
-- ⚠️ 为什么不删 reconciliation_sync_state：
-- 管理面板上配置的 A6 连接凭据（base_url / access_token / user_id 的运行时覆盖值）
-- 就存在这张表里（键名 a6_*_override），而**新的成本取数功能完全依赖这套凭据**——
-- 删掉这张表等于删掉面板上配好的 A6 连接，取数会全部失败。
-- 表名沿用历史命名（未重命名），是为了避免为了改名字去动线上的真实数据。
--
-- 数据可丢：这三张表里的数据都是旧做法在运行期采集/匹配出来的中间产物，
-- 任何一条都能由 usage_logs 与 A6 账单重新推导；且旧代码已删除，留着也无人再读。

-- 1) 删除旧做法专属的三张表。
--    先删子表式的 upstream_bills（它按 usage 维度引用过 usage_logs），再删其余两张。
--    用 IF EXISTS 保证幂等：重复执行或库中本就缺表时都不报错。
DROP TABLE IF EXISTS reconciliation_upstream_bills;
DROP TABLE IF EXISTS reconciliation_usage_extras;
DROP TABLE IF EXISTS reconciliation_account_rules;

-- 2) 清掉状态表里属于旧做法的游标与错误位。
--    这些键记录的是「上一轮批量拉账单/采集用量走到哪、上次报什么错」，
--    旧采集器已经不存在，键留着只会让后来人误以为还有人在写。
--    同步保存 A6 凭据的三个 a6_*_override 键**必须保留**，见上文说明。
DELETE FROM reconciliation_sync_state
WHERE key IN (
    'a6_last_sync_unix',
    'a6_last_sync_error',
    'a6_last_sync_error_at',
    'usage_last_collected_at',
    'usage_last_collected_id',
    'usage_last_batch_size',
    'usage_last_batch_truncated',
    'usage_pending_backlog'
);

-- 3) 在新职责上补注释，避免后来人看到表名又以为对账模块还在。
COMMENT ON TABLE reconciliation_sync_state IS
    '键值状态表。原属「经营对账」模块，该模块已删除；现存两类数据：A6 连接凭据的面板覆盖值（a6_base_url_override / a6_access_token_override_enc / a6_user_id_override，被上游成本取数依赖）与汇率的运行时覆盖值（fx_usd_cny_rate_override）。';
