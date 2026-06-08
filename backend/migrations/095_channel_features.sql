ALTER TABLE channels ADD COLUMN IF NOT EXISTS features TEXT NOT NULL DEFAULT '';
COMMENT ON COLUMN channels.features IS '渠道特性描述，JSON 数组格式，用于支付页面展示';
