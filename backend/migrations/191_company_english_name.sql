-- 为企业账户与企业升级申请增加"公司英文名"，并保证其全局唯一。
--
-- 设计说明：
--   - english_name 保存用户提交的原始英文名（保留大小写与空格折叠后的展示形态）。
--   - normalized_english_name 为规范化后的小写形态，用于唯一性判定。
--   - 历史数据没有英文名，允许为 NULL；唯一索引仅约束非 NULL 值。
--   - organizations 上的唯一索引保证已生效企业的英文名全局唯一。
--   - company_upgrade_applications 上的部分唯一索引保证"待审核"申请之间英文名不冲突。

ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS english_name VARCHAR(255);
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS normalized_english_name VARCHAR(255);
CREATE UNIQUE INDEX IF NOT EXISTS organizations_normalized_english_name_unique
    ON organizations(normalized_english_name)
    WHERE normalized_english_name IS NOT NULL;

ALTER TABLE company_upgrade_applications
    ADD COLUMN IF NOT EXISTS requested_english_name VARCHAR(255);
ALTER TABLE company_upgrade_applications
    ADD COLUMN IF NOT EXISTS normalized_english_name VARCHAR(255);
CREATE UNIQUE INDEX IF NOT EXISTS company_upgrade_pending_english_unique
    ON company_upgrade_applications(normalized_english_name)
    WHERE status = 'pending' AND normalized_english_name IS NOT NULL;
