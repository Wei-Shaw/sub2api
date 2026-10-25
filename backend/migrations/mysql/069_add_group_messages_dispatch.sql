-- MySQL 方言版本（翻译自 ../069_add_group_messages_dispatch.sql）
ALTER TABLE `groups` ADD COLUMN allow_messages_dispatch BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE `groups` ADD COLUMN default_mapped_model VARCHAR(100) NOT NULL DEFAULT '';
