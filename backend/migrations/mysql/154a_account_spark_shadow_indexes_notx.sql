-- MySQL 方言版本（翻译自 ../154a_account_spark_shadow_indexes_notx.sql）
-- （PG 版使用 CONCURRENTLY；MySQL 无此语法，直接创建。
--   第一个索引：非唯一部分索引 WHERE parent_account_id IS NOT NULL → 去掉 WHERE；
--   第二个索引：唯一部分索引 → 函数键部分。）

CREATE INDEX idx_accounts_parent_account_id
    ON accounts (parent_account_id);

CREATE UNIQUE INDEX uq_accounts_spark_shadow_per_parent
    ON accounts (
        parent_account_id,
        (IF(parent_account_id IS NOT NULL AND quota_dimension = 'spark' AND deleted_at IS NULL, 0, NULL))
    );
