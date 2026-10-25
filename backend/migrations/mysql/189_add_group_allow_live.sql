-- MySQL 方言版本（翻译自 ../189_add_group_allow_live.sql）
ALTER TABLE `groups` ADD COLUMN allow_live BOOLEAN NOT NULL DEFAULT FALSE;
