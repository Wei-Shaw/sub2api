-- 智力检测改为管理员人工评审：记录评审人与评审时间。
-- 全部为增量列，不改动任何现有列，历史记录天然落在「待评审」（reviewed_at IS NULL）。
--
-- 背景：跑测本身只负责产出作品。原先的自动判定仅校验「有没有从响应里抽出 HTML」，
-- 并不反映作品质量（画得对不对），因此取消自动判定，判定权交给管理员：
--   unknown = 已产出作品、等待人工评审（列默认值，也是全部历史记录的现状）
--   pass    = 管理员评审为「通过」
--   fail    = 管理员评审为「不通过」
-- 跑测执行失败（超时 / 未配模型 / 没抽出 HTML）仍由 status='failed' + error_code 表达，
-- 与「评审不通过」是两个独立维度，不共用同一列。

ALTER TABLE intelligence_check_runs
    ADD COLUMN IF NOT EXISTS reviewed_by BIGINT,
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;

-- 管理员通常只关心「还没评的」，给一条部分索引，避免全表扫。
CREATE INDEX IF NOT EXISTS idx_intelligence_check_runs_pending_review
    ON intelligence_check_runs (created_at DESC)
    WHERE reviewed_at IS NULL;
