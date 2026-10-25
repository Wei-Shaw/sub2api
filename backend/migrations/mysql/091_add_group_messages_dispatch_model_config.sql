-- MySQL 方言版本（翻译自 ../091_add_group_messages_dispatch_model_config.sql）

ALTER TABLE `groups`
ADD COLUMN messages_dispatch_model_config JSON NOT NULL DEFAULT ('{}');
