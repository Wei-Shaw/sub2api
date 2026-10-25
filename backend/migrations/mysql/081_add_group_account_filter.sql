-- MySQL 方言版本（翻译自 ../081_add_group_account_filter.sql）

ALTER TABLE `groups` ADD COLUMN require_oauth_only BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE `groups` ADD COLUMN require_privacy_set BOOLEAN NOT NULL DEFAULT FALSE;
