-- MySQL 方言版本（翻译自 ../190_add_users_email_alias_dedup_index_notx.sql）
-- Registration alias dedup (repository.existsByEmailAliasWithClient) probes users
-- by the dot-stripped email form, on the public register / send-verify-code paths.
-- Index that exact expression so the probes stay index lookups instead of a
-- sequential scan. text_pattern_ops serves both the equality probe and the
-- "local+%@domain" prefix probe regardless of database collation.
--
-- 说明：_notx 文件保留文件名，CONCURRENTLY 去除（MySQL 不支持，迁移执行器保证单次执行）；
--       text_pattern_ops → 普通 B-tree 索引；非唯一部分索引（WHERE deleted_at IS NULL）→ 去掉 WHERE；
--       users 表已有 idx_users_email 普通索引，本索引名不同、不冲突。
CREATE INDEX idx_users_email_dot_stripped
    ON users ((REPLACE(LOWER(TRIM(email)), '.', '')));
