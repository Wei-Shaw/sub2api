ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS group_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;

COMMENT ON COLUMN accounts.group_rate_multiplier IS
    '账号级分组计费倍率：与用户/API Key生效分组倍率相乘，默认1.0；独立于账号计费倍率';
