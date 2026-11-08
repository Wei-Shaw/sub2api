-- 248_upstream_cost_drop_fx.sql
--
-- 背景：246 号迁移为「上游成本」加了四个金额列，其中两列是为「把上游扣费换算成
-- 人民币」准备的（upstream_cost_cny 换算结果、upstream_cost_fx_rate 当时用的汇率）。
--
-- 主人明确要求：**不要换算**。
--
-- 理由（业务侧）：本站使用记录里的「费用」本来就是美元原值，而对账要看的恰恰是
-- 「收了多少美元 / 上游实际扣了多少美元」这两者的差额。一旦把成本换算成人民币，
-- 页面上就是一列美元、一列人民币，得先自己心算汇率才能比较——换算在这里不是
-- 增值，而是给对比添乱。所以两边统一保留上游/本站的原始币种，直接相减。
--
-- 因此本迁移删掉那两个只服务于换算的列。
--
-- 保留下来的：
--   upstream_cost_original  —— 上游账单里的原币金额（A6 为美元），也是页面显示与
--                              「盈利/利润率」计算的唯一金额来源；
--   upstream_cost_currency  —— 原币币种，用于显示时挑货币符号，不参与计算。
--
-- 数据可丢：这两列从未在生产上使用过（246/247/248 尚未部署到生产），
-- 开发库里的值是验证期间写入的，删掉不影响任何已交付结论。

ALTER TABLE usage_logs DROP COLUMN IF EXISTS upstream_cost_cny;
ALTER TABLE usage_logs DROP COLUMN IF EXISTS upstream_cost_fx_rate;

COMMENT ON COLUMN usage_logs.upstream_cost_original IS
    '上游账单里的原币扣费金额（A6 为美元）。与 usage_logs.actual_cost 同为美元口径，两者相减即毛利。不做任何汇率换算。';
COMMENT ON COLUMN usage_logs.upstream_cost_currency IS
    'upstream_cost_original 的币种，用于显示货币符号，不参与计算。';
