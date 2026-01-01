-- 011_remove_duplicate_unique_indexes.sql
-- 移除重复的唯一索引
-- 这些字段在 ent schema 的 Fields() 中已声明 .Unique()，
-- 因此在 Indexes() 中再次声明 index.Fields("x").Unique() 会创建重复索引。
-- 本迁移脚本清理这些冗余索引。

-- 重复索引命名约定（由 Ent 自动生成/历史迁移遗留）:
-- - 字段级 Unique() 创建的索引名: <table>_<field>_key
-- - Indexes() 中的 Unique() 创建的索引名: <table>_<field>
-- - 初始化迁移中的非唯一索引: idx_<table>_<field>

-- 仅当索引存在时才删除（幂等操作）

-- api_keys 表: key 字段
DROP INDEX IF EXISTS apikey_key;
DROP INDEX IF EXISTS api_keys_key;
DROP INDEX IF EXISTS idx_api_keys_key;

-- users 表: email 字段
DROP INDEX IF EXISTS user_email;
DROP INDEX IF EXISTS users_email;
DROP INDEX IF EXISTS idx_users_email;

-- settings 表: key 字段
DROP INDEX IF EXISTS settings_key;
DROP INDEX IF EXISTS idx_settings_key;

-- redeem_codes 表: code 字段
DROP INDEX IF EXISTS redeemcode_code;
DROP INDEX IF EXISTS redeem_codes_code;
DROP INDEX IF EXISTS idx_redeem_codes_code;

-- groups 表: name 字段
DROP INDEX IF EXISTS group_name;
DROP INDEX IF EXISTS groups_name;
DROP INDEX IF EXISTS idx_groups_name;

-- 注意: 每个字段的唯一约束仍由字段级 Unique() 创建的约束保留，
-- 如 api_keys_key_key、users_email_key 等。
