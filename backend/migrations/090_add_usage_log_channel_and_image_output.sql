ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS channel_id BIGINT,
    ADD COLUMN IF NOT EXISTS model_mapping_chain VARCHAR(500),
    ADD COLUMN IF NOT EXISTS billing_tier VARCHAR(50),
    ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(20),
    ADD COLUMN IF NOT EXISTS image_output_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS image_output_cost DECIMAL(20,10) NOT NULL DEFAULT 0;

COMMENT ON COLUMN usage_logs.channel_id IS '渠道 ID';
COMMENT ON COLUMN usage_logs.model_mapping_chain IS '模型映射链，如 alias->mapped';
COMMENT ON COLUMN usage_logs.billing_tier IS '计费层级标签';
COMMENT ON COLUMN usage_logs.billing_mode IS '计费模式：token/per_request/image';
COMMENT ON COLUMN usage_logs.image_output_tokens IS '图片输出 token 数';
COMMENT ON COLUMN usage_logs.image_output_cost IS '图片输出成本';
