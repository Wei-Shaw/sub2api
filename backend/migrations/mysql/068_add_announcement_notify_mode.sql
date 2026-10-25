-- MySQL 方言版本（翻译自 ../068_add_announcement_notify_mode.sql）
ALTER TABLE announcements ADD COLUMN notify_mode VARCHAR(20) NOT NULL DEFAULT 'silent';
