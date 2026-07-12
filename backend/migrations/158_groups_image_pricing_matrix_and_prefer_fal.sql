-- 为 groups 表新增图片二维定价矩阵与 fal 优先调度开关。
-- image_pricing_matrix: JSONB, nullable，结构 {tier_key: {quality_key: price}}，
--   tier_key ∈ {"1024x768","1024x1024","1024x1536","1920x1080","2560x1440","3840x2160"}，
--   quality_key ∈ {"low","medium","high"}。NULL/'{}' 表示未启用矩阵定价，
--   计费回退到旧 image_price_1k/2k/4k 字段（与本次变更前完全一致）。
-- image_prefer_fal: BOOLEAN，仅 platform='openai' 分组消费；true 时图片调度
--   反转优先级（fal 账号优先，openai 账号兜底）。其他平台分组写入该列无效果。
-- 两列对存量数据零影响：默认 NULL/false，等价现状行为。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_pricing_matrix JSONB,
    ADD COLUMN IF NOT EXISTS image_prefer_fal BOOLEAN NOT NULL DEFAULT FALSE;
