-- MySQL 方言版本（翻译自 ../154_account_spark_shadow.sql）
-- 154_account_spark_shadow.sql
--
-- 说明：MySQL 无 NOT VALID / VALIDATE CONSTRAINT —— 约束添加时即对存量数据全量校验。
--       PG 版「幂等加约束」的存在性检查由迁移执行器保证单次执行，直接执行语句体；
--       三个约束合并为一条 ALTER TABLE（MySQL 会对其中的 CHECK/FK 一次性校验存量行）。
--
--       ⚠ chk_accounts_parent_not_self（禁自指 CHECK parent_account_id <> id）无法翻译：
--       MySQL 的 CHECK 约束禁止引用 AUTO_INCREMENT 列（accounts.id 为自增主键），
--       该不变式改由应用层保证（见迁移报告疑点）。

ALTER TABLE accounts
    ADD COLUMN parent_account_id BIGINT NULL,
    ADD COLUMN quota_dimension VARCHAR(20) NOT NULL DEFAULT 'global';

-- 维度合法 + parent⟺非global 维度一致(评审 P1-d)；禁自指约束见上方说明
ALTER TABLE accounts
    ADD CONSTRAINT chk_accounts_quota_dimension
      CHECK (quota_dimension IN ('global','spark')),
    ADD CONSTRAINT chk_accounts_parent_dimension
      CHECK ((parent_account_id IS NULL AND quota_dimension = 'global')
          OR (parent_account_id IS NOT NULL AND quota_dimension <> 'global')),
    ADD CONSTRAINT fk_accounts_parent_account_id
      FOREIGN KEY (parent_account_id) REFERENCES accounts(id) ON DELETE RESTRICT;
