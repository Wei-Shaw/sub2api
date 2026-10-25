-- MySQL 方言版本（翻译自 ../181_group_duplicate_operation_id.sql）
-- Persist an internal operation identity for safely recovering an already
-- committed group duplicate when the idempotency response write is ambiguous.
--
-- 说明：唯一部分索引 WHERE duplicate_operation_id IS NOT NULL AND deleted_at IS NULL
--       → 函数键部分（两个条件同时为真才参与唯一性）。

ALTER TABLE `groups`
    ADD COLUMN duplicate_operation_id VARCHAR(64) NULL;

CREATE UNIQUE INDEX idx_groups_duplicate_operation_id_active
    ON `groups` (
        duplicate_operation_id,
        (IF(duplicate_operation_id IS NOT NULL AND deleted_at IS NULL, 0, NULL))
    );
