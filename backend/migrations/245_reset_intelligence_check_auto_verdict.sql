-- 智力检测改为管理员人工评审后的历史数据修正。
--
-- 背景：迁移 244 之前，verdict 由跑测流程自动写入 —— 只要从上游响应里抽出了 HTML 就写 pass，
-- 完全不看作品质量，也没有任何评审人（reviewed_at 恒为 NULL）。这些旧结论不能冒充人工评审结果：
-- 一旦打开「评审结论联动账号状态」，它会把一个从未被评审过的结论当作「通过」去恢复账号状态。
--
-- 因此把「从未人工评审过」的旧结论统一重置为 unknown（待评审），交给管理员重新评定。
-- 判定条件严格限定 reviewed_at IS NULL —— 已经人工评审过的记录一律不动。
--
-- 关于跑测执行失败的记录：它们同样是 reviewed_at IS NULL + verdict='fail'，也会被这条语句
-- 重置为 unknown。这是刻意且安全的：前端与判定一律以 status 为准（status='failed' 即「执行失败」），
-- verdict 从此只承载人工评审结论，不再兼任执行结果。

UPDATE intelligence_check_runs
SET verdict = 'unknown'
WHERE reviewed_at IS NULL
  AND verdict <> 'unknown';
