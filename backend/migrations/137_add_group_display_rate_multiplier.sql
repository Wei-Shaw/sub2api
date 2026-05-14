-- 用户侧展示用费率倍数，不参与实际扣费。
-- 默认回填为现有实际费率，保证升级后用户看到的倍率与升级前一致。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS display_rate_multiplier DECIMAL(10,4);

UPDATE groups
SET display_rate_multiplier = rate_multiplier
WHERE display_rate_multiplier IS NULL;

ALTER TABLE groups
    ALTER COLUMN display_rate_multiplier SET DEFAULT 1.0,
    ALTER COLUMN display_rate_multiplier SET NOT NULL;

COMMENT ON COLUMN groups.display_rate_multiplier IS '用户侧展示用费率倍数，不参与实际扣费';
