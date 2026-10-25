-- MySQL 方言版本（翻译自 ../015_fix_settings_unique_constraint.sql）
-- 015_fix_settings_unique_constraint.sql
-- 修复 settings 表 key 字段缺失的唯一约束
-- 此约束是 ON CONFLICT ("key") DO UPDATE 语句所必需的
--
-- 翻译说明：PG DO 块（pg_constraint 检查 settings_key_key 是否存在 + 条件 ADD CONSTRAINT）
--   → 临时存储过程改写。
--   检查语义对齐为「settings.`key` 上是否存在任一 UNIQUE 约束」：
--   PG 全新安装时 005 的内联 UNIQUE 即命名为 settings_key_key（守卫为真，无操作）；
--   MySQL 全新安装时 005 的内联 UNIQUE 索引名为 `key`，同样命中守卫、无操作。
--   仅当 key 列完全缺失唯一约束（老库）时才补建，命名沿用 PG 的 settings_key_key。

DROP PROCEDURE IF EXISTS __mig_tmp;
CREATE PROCEDURE __mig_tmp()
BEGIN
    DECLARE v_uc_count INT;

    SELECT COUNT(*) INTO v_uc_count
    FROM information_schema.table_constraints tc
    JOIN information_schema.key_column_usage kcu
      ON kcu.constraint_schema = tc.constraint_schema
     AND kcu.constraint_name = tc.constraint_name
    WHERE tc.table_schema = DATABASE()
      AND tc.table_name = 'settings'
      AND tc.constraint_type = 'UNIQUE'
      AND kcu.column_name = 'key';

    IF v_uc_count = 0 THEN
        ALTER TABLE settings ADD CONSTRAINT settings_key_key UNIQUE (`key`);
    END IF;
END;
CALL __mig_tmp();
DROP PROCEDURE __mig_tmp;
