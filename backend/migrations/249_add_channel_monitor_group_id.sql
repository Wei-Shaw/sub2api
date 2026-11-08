-- Migration: 249_add_channel_monitor_group_id
--
-- 渠道监控直连分组：把「监控 → 模型广场分组」从「按名字猜」改成真外键。
--
-- 背景（真实事故）
-- ----------------
-- 「模型广场 Pro」页的一张卡片 = 一个渠道监控，而卡片要显示某个分组的模型与定价，
-- 就必须知道「这个监控代表哪个分组」。旧实现没有外键，只能靠三级降级猜：
--
--   ① channel_monitors.account_id → account_groups → group_id
--   ② groups.name = channel_monitors.group_name （自由文本标签）
--   ③ groups.name = channel_monitors.name        （监控名）
--
-- 其中 ②③ 是「按名字精确匹配」，并列时取 id 最小。一旦库里出现同名分组
-- —— 尤其是「删掉旧分组、又建了同名新分组」这种操作 —— 就会稳定地认领到
-- 那条已被软删除的旧记录，导致卡片永远显示「0 个模型」。
-- 生产实例：codex-官方0.3折 有 id=5（已软删）与 id=37（活跃）两条同名记录，
-- 监控认领到了 5，而 5 不在模型广场里，于是卡片拿不到任何模型。
--
-- 本迁移的解法
-- ------------
-- 加真外键 group_id，并保留「硬删除时自动置空」的兜底。
-- ⚠️ 注意 ON DELETE SET NULL 只对**硬删除**生效，而后台删分组是**软删除**
-- （写 deleted_at、status 仍可能为 active），所以读取侧必须额外过滤
-- `deleted_at IS NULL AND status = 'active'`（与 ChannelMonitorV2 的既有做法一致）。
--
-- 回填说明
-- --------
-- 若只加列不回填，所有既有监控的 group_id 都是 NULL，卡片会集体失去模型与定价
-- （严重退化）。因此这里把旧的三级降级结果**固化**下来，但给每一级都加上
-- 「目标分组必须未删除且 active」的约束 —— 于是旧实现认领错的那一个会被自动
-- 修正到正确的活跃分组（本例中 5 → 37），其余保持原样，做到零退化。

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_id
    ON channel_monitors (group_id);

-- 回填：复刻旧的三级优先级（账号 > 分组名标签 > 监控名），
-- 但三级都要求候选分组未删除且 active。
UPDATE channel_monitors m
SET group_id = resolved.group_id
FROM (
    SELECT mk.monitor_id,
           COALESCE(account_group.group_id, group_name_group.id, monitor_name_group.id) AS group_id
    FROM (
        SELECT id AS monitor_id, account_id, group_name, name
        FROM channel_monitors
    ) mk
    LEFT JOIN LATERAL (
        SELECT ag.group_id
        FROM account_groups ag
        JOIN groups g ON g.id = ag.group_id
        WHERE mk.account_id > 0
          AND ag.account_id = mk.account_id
          AND g.deleted_at IS NULL
          AND g.status = 'active'
        ORDER BY ag.priority ASC, ag.group_id ASC
        LIMIT 1
    ) account_group ON TRUE
    LEFT JOIN LATERAL (
        SELECT g.id
        FROM groups g
        WHERE mk.group_name <> ''
          AND g.name = mk.group_name
          AND g.deleted_at IS NULL
          AND g.status = 'active'
        ORDER BY g.id ASC
        LIMIT 1
    ) group_name_group ON TRUE
    LEFT JOIN LATERAL (
        SELECT g.id
        FROM groups g
        WHERE mk.name <> ''
          AND g.name = mk.name
          AND g.deleted_at IS NULL
          AND g.status = 'active'
        ORDER BY g.id ASC
        LIMIT 1
    ) monitor_name_group ON TRUE
) resolved
WHERE m.id = resolved.monitor_id
  AND m.group_id IS NULL
  AND resolved.group_id IS NOT NULL;