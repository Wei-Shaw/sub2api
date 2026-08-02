-- 公司账户改用系统生成的公司ID（company_id）作为对外唯一标识，取代此前的「公司英文名」唯一标识方案。
--
-- 设计说明：
--   * company_id 形如 'c' + 15 位数字（共 16 个字符），与数字型 account_id 分离：
--     - account_id 仍复用 owner 的 16 位数字账号，并用于 IAM 成员账号派生
--       （users.account_id 存在数字格式 CHECK 约束，因此不能直接改成以 'c' 开头）。
--     - company_id 则是公司自身对外展示、检索用的唯一标识。
--   * 历史组织回填一个确定性的 company_id；新组织在审批通过时随机生成（见 accountid.GenerateCompany）。
--   * 同时移除英文名相关的列与唯一索引（英文名不再作为唯一标识）。

-- 1) 新增公司ID列并回填历史数据。
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS company_id VARCHAR(16);

UPDATE organizations
    SET company_id = 'c' || lpad(id::text, 15, '0')
    WHERE company_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS organizations_company_id_unique
    ON organizations(company_id)
    WHERE company_id IS NOT NULL;

DO $$ BEGIN
    ALTER TABLE organizations
        ADD CONSTRAINT organizations_company_id_format_check
        CHECK (company_id IS NULL OR company_id ~ '^c[0-9]{15}$');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- 2) 移除英文名唯一标识方案（列 + 相关唯一索引）。
DROP INDEX IF EXISTS organizations_normalized_english_name_unique;
DROP INDEX IF EXISTS company_upgrade_pending_english_unique;

ALTER TABLE organizations DROP COLUMN IF EXISTS english_name;
ALTER TABLE organizations DROP COLUMN IF EXISTS normalized_english_name;

ALTER TABLE company_upgrade_applications DROP COLUMN IF EXISTS requested_english_name;
ALTER TABLE company_upgrade_applications DROP COLUMN IF EXISTS normalized_english_name;
