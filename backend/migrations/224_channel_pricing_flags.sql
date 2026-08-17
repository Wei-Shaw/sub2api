-- 渠道模型定价增加 enabled(启用/停用) 和 hidden(显示/隐藏) 标志
-- enabled=false 表示该定价条目被停用，等价于不存在（不参与计费，也不在模型列表中显示）
-- hidden=true   表示该定价条目在可用渠道/返回的模型列表中隐藏，但仍参与计费
ALTER TABLE channel_model_pricing
	ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS hidden BOOLEAN NOT NULL DEFAULT FALSE;
