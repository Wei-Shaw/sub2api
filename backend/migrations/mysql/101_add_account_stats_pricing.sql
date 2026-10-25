-- MySQL 方言版本（翻译自 ../101_add_account_stats_pricing.sql）
-- Account statistics pricing: allow channels to configure custom pricing for account cost tracking.
--
-- 说明：BIGINT[]（group_ids/account_ids）→ JSON，默认空数组 DEFAULT ('[]')（数组语义）；
--       NUMERIC → DECIMAL。

-- 1. Channel-level toggle
ALTER TABLE channels ADD COLUMN apply_pricing_to_account_stats BOOLEAN NOT NULL DEFAULT FALSE;

-- 2. Account stats pricing rules (ordered list per channel)
CREATE TABLE IF NOT EXISTS channel_account_stats_pricing_rules (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    channel_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL DEFAULT '',
    group_ids JSON NOT NULL DEFAULT ('[]'),
    account_ids JSON NOT NULL DEFAULT ('[]'),
    sort_order INT NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_cas_pricing_rules_channel_id FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_cas_pricing_rules_channel_id ON channel_account_stats_pricing_rules(channel_id);

-- 3. Model pricing for each rule (same structure as channel_model_pricing)
CREATE TABLE IF NOT EXISTS channel_account_stats_model_pricing (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    rule_id BIGINT NOT NULL,
    platform VARCHAR(50) NOT NULL DEFAULT '',
    models JSON NOT NULL DEFAULT ('[]'),
    billing_mode VARCHAR(20) NOT NULL DEFAULT 'token',
    input_price DECIMAL(20,10),
    output_price DECIMAL(20,10),
    cache_write_price DECIMAL(20,10),
    cache_read_price DECIMAL(20,10),
    image_output_price DECIMAL(20,10),
    per_request_price DECIMAL(20,10),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_cas_model_pricing_rule_id FOREIGN KEY (rule_id) REFERENCES channel_account_stats_pricing_rules(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_cas_model_pricing_rule_id ON channel_account_stats_model_pricing(rule_id);

-- 4. Usage logs: pre-computed account stats cost (NULL = use default formula)
ALTER TABLE usage_logs ADD COLUMN account_stats_cost DECIMAL(20,10) NULL;
