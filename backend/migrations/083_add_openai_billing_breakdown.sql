ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS priority_account_multiplier decimal(10,4),
    ADD COLUMN IF NOT EXISTS effective_multiplier decimal(10,4),
    ADD COLUMN IF NOT EXISTS effective_input_unit_price decimal(20,10),
    ADD COLUMN IF NOT EXISTS effective_output_unit_price decimal(20,10),
    ADD COLUMN IF NOT EXISTS effective_cache_read_unit_price decimal(20,10),
    ADD COLUMN IF NOT EXISTS pricing_source varchar(255);

COMMENT ON COLUMN usage_logs.priority_account_multiplier IS '优先账号倍率快照；命中优先账号链路时为 100，否则为 1';
COMMENT ON COLUMN usage_logs.effective_multiplier IS '最终倍率快照 = rate_multiplier * account_rate_multiplier * priority_account_multiplier';
COMMENT ON COLUMN usage_logs.effective_input_unit_price IS '本次请求实际生效的输入单价（USD/token）';
COMMENT ON COLUMN usage_logs.effective_output_unit_price IS '本次请求实际生效的输出单价（USD/token）';
COMMENT ON COLUMN usage_logs.effective_cache_read_unit_price IS '本次请求实际生效的缓存读取单价（USD/token）';
COMMENT ON COLUMN usage_logs.pricing_source IS '价格来源标记，如 priority_pricing,priority_account_multiplier';

UPDATE usage_logs
SET
    priority_account_multiplier = CASE WHEN LOWER(COALESCE(routing_target_group, '')) = 'active' THEN 100 ELSE 1 END,
    effective_multiplier = COALESCE(rate_multiplier, 1) * COALESCE(account_rate_multiplier, 1) * CASE WHEN LOWER(COALESCE(routing_target_group, '')) = 'active' THEN 100 ELSE 1 END,
    actual_cost = total_cost * (COALESCE(rate_multiplier, 1) * COALESCE(account_rate_multiplier, 1) * CASE WHEN LOWER(COALESCE(routing_target_group, '')) = 'active' THEN 100 ELSE 1 END),
    pricing_source = TRIM(BOTH ',' FROM CONCAT(
        CASE WHEN LOWER(COALESCE(service_tier, '')) = 'priority' THEN 'priority_pricing' ELSE '' END,
        CASE WHEN LOWER(COALESCE(service_tier, '')) = 'priority' AND LOWER(COALESCE(routing_target_group, '')) = 'active' THEN ',' ELSE '' END,
        CASE WHEN LOWER(COALESCE(routing_target_group, '')) = 'active' THEN 'priority_account_multiplier' ELSE '' END
    ))
WHERE priority_account_multiplier IS NULL
   OR effective_multiplier IS NULL
   OR pricing_source IS NULL;
