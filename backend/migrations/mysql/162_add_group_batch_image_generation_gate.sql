-- MySQL 方言版本（翻译自 ../162_add_group_batch_image_generation_gate.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。
--   - COMMENT ON COLUMN → 整句跳过。

ALTER TABLE `groups`
    ADD COLUMN allow_batch_image_generation BOOLEAN NOT NULL DEFAULT FALSE;

-- (comment skipped)
