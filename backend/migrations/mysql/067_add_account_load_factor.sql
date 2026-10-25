-- MySQL 方言版本（翻译自 ../067_add_account_load_factor.sql）
ALTER TABLE accounts ADD COLUMN load_factor INT NULL;
