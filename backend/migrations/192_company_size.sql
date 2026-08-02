-- 为企业账户与企业升级申请增加"公司规模"字段。
--
-- 设计说明：
--   - company_size 保存申请人选择的公司规模区间（枚举字符串）。
--   - 取值范围：'1-20' / '20-100' / '100-300' / '300-1000' / '1000+'。
--   - 历史数据没有该字段，允许为 NULL。
--   - organizations 上记录审批通过时快照的公司规模。

ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS company_size VARCHAR(20);

ALTER TABLE company_upgrade_applications
    ADD COLUMN IF NOT EXISTS company_size VARCHAR(20);
