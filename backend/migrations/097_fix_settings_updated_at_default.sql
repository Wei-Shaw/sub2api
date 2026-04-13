-- 097_fix_settings_updated_at_default.sql
--
-- 修复 settings.updated_at 列在历史实例上可能缺失 SQL DEFAULT 的问题。
--
-- 背景：
--   早期版本曾依赖 ent 自动迁移建表（ent 的 Default(time.Now) 仅是 Go 层默认值，
--   不会在 SQL 层落地为 DEFAULT），随后引入的 005_schema_parity.sql 使用了
--   CREATE TABLE IF NOT EXISTS，对已存在的 settings 表不会重建，导致这部分实例
--   的 updated_at 列虽然是 NOT NULL，但缺少 SQL DEFAULT。
--
--   后续 098_migrate_purchase_subscription_to_custom_menu.sql 是项目中唯一使用
--   原生 SQL INSERT INTO settings 的迁移（其余 settings 写入都走 ent / Go 层），
--   因此该 schema 缺陷直到 098 才会触发：
--     "null value in column \"updated_at\" of relation \"settings\" violates not-null constraint"
--
-- 幂等性：
--   - ALTER COLUMN ... SET DEFAULT NOW() 在已经具备相同默认值的实例上是无操作，
--     不会报错（PostgreSQL 允许重复设置相同的默认值）。
--   - UPDATE 子句的 WHERE updated_at IS NULL 在健康实例上匹配 0 行，不影响数据。
--
-- 这样可以同时兼容：
--   1. 从未运行过旧版迁移的全新部署（005 已经把列建对，本迁移变成 no-op）。
--   2. 历史损坏实例（本迁移修复缺失的默认值，使后续 098 能够正常 INSERT）。

ALTER TABLE settings ALTER COLUMN updated_at SET DEFAULT NOW();

UPDATE settings SET updated_at = NOW() WHERE updated_at IS NULL;
