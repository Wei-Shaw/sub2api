-- MySQL 方言版本（翻译自 ../041_add_model_routing_enabled.sql）

-- Add model_routing_enabled field to groups table
-- （`groups` 为 MySQL 8 保留字，加反引号）
ALTER TABLE `groups` ADD COLUMN model_routing_enabled BOOLEAN NOT NULL DEFAULT FALSE;
