-- MySQL 方言版本（翻译自 ../008_seed_default_group.sql）
-- Seed a default group for fresh installs.
--
-- 翻译说明：PG 的 SELECT 无 FROM 子句 → MySQL 需 FROM DUAL 才能带 WHERE；
--           `groups` 为 MySQL 8 保留字；NOW() → NOW(3)。
INSERT INTO `groups` (name, description, created_at, updated_at)
SELECT 'default', 'Default group', NOW(3), NOW(3)
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `groups`);
